package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"switchblade/internal/api"
	"switchblade/internal/auth"
	"switchblade/internal/billing"
	"switchblade/internal/config"
	"switchblade/internal/db"
	"switchblade/internal/metrics"
	"switchblade/internal/mitm"
	"switchblade/internal/mitm/handlers"
	"switchblade/internal/providers"
	"switchblade/internal/proxy"
	"switchblade/internal/relay"
	"switchblade/internal/reqctx"
	"switchblade/internal/ws"
)

// Version info — injected via -ldflags at build time.
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func main() {
	setupLogging()

	cmds := map[string]bool{
		"init":    true,
		"status":  true,
		"tenants": true,
		"users":   true,
		"keys":    true,
		"tiers":   true,
		"backup":  true,
		"stats":   true,
		"seed":    true,
		"dev":     true,
		"version": true,
		"restore": true,
		"help":    true,
		"-h":      true,
		"--help":  true,
	}

	if len(os.Args) >= 2 {
		first := os.Args[1]
		if first == "start" {
			runServer()
		} else if cmds[first] {
			runCLI()
		} else {
			errorf("unknown command: %s", first)
			printHelp("")
			os.Exit(1)
		}
	} else {
		runServer()
	}
}

// setupLogging installs the default slog logger: text on stderr with the
// level from LOG_LEVEL (debug|info|warn|error, default info).
func setupLogging() {
	level := slog.LevelInfo
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
}

func runServer() {
	cfg := config.Load()

	// Database
	database, err := db.Open(cfg.DatabasePath)
	if err != nil {
		slog.Error("db open", "err", err)
		os.Exit(1)
	}
	defer database.Close()
	if err := database.Migrate(); err != nil {
		slog.Error("db migrate", "err", err)
		os.Exit(1)
	}
	// Versioned migrations (applies pending only)
	if err := db.ApplyVersionedMigrations(database.DB); err != nil {
		slog.Error("db versioned migrate", "err", err)
		os.Exit(1)
	}

	// Seed default owner if no users exist.
	seedDefaultOwner(database.DB)

	// Pre-startup backup
	backupDir := filepath.Join(filepath.Dir(cfg.DatabasePath), "backups")
	if _, err := db.Backup(cfg.DatabasePath, backupDir, cfg.BackupRetention); err != nil {
		slog.Error("[backup] pre-startup backup failed", "err", err)
	}

	// Context cancelled on SIGINT/SIGTERM — shared by all background goroutines
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Auto backup goroutine
	db.StartAutoBackup(ctx, cfg.DatabasePath, backupDir, cfg.BackupIntervalMinutes, cfg.BackupRetention)

	// Log rotation goroutine (run once at startup, then daily)
	go func() {
		if err := database.PruneRequestLogs(cfg.LogRetentionDays); err != nil {
			slog.Warn("[db] prune request_logs", "err", err)
		}
		if err := database.PruneUsageSummary(cfg.UsageRetentionDays); err != nil {
			slog.Warn("[db] prune usage_summary", "err", err)
		}
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = database.PruneRequestLogs(cfg.LogRetentionDays)
				_ = database.PruneUsageSummary(cfg.UsageRetentionDays)
			}
		}
	}()

	// Image cleanup goroutine
	api.StartImageCleanup(ctx, database, cfg.StorageDir, cfg.ImageRetentionDays)

	// Response cache (proxy-level)
	cache := proxy.NewResponseCache(cfg.CacheDefaultTTLSec, cfg.CacheEnabled)

	// API-level cache — separate from proxy.ResponseCache
	apiCache := api.NewCache(cfg.CacheDefaultTTLSec)

	// Rate limiter
	rl := proxy.NewRateLimiter(ctx, cfg.RateLimitDefault, cfg.RateLimitBurst, cfg.RateLimitEnabled)

	// Provider config store — hot-reloads base URLs from DB every 30s.
	store := providers.NewConfigStore(database.DB)
	defer store.Stop()

	// Provider registry — priority order matters (first OwnsModel match wins).
	reg := &providers.Registry{ConfigStore: store}
	providers.RegisterAll(reg, store)
	slog.Info(fmt.Sprintf("[providers] registered %d providers", len(reg.All())))

	// SSE hub
	hub := ws.NewHub()
	go hub.Run()

	// Quota monitor — checks tenant quotas every 5 minutes
	quotaMon := api.StartQuotaMonitor(database.DB, 5*time.Minute)
	defer func() {
		for _, id := range quotaMon.BlockedTenants() {
			slog.Info(fmt.Sprintf("[quota] tenant %s blocked at shutdown", id))
		}
	}()

	// Account pools — resolved per (tenant, provider) at request time so one
	// tenant's credentials can never serve another tenant's traffic.
	poolMgr := proxy.NewPoolManager(database, "round_robin")

	// Usage meter — prices every request, writes the audit trail, and charges
	// the tenant. Without it usage_records and request_logs stay empty and
	// nothing can be invoiced.
	meter := billing.NewMeter(database.DB, 4096)
	meter.Start()
	defer meter.Close()

	// Pipeline components — wire all scaffolded stages into the router
	filters := proxy.NewFilters(database)
	modelMapper := proxy.NewModelMapper(database)
	comboStore := proxy.NewComboStore(database)
	fallback := proxy.NewTenantFallbackExecutor(reg, poolMgr, cfg.FallbackMaxAttempts, cfg.FallbackTimeoutMS)

	proxyRouter := proxy.NewRouter(reg, nil, hub, proxy.RouterOptions{
		Filters:     filters,
		Cache:       cache,
		ModelMapper: modelMapper,
		Fallback:    fallback,
		Combos:      comboStore,
		Meter:       meter,
		Pools:       poolMgr,
	})

	// MITM Bridge — TLS-intercepting proxy for IDE traffic
	var mitmProxy *mitm.Proxy
	var mitmDNS *mitm.DNSHijacker
	if cfg.MitmEnabled {
		mitmCA, err := mitm.LoadOrGenerateCA(cfg.MitmCACert, cfg.MitmCAKey)
		if err != nil {
			slog.Error("mitm ca", "err", err)
			os.Exit(1)
		}

		mitmRegistry := handlers.NewRegistry()
		mitmRegistry.Register(handlers.NewCursorHandler())
		mitmRegistry.Register(handlers.NewCopilotHandler())
		mitmRegistry.Register(handlers.NewKiroHandler())
		mitmRegistry.Register(handlers.NewAntigravityHandler())

		mitmCfg := mitm.Config{
			Enabled: cfg.MitmEnabled,
			Port:    cfg.MitmPort,
			CertDir: cfg.MitmCertDir,
			CAKey:   cfg.MitmCAKey,
			CACert:  cfg.MitmCACert,
		}

		// Intercepted IDE traffic carries no tenant identity, so the bridge is
		// entitled to the shared inventory only.
		mitmPools := make(map[string]*proxy.AccountPool)
		for _, p := range reg.All() {
			mitmPools[p.Name()] = poolMgr.Shared(p.Name())
		}
		mitmProxy = mitm.NewProxy(mitmCfg, mitmCA, database, mitmPools, mitmRegistry)
		mitmDNS = mitm.NewDNSHijacker()

		slog.Info(fmt.Sprintf("[mitm] bridge initialized (port %d)", cfg.MitmPort))
	}

	// Login queue
	lq := auth.NewLoginQueue(3)
	lq.SetPythonPath(cfg.PythonPath)
	lq.OnSuccess = func(id int64, tokens string) {
		if err := database.UpdateAccountTokens(id, tokens); err != nil {
			slog.Info(fmt.Sprintf("[auth/queue] UpdateAccountTokens %d: %v", id, err))
		}
	}
	lq.OnError = func(id int64, msg string) {
		if err := database.UpdateAccountStatus(id, "error", msg); err != nil {
			slog.Info(fmt.Sprintf("[auth/queue] UpdateAccountStatus %d: %v", id, err))
		}
	}
	lq.Start()

	// Warmup scheduler
	warmupQ := auth.NewWarmupQueue(database, reg, 5)
	scheduler := auth.NewScheduler(warmupQ, 15)
	scheduler.Start()

	// HTTP router — port cfg.Port (1930), proxy + public endpoints
	r := chi.NewRouter()
	r.Use(chimiddleware.Recoverer)
	r.Use(api.SecurityHeaders)
	r.Use(api.CORS)
	r.Use(api.RequestLogger)

	// Public endpoints (no auth) — k8s-grade health checks
	startTime := time.Now()
	r.Get("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"version": Version,
			"commit":  Commit,
			"date":    Date,
			"go":      "go1.25",
		})
	})
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		uptime := time.Since(startTime).Round(time.Second)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":  "ok",
			"version": "1.0.0",
			"uptime":  uptime.String(),
		})
	})
	r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := database.DB.Ping(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]any{
				"status": "not_ready",
				"error":  err.Error(),
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	})
	r.Get("/live", api.HandleLiveness)
	r.Get("/metrics", metrics.Handler())

	// Authenticated routes on proxy router — hybrid auth (DB keys + legacy fallback)
	r.Group(func(r chi.Router) {
		// Google's SDKs carry the credential as x-goog-api-key or ?key=; the shim
		// runs ahead of the authenticator so those clients reach the same path.
		r.Use(api.GeminiAuthShim)
		r.Use(api.AuthKeyV2OrLegacy(database.DB, cfg.APIKey))
		r.Use(api.BillingGate(meter))
		if cfg.RateLimitEnabled {
			r.Use(rl.Middleware)
		}

		r.Post("/v1/chat/completions", proxyRouter.ServeChat)

		// Native client dialects — translated onto the same metered chat path so
		// billing, filters, caching and fallback see one shape.
		api.MountAnthropicIngress(r, proxyRouter)
		api.MountGeminiIngress(r, proxyRouter)

		// Embeddings, TTS, STT (OpenAI-compatible)
		api.MountEmbeddingsAPI(r, proxyRouter)
		api.MountTTSAPI(r, proxyRouter)
		api.MountSTTAPI(r, proxyRouter)

		// Image generation proxy (OpenAI-compatible)
		api.MountImagesProxyAPI(r, database, cfg.StorageDir)

		// Models list via MountModelsAPI (full knownModels map)
		api.MountModelsAPI(r, reg)

		// Cache stats + purge (proxy-level)
		r.Get("/api/cache/stats", func(w http.ResponseWriter, r *http.Request) {
			total, live := cache.Stats()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]int{"total": total, "live": live})
		})
		r.Post("/api/cache/purge", func(w http.ResponseWriter, r *http.Request) {
			cache.Purge()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "purged"})
		})

		// Backup on-demand
		r.Post("/api/backup", func(w http.ResponseWriter, r *http.Request) {
			path, err := db.Backup(cfg.DatabasePath, backupDir, cfg.BackupRetention)
			w.Header().Set("Content-Type", "application/json")
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"path": path})
		})

		// MITM Bridge API
		if cfg.MitmEnabled && mitmProxy != nil {
			mitmAPI := api.NewMITMAPI(mitmProxy, mitmDNS, mitmProxy.CA(), database)
			mitmAPI.RegisterRoutes(r)
		}

		// Tier quota status
		r.Get("/api/v1/tiers/quota-status", api.HandleTierQuotaStatus(database))

		// Services stats
		r.Get("/api/v1/services/stats", api.HandleServicesStats(database))
	})

	// JWT secret — must NOT be the default in production
	jwtSecretRaw := os.Getenv("JWT_SECRET")
	if jwtSecretRaw == "" {
		jwtSecretRaw = "switchblade-jwt-secret-change-in-production"
	}
	if jwtSecretRaw == "switchblade-jwt-secret-change-in-production" && os.Getenv("ALLOW_DEFAULT_SECRETS") != "true" {
		slog.Error("JWT_SECRET is still set to the default value. Set JWT_SECRET env var before starting, or set ALLOW_DEFAULT_SECRETS=true to bypass (not recommended for production)")
		os.Exit(1)
	}
	jwtSecret := []byte(jwtSecretRaw)

	// Enforce non-default secrets for critical keys
	if cfg.APIKey == "switchblade-secret" && os.Getenv("ALLOW_DEFAULT_SECRETS") != "true" {
		slog.Error("API_KEY is still set to the default value. Set API_KEY env var before starting, or set ALLOW_DEFAULT_SECRETS=true to bypass (not recommended for production)")
		os.Exit(1)
	}
	if cfg.EncryptionKey == "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6" && os.Getenv("ALLOW_DEFAULT_SECRETS") != "true" {
		slog.Error("ENCRYPTION_KEY is still set to the default value. Set ENCRYPTION_KEY env var before starting, or set ALLOW_DEFAULT_SECRETS=true to bypass (not recommended for production)")
		os.Exit(1)
	}

	// Dashboard router — port cfg.DashboardPort (1931)
	dash := chi.NewRouter()
	dash.Use(chimiddleware.Recoverer)
	dash.Use(api.SecurityHeaders)
	dash.Use(api.CORS)
	dash.Use(api.RequestLogger)

	// ── Public auth endpoints (no API key required) ─────────────────────
	api.MountAuthAPI(dash, api.AuthConfig{DB: database.DB, Hub: hub, JWTSecret: jwtSecret})

	// ── Authenticated routes (JWT or legacy API key) ───────────────────
	dash.Group(func(r chi.Router) {
		r.Use(api.AuthJWTOrAPIKey(jwtSecret, database.DB, cfg.APIKey))
		r.Use(api.LimitBodySize(1 << 20))

		// Management, keys, export, cache APIs
		api.MountManagementAPI(r, database)
		api.MountKeysAPI(r, database)
		api.MountExportAPI(r, database)
		api.MountCacheAPI(r, apiCache)
		api.MountRelayMgmtAPI(r, database)
		api.MountCombosAPI(r, database)
		api.MountModelMappingsAPI(r, database)
		api.MountProxyPoolAPI(r, database)
		api.MountVCCAPI(r, database)
		api.MountToolsAPI(r)
		api.MountProviderConfigAPI(r, database)
		api.MountDashboardModelsAPI(r, reg)
		api.MountImagesAPI(r, database, cfg.StorageDir)
		api.MountAdminAPI(r, database)
		api.MountBillingAPI(r, database.DB)

		// Tier management (owner CRUD)
		api.MountTierAPI(r, database)

		// Usage tracking + stats
		r.Get("/api/usage", api.HandleUsageStats(database.DB))
		r.Get("/api/usage/records", api.HandleListUsageRecords(database.DB))

		// API keys v2 (with model scoping)
		r.Post("/api/keys/v2", api.HandleCreateKeyV2(database.DB))
		r.Get("/api/keys/v2", api.HandleListKeysV2(database.DB))
		r.Delete("/api/keys/v2/{id}", api.HandleDeleteKeyV2(database.DB))
		r.Get("/api/keys/v2/lookup", api.HandleGetKeyByValue(database.DB))
	})

	// SSE events stream — auth-protected (token via query param for EventSource compat)
	dash.Get("/api/events", func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			// Fallback: try Authorization header
			authHeader := r.Header.Get("Authorization")
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				token = authHeader[7:]
			}
		}
		if token == "" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "authentication required"})
			return
		}
		claims, err := auth.VerifyJWT(token, jwtSecret)
		if err != nil || claims == nil {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid or expired token"})
			return
		}
		// Valid JWT — pass tenant_id to hub for tenant-scoped events
		r2 := r.WithContext(reqctx.WithTenant(r.Context(), claims.TenantID))
		hub.ServeSSE(w, r2)
	})

	// Metrics — auth-protected (sensitive internal data)
	dash.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token := authHeader[7:]
			if claims, err := auth.VerifyJWT(token, jwtSecret); err == nil && claims != nil {
				metrics.Handler().ServeHTTP(w, r)
				return
			}
		}
		// Fallback: allow API key auth too
		key := r.URL.Query().Get("key")
		if key != "" && key == cfg.APIKey {
			metrics.Handler().ServeHTTP(w, r)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "authentication required"})
	})

	// ── React SPA (production build) ────────────────────────────────────
	distDir := filepath.Join(".", "web", "dist")
	if _, err := os.Stat(distDir); err == nil {
		spa := http.FileServer(http.Dir(distDir))
		dash.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Serve static files directly, SPA index.html for all other routes
			if r.URL.Path != "/" && filepath.Ext(r.URL.Path) != "" {
				spa.ServeHTTP(w, r)
				return
			}
			http.ServeFile(w, r, filepath.Join(distDir, "index.html"))
		}))
		slog.Info(fmt.Sprintf("[spa] serving React SPA from %s", distDir))
	} else {
		// The legacy dashboard/ tree is gone; without a build there is nothing to
		// serve. Say so explicitly instead of handing out 404s from a missing dir.
		dash.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, "dashboard not built: run `npm --prefix web ci && npm --prefix web run build`,\n"+
				"then restart switchblade from the repository root (looked in %s)\n", distDir)
		}))
		slog.Info(fmt.Sprintf("[spa] %s not found — dashboard disabled; run `npm --prefix web run build` and start from the repo root", distDir))
	}

	// Relay wiring
	switch cfg.RelayMode {
	case "server":
		// Mount relay server on dashboard: forwards requests to the proxy router
		relayServer := relay.NewServer(r, cfg.RelaySecret)
		dash.Mount("/relay/forward", relayServer)
		slog.Info("[relay] server mode — /relay/forward → proxy")
	case "client":
		// ponytail: store client for optional use; not forced on all traffic
		_ = relay.NewClient(cfg.RelayServerURL, cfg.RelaySecret)
		slog.Info(fmt.Sprintf("[relay] client mode — relay target: %s", cfg.RelayServerURL))
	default:
		slog.Info("[relay] disabled")
	}

	// WriteTimeout must stay 0 on the proxy listener: it is an absolute deadline
	// from the start of the request, so any non-zero value silently truncates a
	// stream that runs longer than it. Request lifetime is bounded by
	// ProviderRequestTimeoutMS on the outbound context instead, and
	// ReadHeaderTimeout still covers slow-header attacks.
	srv := &http.Server{
		Addr:              ":" + itoa(cfg.Port),
		Handler:           r,
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	dashSrv := &http.Server{
		Addr:         ":" + itoa(cfg.DashboardPort),
		Handler:      dash,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("Switchblade listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen", "err", err)
			os.Exit(1)
		}
	}()

	go func() {
		slog.Info("Switchblade dashboard", "port", cfg.DashboardPort)
		if err := dashSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("dashboard listen", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down...")

	stop() // cancel context for all background goroutines
	scheduler.Stop()

	shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		slog.Error("shutdown", "err", err)
		os.Exit(1)
	}
	if err := dashSrv.Shutdown(shutCtx); err != nil {
		slog.Error("dashboard shutdown", "err", err)
		os.Exit(1)
	}
	slog.Info("stopped")
}

func itoa(n int) string {
	buf := [20]byte{}
	pos := len(buf)
	for n >= 10 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	pos--
	buf[pos] = byte('0' + n)
	return string(buf[pos:])
}

// seedDefaultOwner creates a default owner account (admin/123456) if no users exist.
// The password can be changed later via dashboard settings.
func seedDefaultOwner(database *sql.DB) {
	var count int
	err := database.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		slog.Error("[seed] check users failed", "err", err)
		return
	}
	if count > 0 {
		return
	}

	tx, err := database.Begin()
	if err != nil {
		slog.Error("[seed] begin tx failed", "err", err)
		return
	}
	defer tx.Rollback()

	// Create default tenant.
	tenantID := "tenant_default"
	now := time.Now().Unix()
	_, err = tx.Exec(
		`INSERT OR IGNORE INTO tenants (id, name, email, status, created_at, updated_at) VALUES (?, 'Default Org', 'admin@switchblade.local', 'active', ?, ?)`,
		tenantID, now, now,
	)
	if err != nil {
		slog.Error("[seed] create tenant failed", "err", err)
		return
	}

	// Hash password.
	hash, err := auth.HashPassword("123456")
	if err != nil {
		slog.Error("[seed] hash password failed", "err", err)
		return
	}

	// Create owner user.
	_, err = tx.Exec(
		`INSERT INTO users (id, tenant_id, username, password_hash, role, email, status, created_at) VALUES (?, ?, 'admin', ?, 'owner', 'admin@switchblade.local', 'active', ?)`,
		"user_admin", tenantID, hash, now,
	)
	if err != nil {
		slog.Error("[seed] create admin user failed", "err", err)
		return
	}

	if err := tx.Commit(); err != nil {
		slog.Error("[seed] commit failed", "err", err)
		return
	}

	slog.Info("[seed] default owner created — username: admin, password: 123456")
}
