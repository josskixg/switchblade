package config

import (
	"os"
	"strconv"
)

// C is the global config singleton. Call Load() to initialize.
var C *Config

// Config holds all environment-variable-driven configuration.
type Config struct {
	// Server
	Port          int
	DashboardPort int
	APIKey        string

	// Security
	EncryptionKey string

	// Database
	DatabasePath string

	// Auth / Python
	AuthScriptPath string
	PythonPath     string
	AuthScriptCWD  string
	Headless       bool

	// Logging
	LogBodyEnabled  bool
	LogBodyFull     bool
	LogBodyRedact   bool
	LogBodyMaxBytes int

	// Timeouts & caching
	AccountCacheTTLMS        int
	AuthProcessTimeoutMS     int
	ProviderRequestTimeoutMS int
	ProviderQuotaTimeoutMS   int

	// Providers
	KiroProUpgrade bool
	BrowserEngine  string

	// Relay
	RelayMode          string
	RelayServerURL     string
	RelaySecret        string
	RelayPeerName      string
	RelayPublicBaseURL string
	RelayMaxTunnels    int
	RelayAutoStart     bool

	// Captcha
	CaptchaService string
	CaptchaAPIKey  string

	// Storage
	StorageDir string

	// Retention
	LogRetentionDays      int
	UsageRetentionDays    int
	ImageRetentionDays    int
	BackupIntervalMinutes int
	BackupRetention       int

	// Fallback
	FallbackEnabled     bool
	FallbackMaxAttempts int
	FallbackTimeoutMS   int

	// Rate limiting
	RateLimitEnabled bool
	RateLimitDefault int
	RateLimitBurst   int

	// Cache
	CacheEnabled       bool
	CacheDefaultTTLSec int

	// MITM Bridge
	MitmEnabled bool
	MitmPort    int
	MitmCertDir string
	MitmCAKey   string
	MitmCACert  string
}

// Load reads env vars, applies defaults, sets the global C, and returns it.
func Load() *Config {
	c := &Config{
		Port:                     envInt("PORT", 1930),
		DashboardPort:            envInt("DASHBOARD_PORT", 1931),
		APIKey:                   envStr("API_KEY", "switchblade-secret"),
		EncryptionKey:            envStr("ENCRYPTION_KEY", ""),
		DatabasePath:             envStr("DATABASE_PATH", "data/switchblade.db"),
		AuthScriptPath:           envStr("AUTH_SCRIPT_PATH", "scripts/auth/login.py"),
		PythonPath:               envStr("PYTHON_PATH", ".venv/Scripts/python.exe"),
		AuthScriptCWD:            envStr("AUTH_SCRIPT_CWD", "scripts/auth/"),
		Headless:                 envBool("HEADLESS", true),
		LogBodyEnabled:           envBool("POOLPROX_LOG_BODY_ENABLED", true),
		LogBodyFull:              envBool("POOLPROX_LOG_BODY_FULL", true),
		LogBodyRedact:            envBool("POOLPROX_LOG_BODY_REDACT", false),
		LogBodyMaxBytes:          envInt("POOLPROX_LOG_BODY_MAX_BYTES", 65536),
		AccountCacheTTLMS:        envInt("POOLPROX_ACCOUNT_CACHE_TTL_MS", 3000),
		AuthProcessTimeoutMS:     envInt("POOLPROX_AUTH_PROCESS_TIMEOUT_MS", 600000),
		ProviderRequestTimeoutMS: envInt("POOLPROX_PROVIDER_REQUEST_TIMEOUT_MS", 120000),
		ProviderQuotaTimeoutMS:   envInt("POOLPROX_PROVIDER_QUOTA_TIMEOUT_MS", 15000),
		KiroProUpgrade:           envBool("KIRO_PRO_UPGRADE", true),
		BrowserEngine:            envStr("BROWSER_ENGINE", "camoufox"),
		RelayMode:                envStr("RELAY_MODE", "disabled"),
		RelayServerURL:           envStr("RELAY_SERVER_URL", ""),
		RelaySecret:              envStr("RELAY_SECRET", ""),
		RelayPeerName:            envStr("RELAY_PEER_NAME", ""),
		RelayPublicBaseURL:       envStr("RELAY_PUBLIC_BASE_URL", ""),
		RelayMaxTunnels:          envInt("RELAY_MAX_TUNNELS", 50),
		RelayAutoStart:           envBool("RELAY_AUTO_START", false),
		CaptchaService:           envStr("CAPTCHA_SERVICE", "none"),
		CaptchaAPIKey:            envStr("CAPTCHA_API_KEY", ""),
		StorageDir:               envStr("STORAGE_DIR", "data/images"),
		LogRetentionDays:         envInt("LOG_RETENTION_DAYS", 30),
		UsageRetentionDays:       envInt("USAGE_RETENTION_DAYS", 90),
		ImageRetentionDays:       envInt("IMAGE_RETENTION_DAYS", 0),
		BackupIntervalMinutes:    envInt("BACKUP_INTERVAL_MINUTES", 15),
		BackupRetention:          envInt("BACKUP_RETENTION", 10),
		FallbackEnabled:          envBool("FALLBACK_ENABLED", true),
		FallbackMaxAttempts:      envInt("FALLBACK_MAX_ATTEMPTS", 3),
		FallbackTimeoutMS:        envInt("FALLBACK_TIMEOUT_MS", 60000),
		RateLimitEnabled:         envBool("RATE_LIMIT_ENABLED", true),
		RateLimitDefault:         envInt("RATE_LIMIT_DEFAULT", 100),
		RateLimitBurst:           envInt("RATE_LIMIT_BURST", 20),
		CacheEnabled:             envBool("CACHE_ENABLED", true),
		CacheDefaultTTLSec:       envInt("CACHE_DEFAULT_TTL_SEC", 300),

		// MITM Bridge
		MitmEnabled: envBool("MITM_ENABLED", false),
		MitmPort:    envInt("MITM_PORT", 443),
		MitmCertDir: envStr("MITM_CERT_DIR", "data/certs"),
		MitmCAKey:   envStr("MITM_CA_KEY", "data/certs/ca.key"),
		MitmCACert:  envStr("MITM_CA_CERT", "data/certs/ca.crt"),
	}

	if c.EncryptionKey == "" {
		panic("ENCRYPTION_KEY environment variable is required")
	}

	C = c
	return c
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}
