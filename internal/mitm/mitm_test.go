package mitm

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"switchblade/internal/db"
	"switchblade/internal/mitm/handlers"
)

func testDB(t *testing.T) *db.DB {
	t.Helper()
	raw, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { raw.Close() })
	if err := raw.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return raw
}

func TestCA_GenerateAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	caKey := filepath.Join(tmpDir, "ca.key")
	caCert := filepath.Join(tmpDir, "ca.crt")

	// Generate new CA
	ca, err := LoadOrGenerateCA(caCert, caKey)
	if err != nil {
		t.Fatalf("LoadOrGenerateCA: %v", err)
	}
	if ca == nil {
		t.Fatal("CA is nil")
	}
	if len(ca.CertPEM) == 0 {
		t.Fatal("CA CertPEM is empty")
	}
	if len(ca.KeyPEM) == 0 {
		t.Fatal("CA KeyPEM is empty")
	}

	// Verify files were created
	if _, err := os.Stat(caKey); os.IsNotExist(err) {
		t.Fatal("CA key file not created")
	}
	if _, err := os.Stat(caCert); os.IsNotExist(err) {
		t.Fatal("CA cert file not created")
	}

	// Load existing CA
	ca2, err := LoadOrGenerateCA(caCert, caKey)
	if err != nil {
		t.Fatalf("LoadOrGenerateCA (load): %v", err)
	}
	if string(ca2.CertPEM) != string(ca.CertPEM) {
		t.Fatal("Loaded CA cert doesn't match generated cert")
	}
}

func TestCertStore_GenerateCertificate(t *testing.T) {
	tmpDir := t.TempDir()
	database := testDB(t)

	ca, err := LoadOrGenerateCA(filepath.Join(tmpDir, "ca.crt"), filepath.Join(tmpDir, "ca.key"))
	if err != nil {
		t.Fatalf("LoadOrGenerateCA: %v", err)
	}

	store := NewCertStore(ca, database.DB)

	// Generate certificate for a domain
	cert, err := store.GetCert("api.openai.com")
	if err != nil {
		t.Fatalf("GetCert: %v", err)
	}
	if cert == nil {
		t.Fatal("Certificate is nil")
	}

	// Verify the certificate is valid
	if len(cert.Certificate) == 0 {
		t.Fatal("Certificate has no data")
	}
}

func TestCertStore_GetCert(t *testing.T) {
	tmpDir := t.TempDir()
	database := testDB(t)

	ca, err := LoadOrGenerateCA(filepath.Join(tmpDir, "ca.crt"), filepath.Join(tmpDir, "ca.key"))
	if err != nil {
		t.Fatalf("LoadOrGenerateCA: %v", err)
	}

	store := NewCertStore(ca, database.DB)

	// Get cert for first time (should generate)
	cert1, err := store.GetCert("api.openai.com")
	if err != nil {
		t.Fatalf("GetCert (first): %v", err)
	}
	if cert1 == nil {
		t.Fatal("Cert is nil")
	}

	// Get cert again (should return cached)
	cert2, err := store.GetCert("api.openai.com")
	if err != nil {
		t.Fatalf("GetCert (cached): %v", err)
	}
	if cert2 == nil {
		t.Fatal("Cached cert is nil")
	}

	// Get cert for different domain
	cert3, err := store.GetCert("api.anthropic.com")
	if err != nil {
		t.Fatalf("GetCert (different): %v", err)
	}
	if cert3 == nil {
		t.Fatal("Different domain cert is nil")
	}
}

func TestProxy_StartStop(t *testing.T) {
	tmpDir := t.TempDir()
	database := testDB(t)

	ca, err := LoadOrGenerateCA(filepath.Join(tmpDir, "ca.crt"), filepath.Join(tmpDir, "ca.key"))
	if err != nil {
		t.Fatalf("LoadOrGenerateCA: %v", err)
	}

	cfg := Config{
		Enabled: true,
		Port:    0, // Let OS assign port
		CertDir: tmpDir,
		CAKey:   filepath.Join(tmpDir, "ca.key"),
		CACert:  filepath.Join(tmpDir, "ca.crt"),
	}

	registry := handlers.NewRegistry()
	proxy := NewProxy(cfg, ca, database, nil, registry)

	// Start proxy
	if err := proxy.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !proxy.IsRunning() {
		t.Fatal("Proxy not running after Start")
	}

	// Stop proxy
	if err := proxy.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if proxy.IsRunning() {
		t.Fatal("Proxy still running after Stop")
	}
}

func TestProxy_CACertPEM(t *testing.T) {
	tmpDir := t.TempDir()
	database := testDB(t)

	ca, err := LoadOrGenerateCA(filepath.Join(tmpDir, "ca.crt"), filepath.Join(tmpDir, "ca.key"))
	if err != nil {
		t.Fatalf("LoadOrGenerateCA: %v", err)
	}

	cfg := Config{
		Enabled: true,
		Port:    0,
		CertDir: tmpDir,
		CAKey:   filepath.Join(tmpDir, "ca.key"),
		CACert:  filepath.Join(tmpDir, "ca.crt"),
	}

	registry := handlers.NewRegistry()
	proxy := NewProxy(cfg, ca, database, nil, registry)

	pem := proxy.CACertPEM()
	if len(pem) == 0 {
		t.Fatal("CACertPEM is empty")
	}
}

func TestProxy_HandleConnect(t *testing.T) {
	tmpDir := t.TempDir()
	database := testDB(t)

	ca, err := LoadOrGenerateCA(filepath.Join(tmpDir, "ca.crt"), filepath.Join(tmpDir, "ca.key"))
	if err != nil {
		t.Fatalf("LoadOrGenerateCA: %v", err)
	}

	cfg := Config{
		Enabled: true,
		Port:    0,
		CertDir: tmpDir,
		CAKey:   filepath.Join(tmpDir, "ca.key"),
		CACert:  filepath.Join(tmpDir, "ca.crt"),
	}

	registry := handlers.NewRegistry()
	proxy := NewProxy(cfg, ca, database, nil, registry)

	// Create a test CONNECT request
	req := httptest.NewRequest(http.MethodConnect, "https://api.openai.com:443", nil)
	w := httptest.NewRecorder()

	// This will fail because we can't hijack httptest.ResponseRecorder,
	// but we can verify the proxy handles the request
	proxy.ServeHTTP(w, req)

	// The proxy should attempt to hijack the connection
	// Since httptest doesn't support hijacking, we just verify it doesn't panic
}

func TestProxy_TLSConfig(t *testing.T) {
	tmpDir := t.TempDir()
	database := testDB(t)

	ca, err := LoadOrGenerateCA(filepath.Join(tmpDir, "ca.crt"), filepath.Join(tmpDir, "ca.key"))
	if err != nil {
		t.Fatalf("LoadOrGenerateCA: %v", err)
	}

	cfg := Config{
		Enabled: true,
		Port:    0,
		CertDir: tmpDir,
		CAKey:   filepath.Join(tmpDir, "ca.key"),
		CACert:  filepath.Join(tmpDir, "ca.crt"),
	}

	registry := handlers.NewRegistry()
	proxy := NewProxy(cfg, ca, database, nil, registry)

	// Verify proxy can generate TLS config
	cert, err := proxy.certs.GetCert("test.example.com")
	if err != nil {
		t.Fatalf("GetCert: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*cert},
	}
	if len(tlsConfig.Certificates) == 0 {
		t.Fatal("TLS config has no certificates")
	}
}
