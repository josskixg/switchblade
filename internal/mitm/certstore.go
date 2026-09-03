package mitm

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"sync"
	"time"
)

// CertStore generates and caches per-domain TLS certificates signed by the CA.
type CertStore struct {
	mu    sync.RWMutex
	cache map[string]*tls.Certificate // domain -> cert
	ca    *CA
	db    *sql.DB
}

// NewCertStore creates a cert store backed by the given CA and optional DB.
func NewCertStore(ca *CA, database *sql.DB) *CertStore {
	cs := &CertStore{
		cache: make(map[string]*tls.Certificate),
		ca:    ca,
		db:    database,
	}
	cs.loadFromDB()
	return cs
}

// GetCert returns a TLS certificate for the given domain, generating one if needed.
func (cs *CertStore) GetCert(domain string) (*tls.Certificate, error) {
	// Fast path: in-memory cache
	cs.mu.RLock()
	if cert, ok := cs.cache[domain]; ok {
		cs.mu.RUnlock()
		return cert, nil
	}
	cs.mu.RUnlock()

	// Slow path: generate
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Double-check after acquiring write lock
	if cert, ok := cs.cache[domain]; ok {
		return cert, nil
	}

	cert, err := cs.generateAndCache(domain)
	if err != nil {
		return nil, err
	}
	cs.cache[domain] = cert
	return cert, nil
}

func (cs *CertStore) generateAndCache(domain string) (*tls.Certificate, error) {
	// Try DB first
	if cert, err := cs.loadFromDBDomain(domain); err == nil {
		return cert, nil
	}

	// Generate new cert
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate key for %s: %w", domain, err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"Switchblade MITM"},
			CommonName:   domain,
		},
		DNSNames:              []string{domain},
		NotBefore:             time.Now().Add(-24 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, cs.ca.Cert, &key.PublicKey, cs.ca.Key)
	if err != nil {
		return nil, fmt.Errorf("create cert for %s: %w", domain, err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse tls cert for %s: %w", domain, err)
	}

	// Persist to DB
	if cs.db != nil {
		expiresAt := time.Now().Add(365 * 24 * time.Hour).Unix()
		_, err = cs.db.Exec(
			`INSERT OR REPLACE INTO mitm_certs (domain, cert_pem, key_pem, expires_at) VALUES (?, ?, ?, ?)`,
			domain, certPEM, keyPEM, expiresAt,
		)
		if err != nil {
			slog.Info(fmt.Sprintf("[mitm/certstore] db persist %s: %v", domain, err))
		}
	}

	slog.Info(fmt.Sprintf("[mitm/certstore] generated cert for %s", domain))
	return &tlsCert, nil
}

func (cs *CertStore) loadFromDBDomain(domain string) (*tls.Certificate, error) {
	if cs.db == nil {
		return nil, fmt.Errorf("no db")
	}

	var certPEM, keyPEM []byte
	var expiresAt int64
	err := cs.db.QueryRow(
		`SELECT cert_pem, key_pem, expires_at FROM mitm_certs WHERE domain = ? AND expires_at > ?`,
		domain, time.Now().Unix(),
	).Scan(&certPEM, &keyPEM, &expiresAt)
	if err != nil {
		return nil, err
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &cert, nil
}

func (cs *CertStore) loadFromDB() {
	if cs.db == nil {
		return
	}

	rows, err := cs.db.Query(
		`SELECT domain, cert_pem, key_pem, expires_at FROM mitm_certs WHERE expires_at > ?`,
		time.Now().Unix(),
	)
	if err != nil {
		slog.Warn("[mitm/certstore] load from db", "err", err)
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var domain string
		var certPEM, keyPEM []byte
		var expiresAt int64
		if err := rows.Scan(&domain, &certPEM, &keyPEM, &expiresAt); err != nil {
			slog.Warn("[mitm/certstore] scan row", "err", err)
			continue
		}
		cert, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			slog.Info(fmt.Sprintf("[mitm/certstore] parse cert for %s: %v", domain, err))
			continue
		}
		cs.cache[domain] = &cert
		count++
	}
	if count > 0 {
		slog.Info(fmt.Sprintf("[mitm/certstore] loaded %d certs from DB", count))
	}
}
