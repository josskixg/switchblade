package mitm

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"switchblade/internal/db"
	"switchblade/internal/mitm/handlers"
	"switchblade/internal/proxy"
)

// Proxy is a TLS-intercepting HTTPS proxy that intercepts IDE traffic
// and routes it through the account pool.
type Proxy struct {
	cfg      Config
	ca       *CA
	certs    *CertStore
	registry *handlers.Registry
	pools    map[string]*proxy.AccountPool
	db       *db.DB
	server   *http.Server
	mu       sync.Mutex
	running  bool
}

// NewProxy creates a new MITM proxy server.
func NewProxy(cfg Config, ca *CA, database *db.DB, pools map[string]*proxy.AccountPool, registry *handlers.Registry) *Proxy {
	return &Proxy{
		cfg:      cfg,
		ca:       ca,
		certs:    NewCertStore(ca, database.DB),
		registry: registry,
		pools:    pools,
		db:       database,
	}
}

// Start begins listening for HTTPS connections.
func (p *Proxy) Start() error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return fmt.Errorf("proxy already running")
	}
	p.mu.Unlock()

	addr := fmt.Sprintf(":%d", p.cfg.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	p.server = &http.Server{
		Handler: p,
	}

	p.mu.Lock()
	p.running = true
	p.mu.Unlock()

	log.Printf("[mitm] proxy listening on %s", addr)

	go func() {
		if err := p.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("[mitm] server error: %v", err)
			p.mu.Lock()
			p.running = false
			p.mu.Unlock()
		}
	}()

	return nil
}

// ServeHTTP handles CONNECT requests for HTTPS tunneling.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}
	// Non-CONNECT requests — just pass through
	http.Error(w, "only CONNECT supported", http.StatusMethodNotAllowed)
}

func (p *Proxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if !strings.Contains(host, ":") {
		host += ":443"
	}

	// Hijack the connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack not supported", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		log.Printf("[mitm] hijack failed: %v", err)
		return
	}

	// Send 200 Connection Established
	if _, err := fmt.Fprintf(clientConn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		clientConn.Close()
		return
	}

	// Extract hostname for TLS cert
	hostname := strings.Split(host, ":")[0]

	// Get or generate TLS cert for this domain
	tlsCert, err := p.certs.GetCert(hostname)
	if err != nil {
		log.Printf("[mitm] cert for %s: %v", hostname, err)
		clientConn.Close()
		return
	}

	// TLS handshake with the client (IDE)
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*tlsCert},
	}
	tlsConn := tls.Server(clientConn, tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		log.Printf("[mitm] tls handshake %s: %v", hostname, err)
		clientConn.Close()
		return
	}

	// Read HTTP requests from the TLS connection
	go p.handleTLSConn(tlsConn, hostname)
}

func (p *Proxy) handleTLSConn(conn *tls.Conn, hostname string) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	for {
		req, err := http.ReadRequest(reader)
		if err != nil {
			if err != io.EOF {
				log.Printf("[mitm] read request from %s: %v", hostname, err)
			}
			return
		}

		// Set the host header
		req.Host = hostname
		req.URL.Host = hostname
		req.URL.Scheme = "https"
		req.RequestURI = ""

		// Find matching handler
		handler := p.registry.Match(req)
		if handler == nil {
			// No handler — forward as-is to the real server
			p.passthrough(conn, req, hostname)
			continue
		}

		// Intercept and rewrite
		p.intercept(conn, req, handler)
	}
}

func (p *Proxy) intercept(clientConn *tls.Conn, req *http.Request, h handlers.Handler) {
	// Parse the request to get provider and model
	provider, model, err := h.Parse(req)
	if err != nil {
		log.Printf("[mitm] parse %s: %v", h.Name(), err)
		p.writeError(clientConn, http.StatusBadRequest, "parse error")
		return
	}

	// Pick an account from the pool
	pool, ok := p.pools[provider]
	if !ok {
		log.Printf("[mitm] no pool for provider %s", provider)
		p.writeError(clientConn, http.StatusBadGateway, "no pool for provider")
		return
	}

	acc, err := pool.Pick(provider)
	if err != nil {
		log.Printf("[mitm] pick account for %s: %v", provider, err)
		p.writeError(clientConn, http.StatusServiceUnavailable, "no accounts available")
		return
	}
	defer pool.MarkDone(acc.ID, nil)

	// Rewrite the request with account credentials
	if err := h.Rewrite(req, acc); err != nil {
		log.Printf("[mitm] rewrite %s: %v", h.Name(), err)
		p.writeError(clientConn, http.StatusInternalServerError, "rewrite error")
		return
	}

	// Forward to the real upstream
	resp, err := p.forwardRequest(req, hostname(req))
	if err != nil {
		log.Printf("[mitm] forward %s: %v", hostname(req), err)
		p.writeError(clientConn, http.StatusBadGateway, "upstream error")
		return
	}
	defer resp.Body.Close()

	// Write response back to client
	if err := resp.Write(clientConn); err != nil {
		log.Printf("[mitm] write response: %v", err)
	}

	// Log session
	p.logSession(h.Name(), provider, acc.ID, req.Method, req.URL.Path, resp.StatusCode, 0)

	_ = model // model info available for future routing decisions
}

func (p *Proxy) passthrough(clientConn *tls.Conn, req *http.Request, hostname string) {
	resp, err := p.forwardRequest(req, hostname)
	if err != nil {
		log.Printf("[mitm] passthrough %s: %v", hostname, err)
		p.writeError(clientConn, http.StatusBadGateway, "upstream error")
		return
	}
	defer resp.Body.Close()

	if err := resp.Write(clientConn); err != nil {
		log.Printf("[mitm] write passthrough response: %v", err)
	}
}

func (p *Proxy) forwardRequest(req *http.Request, targetHost string) (*http.Response, error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
		DialContext: (&net.Dialer{
			Timeout: 30 * time.Second,
		}).DialContext,
	}
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Transport: transport,
		Timeout:   120 * time.Second,
	}

	// Ensure the request goes to the real server
	req.URL.Host = targetHost
	req.RequestURI = ""

	return client.Do(req)
}

func (p *Proxy) writeError(conn *tls.Conn, code int, msg string) {
	resp := &http.Response{
		StatusCode: code,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     http.Header{"Content-Type": {"text/plain"}},
		Body:       io.NopCloser(strings.NewReader(msg)),
	}
	resp.Write(conn)
}

func (p *Proxy) logSession(ide, provider string, accountID int64, method, path string, statusCode int, bytesSent int64) {
	if p.db == nil {
		return
	}
	_, err := p.db.Exec(
		`INSERT INTO mitm_sessions (ide, provider, account_id, method, path, status_code, bytes_sent) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ide, provider, accountID, method, path, statusCode, bytesSent,
	)
	if err != nil {
		log.Printf("[mitm] log session: %v", err)
	}
}

// Stop gracefully shuts down the proxy.
func (p *Proxy) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		return nil
	}
	p.running = false

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return p.server.Shutdown(ctx)
}

// IsRunning returns whether the proxy is currently active.
func (p *Proxy) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.running
}

// CACertPEM returns the PEM-encoded CA certificate.
func (p *Proxy) CACertPEM() []byte {
	return p.ca.CertPEM
}

// CA returns the CA struct for API access.
func (p *Proxy) CA() *CA {
	return p.ca
}

// hostname extracts the hostname from a request.
func hostname(req *http.Request) string {
	if req.URL.Host != "" {
		return req.URL.Host
	}
	return req.Host
}
