package schema

import (
	"encoding/json"
	"testing"
)

// go:embed fails at compile time when the file is missing, but not when the
// file is empty or has been corrupted by a bad merge. The schema is the
// compiler's frontend, so it is worth asserting it arrived intact.
func TestOrganizationSchemaIsEmbedded(t *testing.T) {
	if len(Organization) == 0 {
		t.Fatal("вбудована схема порожня")
	}

	var doc map[string]any
	if err := json.Unmarshal(Organization, &doc); err != nil {
		t.Fatalf("вбудована схема не є коректним JSON: %v", err)
	}

	if got, want := doc["$schema"], "https://json-schema.org/draft/2020-12/schema"; got != want {
		t.Errorf("$schema = %v, want %v", got, want)
	}
	if _, ok := doc["$id"]; !ok {
		t.Error("у схемі немає $id")
	}
	if _, ok := doc["$defs"]; !ok {
		t.Error("у схемі немає $defs")
	}
}
