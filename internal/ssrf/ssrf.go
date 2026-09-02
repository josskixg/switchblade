// Package ssrf provides URL validation against internal/private IP ranges.
package ssrf

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"
)

var internalCIDRs = []string{
	"127.0.0.0/8",    // IPv4 loopback
	"::1/128",        // IPv6 loopback
	"10.0.0.0/8",     // RFC 1918
	"172.16.0.0/12",  // RFC 1918
	"192.168.0.0/16", // RFC 1918
	"169.254.0.0/16", // link-local (incl. cloud metadata)
	"0.0.0.0/8",      // "this" network
	"::/128",         // IPv6 unspecified
	"fc00::/7",       // IPv6 unique-local
	"fe80::/10",      // IPv6 link-local
	// NOTE: no "::ffff:0:0/96" — net.IPNet.Contains reduces the network to
	// its To4 form where the /96 mask truncates to all-zero bits, matching
	// every IPv4 address. IPv4-mapped internal hosts are already caught by
	// the RFC 1918/loopback ranges via To4 normalization of the candidate IP.
}

var dnsResolver = &net.Resolver{
	PreferGo: true,
	Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		d := net.Dialer{Timeout: 5 * time.Second}
		return d.DialContext(ctx, network, address)
	},
}

// IsInternalIP returns true if host is an internal/private address.
func IsInternalIP(host string) bool {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}

	ip := net.ParseIP(h)
	if ip == nil {
		return false
	}

	for _, cidr := range internalCIDRs {
		_, netw, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if netw.Contains(ip) {
			return true
		}
	}
	return false
}

func resolveHost(host string) ([]net.IP, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	addrs, err := dnsResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}
	return ips, nil
}

// ValidateURL returns nil if rawURL is safe to fetch (not pointing to internal/private IPs).
func ValidateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL has no host")
	}

	if IsInternalIP(host) {
		return fmt.Errorf("URL points to an internal address and is not allowed")
	}

	addrs, err := resolveHost(host)
	if err != nil {
		return fmt.Errorf("cannot resolve host %q: %w", host, err)
	}

	for _, ip := range addrs {
		if IsInternalIP(ip.String()) {
			return fmt.Errorf("URL resolves to an internal address (%s) and is not allowed", ip)
		}
	}

	return nil
}
