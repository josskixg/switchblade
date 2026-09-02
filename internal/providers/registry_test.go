package providers

import (
	"context"
	"testing"
)

// stub is a minimal Provider for testing the registry — no network, no DB.
type stub struct{ name string }

func (s *stub) Name() string                { return s.name }
func (s *stub) OwnsModel(model string) bool { return model == s.name }
func (s *stub) Chat(_ context.Context, _ *Account, _ *ChatRequest) (*ChatResponse, error) {
	return nil, nil
}
func (s *stub) Healthy(_ context.Context, _ *Account) bool { return true }

func TestRegistry_Register(t *testing.T) {
	var r Registry
	r.Register(&stub{name: "gpt-4"})
	if len(r.All()) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(r.All()))
	}
}

func TestRegistry_FindForModel(t *testing.T) {
	var r Registry
	r.Register(&stub{name: "gpt-4"})
	p := r.Route("gpt-4")
	if p == nil {
		t.Fatal("expected a provider for 'gpt-4', got nil")
	}
	if p.Name() != "gpt-4" {
		t.Fatalf("expected 'gpt-4', got %q", p.Name())
	}
}

func TestRegistry_NoMatch(t *testing.T) {
	var r Registry
	r.Register(&stub{name: "gpt-4"})
	if r.Route("unknown-model") != nil {
		t.Fatal("expected nil for unknown model")
	}
}

func TestRegistry_RouteUsesConfiguredModels(t *testing.T) {
	store := &ConfigStore{data: map[string]ProviderConfig{
		"gpt-4": {Provider: "gpt-4", Models: `["configured-model"]`, Enabled: true},
	}}
	r := Registry{ConfigStore: store}
	r.Register(&stub{name: "gpt-4"})

	if p := r.Route("configured-model"); p == nil || p.Name() != "gpt-4" {
		t.Fatalf("configured model routed to %v, want gpt-4", p)
	}
	if p := r.Route("gpt-4"); p != nil {
		t.Fatalf("provider's static model should be replaced by configured models, got %q", p.Name())
	}
}

func TestRegistry_RouteSkipsDisabledProvider(t *testing.T) {
	store := &ConfigStore{data: map[string]ProviderConfig{
		"gpt-4": {Provider: "gpt-4", Models: `["gpt-4"]`, Enabled: false},
	}}
	r := Registry{ConfigStore: store}
	r.Register(&stub{name: "gpt-4"})

	if p := r.Route("gpt-4"); p != nil {
		t.Fatalf("disabled provider routed model via %q", p.Name())
	}
}
