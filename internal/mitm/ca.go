package mitm

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// CA holds the root certificate authority used to sign per-domain certs.
type CA struct {
	Cert    *x509.Certificate
	Key     *rsa.PrivateKey
	CertPEM []byte
	KeyPEM  []byte
}

// LoadOrGenerateCA loads the CA from disk or generates a new one.
func LoadOrGenerateCA(certPath, keyPath string) (*CA, error) {
	if cert, key, err := loadCAFromDisk(certPath, keyPath); err == nil {
		log.Printf("[mitm/ca] loaded existing CA from %s", certPath)
		return &CA{
			Cert:    cert,
			Key:     key,
			CertPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}),
			KeyPEM:  pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}),
		}, nil
	}

	log.Printf("[mitm/ca] generating new root CA")
	ca, err := generateCA()
	if err != nil {
		return nil, err
	}

	// Persist to disk
	if err := os.MkdirAll(filepath.Dir(certPath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir cert dir: %w", err)
	}
	if err := os.WriteFile(certPath, ca.CertPEM, 0o644); err != nil {
		return nil, fmt.Errorf("write ca cert: %w", err)
	}
	if err := os.WriteFile(keyPath, ca.KeyPEM, 0o600); err != nil {
		return nil, fmt.Errorf("write ca key: %w", err)
	}

	return ca, nil
}

func loadCAFromDisk(certPath, keyPath string) (*x509.Certificate, *rsa.PrivateKey, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, nil, err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, err
	}

	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, nil, fmt.Errorf("failed to decode CA cert PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse ca cert: %w", err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("failed to decode CA key PEM")
	}
	key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse ca key: %w", err)
	}

	return cert, key, nil
}

func generateCA() (*CA, error) {
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, fmt.Errorf("generate ca key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"Switchblade MITM"},
			CommonName:   "Switchblade Root CA",
		},
		NotBefore:             time.Now().Add(-24 * time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create ca cert: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("parse ca cert: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	return &CA{
		Cert:    cert,
		Key:     key,
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
	}, nil
}

// InstallSystemTrust installs the CA cert into the system trust store.
func (ca *CA) InstallSystemTrust() error {
	// Write cert to temp file
	tmp, err := os.CreateTemp("", "switchblade-ca-*.crt")
	if err != nil {
		return fmt.Errorf("temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(ca.CertPEM); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp cert: %w", err)
	}
	tmp.Close()

	switch runtime.GOOS {
	case "windows":
		// certutil -addstore -user ROOT <cert>
		cmd := exec.Command("certutil", "-addstore", "-user", "ROOT", tmp.Name())
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("certutil: %w: %s", err, string(out))
		}
	case "linux":
		// Copy to /usr/local/share/ca-certificates and run update-ca-certificates
		dest := "/usr/local/share/ca-certificates/switchblade-ca.crt"
		if err := os.WriteFile(dest, ca.CertPEM, 0o644); err != nil {
			return fmt.Errorf("copy to ca-certificates: %w", err)
		}
		cmd := exec.Command("update-ca-certificates")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("update-ca-certificates: %w: %s", err, string(out))
		}
	default:
		return fmt.Errorf("unsupported OS for system trust install: %s", runtime.GOOS)
	}

	log.Printf("[mitm/ca] installed root CA into system trust store")
	return nil
}

// UninstallSystemTrust removes the CA cert from the system trust store.
func UninstallSystemTrust() error {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("certutil", "-delstore", "-user", "ROOT", "Switchblade Root CA")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("certutil del: %w: %s", err, string(out))
		}
	case "linux":
		dest := "/usr/local/share/ca-certificates/switchblade-ca.crt"
		os.Remove(dest)
		cmd := exec.Command("update-ca-certificates", "--fresh")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("update-ca-certificates: %w: %s", err, string(out))
		}
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	log.Printf("[mitm/ca] removed root CA from system trust store")
	return nil
}
