package ssrf

import "testing"

func TestIsInternalIP(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		// IPv4 private / reserved
		{"127.0.0.1", true},
		{"127.9.9.9", true},
		{"10.1.2.3", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.1.1", true},
		{"169.254.169.254", true}, // cloud metadata endpoint
		{"0.0.0.0", true},
		// IPv6
		{"::1", true},
		{"::", true},
		{"::ffff:10.0.0.1", true}, // IPv4-mapped
		{"::ffff:127.0.0.1", true},
		// Public — must not be flagged
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"172.32.0.1", false},  // just outside 172.16/12
		{"192.169.0.1", false}, // just outside 192.168/16
		// Not an IP at all — hostname, resolved later by ValidateURL
		{"example.com", false},
		{"localhost", false},
		// host:port form
		{"127.0.0.1:8080", true},
		{"10.0.0.1:443", true},
		{"8.8.8.8:53", false},
	}
	for _, c := range cases {
		if got := IsInternalIP(c.host); got != c.want {
			t.Errorf("IsInternalIP(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestValidateURLInternalBlocked(t *testing.T) {
	blocked := []string{
		"http://127.0.0.1:1930/health",
		"http://10.0.0.1/admin",
		"http://192.168.1.100/router",
		"http://169.254.169.254/latest/meta-data/",
		"http://[::1]:8080/",
		"http://0.0.0.0/",
		"http://user@10.1.2.3:8080/path",
	}
	for _, u := range blocked {
		if err := ValidateURL(u); err == nil {
			t.Errorf("ValidateURL(%q) = nil, want error (internal address)", u)
		}
	}
}

func TestValidateURLPublicIPAllowed(t *testing.T) {
	// IP literals: no DNS lookup needed, safe to assert offline.
	allowed := []string{
		"http://8.8.8.8/dns-query",
		"https://1.1.1.1/",
		"http://192.0.2.1/", // TEST-NET-1 — public range, not internal
	}
	for _, u := range allowed {
		if err := ValidateURL(u); err != nil {
			t.Errorf("ValidateURL(%q) = %v, want nil", u, err)
		}
	}
}

func TestValidateURLInvalidInput(t *testing.T) {
	cases := []struct {
		in string
	}{
		{"/relative/path"},          // no host
		{"://missing-scheme"},       // unparsable
		{"http://no-host.invalid/"}, // .invalid TLD never resolves
	}
	for _, c := range cases {
		if err := ValidateURL(c.in); err == nil {
			t.Errorf("ValidateURL(%q) = nil, want error", c.in)
		}
	}
}

// Localhost resolves to a loopback address, so it must be blocked even
// though the bare hostname is not itself an IP.
func TestValidateURLLocalhostBlocked(t *testing.T) {
	if err := ValidateURL("http://localhost:1930/api"); err == nil {
		t.Error("ValidateURL(localhost) = nil, want error (resolves to loopback)")
	}
}
