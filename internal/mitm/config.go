// Package mitm implements a TLS-intercepting proxy for IDE traffic.
package mitm

// Config holds MITM bridge configuration, typically populated from config.Config.
type Config struct {
	Enabled bool
	Port    int
	CertDir string
	CAKey   string
	CACert  string
}
