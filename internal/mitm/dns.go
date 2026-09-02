package mitm

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
)

const markerStart = "# >>> switchblade mitm >>>"
const markerEnd = "# <<< switchblade mitm <<<"

// DNSHijacker manipulates the system hosts file to redirect IDE domains
// to the local MITM proxy.
type DNSHijacker struct {
	mu      sync.Mutex
	entries map[string]string // domain -> IP
}

// NewDNSHijacker creates a new DNS hijacker.
func NewDNSHijacker() *DNSHijacker {
	return &DNSHijacker{
		entries: make(map[string]string),
	}
}

// Install adds hosts file entries redirecting domains to 127.0.0.1.
func (h *DNSHijacker) Install(domains []string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	hostsPath := hostsFilePath()

	// Read existing hosts file
	lines, err := readLines(hostsPath)
	if err != nil {
		return fmt.Errorf("read hosts file: %w", err)
	}

	// Remove any existing switchblade block
	lines = removeBlock(lines)

	// Build new block
	var block []string
	block = append(block, markerStart)
	for _, domain := range domains {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		block = append(block, fmt.Sprintf("127.0.0.1\t%s", domain))
		h.entries[domain] = "127.0.0.1"
	}
	block = append(block, markerEnd)

	// Append block
	lines = append(lines, block...)

	if err := writeLines(hostsPath, lines); err != nil {
		return fmt.Errorf("write hosts file: %w", err)
	}

	log.Printf("[mitm/dns] installed %d hosts entries", len(domains))
	return nil
}

// Uninstall removes the switchblade block from the hosts file.
func (h *DNSHijacker) Uninstall() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	hostsPath := hostsFilePath()

	lines, err := readLines(hostsPath)
	if err != nil {
		return fmt.Errorf("read hosts file: %w", err)
	}

	lines = removeBlock(lines)

	if err := writeLines(hostsPath, lines); err != nil {
		return fmt.Errorf("write hosts file: %w", err)
	}

	h.entries = make(map[string]string)
	log.Printf("[mitm/dns] uninstalled hosts entries")
	return nil
}

// IsInstalled returns whether any switchblade hosts entries exist.
func (h *DNSHijacker) IsInstalled() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.entries) > 0
}

func hostsFilePath() string {
	if runtime.GOOS == "windows" {
		return `C:\Windows\System32\drivers\etc\hosts`
	}
	return "/etc/hosts"
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func writeLines(path string, lines []string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, line := range lines {
		fmt.Fprintln(w, line)
	}
	return w.Flush()
}

func removeBlock(lines []string) []string {
	var result []string
	inBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == markerStart {
			inBlock = true
			continue
		}
		if trimmed == markerEnd {
			inBlock = false
			continue
		}
		if !inBlock {
			result = append(result, line)
		}
	}
	return result
}
