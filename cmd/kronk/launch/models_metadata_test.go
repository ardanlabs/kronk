package launch

import (
	"strings"
	"testing"
)

// TestLaunchModelsParses verifies the embedded curated-models metadata parses
// and reports a schema version this build understands.
func TestLaunchModelsParses(t *testing.T) {
	lm, err := loadLaunchModels()
	if err != nil {
		t.Fatalf("loadLaunchModels: %v", err)
	}

	if lm.Schema != launchModelsSchemaVersion {
		t.Fatalf("schema: got %d, want %d", lm.Schema, launchModelsSchemaVersion)
	}

	if len(lm.Order) == 0 {
		t.Fatal("expected a non-empty order")
	}
}

// TestLaunchModelsEntriesWellFormed checks every curated entry has the fields
// the launcher needs and that its key is backfilled.
func TestLaunchModelsEntriesWellFormed(t *testing.T) {
	lm, err := loadLaunchModels()
	if err != nil {
		t.Fatalf("loadLaunchModels: %v", err)
	}

	for key, m := range lm.Models {
		if m.Key != key {
			t.Errorf("%s: key not backfilled, got %q", key, m.Key)
		}
		if m.Display == "" {
			t.Errorf("%s: empty display", key)
		}
		if m.Quant == "" {
			t.Errorf("%s: empty quant", key)
		}
		if m.PullID == "" {
			t.Errorf("%s: empty pull_id", key)
		}
		// The pull id must be a canonical provider/model id, and the base name
		// must match the entry key so a pull surfaces the curated model.
		if !strings.HasSuffix(m.PullID, key) {
			t.Errorf("%s: pull_id %q should end with the entry key", key, m.PullID)
		}
	}
}

// TestLaunchModelsOrderResolves verifies every id in order has an entry (the
// loader enforces this; this guards against regressions).
func TestLaunchModelsOrderResolves(t *testing.T) {
	lm, err := loadLaunchModels()
	if err != nil {
		t.Fatalf("loadLaunchModels: %v", err)
	}

	for _, key := range lm.Order {
		if _, ok := lm.Models[key]; !ok {
			t.Errorf("order references unknown model %q", key)
		}
	}
}

// TestMatchCurated exercises the segment-based matcher against the id forms the
// server can report for a profile model, including the base-vs-profile
// distinction.
func TestMatchCurated(t *testing.T) {
	const key = "Qwen3.6-35B-A3B-UD-Q8_K_XL"

	tests := []struct {
		id   string
		want curatedMatch
	}{
		{"Qwen3.6-35B-A3B-UD-Q8_K_XL", baseMatch},
		{"unsloth/Qwen3.6-35B-A3B-UD-Q8_K_XL", baseMatch},
		{"Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT", profileMatch},
		{"unsloth/Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT", profileMatch},
		{"gemma-4-26B-A4B-it-UD-Q8_K_XL/AGENT", noMatch},
		{"mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT", noMatch},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if got := matchCurated(tt.id, key); got != tt.want {
				t.Errorf("matchCurated(%q, %q) = %v, want %v", tt.id, key, got, tt.want)
			}
		})
	}
}

// TestFirstInstalledCuratedPrefersOrderAndVariant verifies the selector honors
// metadata order and prefers a profile variant (which carries the AGENT
// context/sampling profile) over a bare match.
func TestFirstInstalledCuratedPrefersOrder(t *testing.T) {
	lm, err := loadLaunchModels()
	if err != nil {
		t.Fatalf("loadLaunchModels: %v", err)
	}
	if len(lm.Order) < 2 {
		t.Skip("need at least two curated models to test ordering")
	}

	first := lm.Order[0]
	second := lm.Order[1]

	// Both installed as AGENT variants; the first in order must win.
	chatModels := []Model{
		{ID: "unsloth/" + second + "/AGENT", Variant: true},
		{ID: "unsloth/" + first + "/AGENT", Variant: true},
	}

	entry, m, ok := firstInstalledCurated(chatModels)
	if !ok {
		t.Fatal("expected a curated match")
	}
	if entry.Key != first {
		t.Errorf("preferred entry: got %q, want %q", entry.Key, first)
	}
	if !strings.Contains(m.ID, first) {
		t.Errorf("selected model %q should match %q", m.ID, first)
	}
}

// TestFirstInstalledCuratedPrefersVariant verifies that when both a bare and a
// variant form of the same curated model are installed, the variant wins.
func TestFirstInstalledCuratedPrefersVariant(t *testing.T) {
	lm, err := loadLaunchModels()
	if err != nil {
		t.Fatalf("loadLaunchModels: %v", err)
	}
	if len(lm.Order) == 0 {
		t.Skip("no curated models")
	}

	key := lm.Order[0]
	chatModels := []Model{
		{ID: "unsloth/" + key, Variant: true}, // provider/model (has "/")
		{ID: key + "/AGENT", Variant: true},   // the AGENT profile variant
	}

	_, m, ok := firstInstalledCurated(chatModels)
	if !ok {
		t.Fatal("expected a curated match")
	}
	if !strings.HasSuffix(m.ID, "/AGENT") {
		t.Errorf("expected the AGENT variant to win, got %q", m.ID)
	}
}

// TestFirstInstalledCuratedNoneInstalled verifies the selector reports no match
// when nothing curated is installed.
func TestFirstInstalledCuratedNoneInstalled(t *testing.T) {
	chatModels := []Model{
		{ID: "some-other-model-Q8_0"},
		{ID: "unsloth/unrelated-model"},
	}

	if _, _, ok := firstInstalledCurated(chatModels); ok {
		t.Error("expected no curated match")
	}
}
