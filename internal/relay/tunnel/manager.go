package tunnel

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"sync"
)

// Manager manages a cloudflared subprocess.
type Manager struct {
	binaryPath string
	tunnelURL  string

	mu  sync.Mutex
	cmd *exec.Cmd
}

// NewManager creates a tunnel manager.
// binaryPath is the path to the cloudflared binary.
// tunnelURL is the public URL cloudflared exposes (set externally or parsed from stdout).
func NewManager(binaryPath, tunnelURL string) *Manager {
	return &Manager{binaryPath: binaryPath, tunnelURL: tunnelURL}
}

// Start launches cloudflared as a subprocess. It returns an error if the
// process fails to start; it does NOT wait for the tunnel to be established.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmd != nil {
		return fmt.Errorf("[tunnel] already running")
	}

	// ponytail: forward stdout/stderr to the default logger via pipe
	cmd := exec.CommandContext(ctx, m.binaryPath, "tunnel", "--url", m.tunnelURL)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("[tunnel] start: %w", err)
	}

	m.cmd = cmd
	log.Printf("[tunnel] started pid=%d url=%s", cmd.Process.Pid, m.tunnelURL)

	// Reap the process in the background so it doesn't become a zombie.
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("[tunnel] exited: %v", err)
		} else {
			log.Printf("[tunnel] exited cleanly")
		}
		m.mu.Lock()
		m.cmd = nil
		m.mu.Unlock()
	}()

	return nil
}

// Stop kills the cloudflared subprocess if running.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmd == nil || m.cmd.Process == nil {
		return
	}
	if err := m.cmd.Process.Kill(); err != nil {
		log.Printf("[tunnel] kill: %v", err)
	}
}

// URL returns the configured public tunnel URL.
func (m *Manager) URL() string { return m.tunnelURL }
