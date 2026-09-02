// Package proxy — model alias resolution regression tests.
package proxy

import (
	"testing"

	"switchblade/internal/db"
)

func TestModelMapperResolveUsesSchemaColumns(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	_, err = database.Exec(`INSERT INTO model_mappings (source_pattern, match_type, target_model, enabled, priority, created_at) VALUES ('alias', 'exact', 'real-model', 1, 1, 1)`)
	if err != nil {
		t.Fatalf("insert mapping: %v", err)
	}

	if got := NewModelMapper(database).Resolve("alias"); got != "real-model" {
		t.Fatalf("Resolve(alias) = %q, want real-model", got)
	}
}
