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
		// The pull id must be a canonical provider-scoped id (contains a "/").
		if !strings.Contains(m.PullID, "/") {
			t.Errorf("%s: pull_id %q should be provider-scoped (provider/...)", key, m.PullID)
		}
		// It must reference this entry's quant so a pull surfaces the right
		// variant. This holds for both id forms the pull endpoint accepts:
		// the base "provider/model-<quant>" form and the dedicated-repo
		// "provider/repo:tag" form (where the tag carries the quant), which
		// is how the standalone MTP GGUF is pulled.
		if !strings.Contains(m.PullID, m.Quant) {
			t.Errorf("%s: pull_id %q should reference quant %q", key, m.PullID, m.Quant)
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

// TestCuratedInstallStatus verifies the status helper reports the correct
// total and lists exactly the missing curated models, in metadata order.
func TestCuratedInstallStatus(t *testing.T) {
	lm, err := loadLaunchModels()
	if err != nil {
		t.Fatalf("loadLaunchModels: %v", err)
	}
	if len(lm.Order) < 2 {
		t.Skip("need at least two curated models to test partial installs")
	}

	// None installed: every curated model is missing.
	total, missing := curatedInstallStatus([]Model{{ID: "unrelated-Q8_0"}})
	if total != len(lm.Order) {
		t.Errorf("total: got %d, want %d", total, len(lm.Order))
	}
	if len(missing) != len(lm.Order) {
		t.Errorf("missing (none installed): got %d, want %d", len(missing), len(lm.Order))
	}

	// Only the first curated model installed: the rest are missing, in order.
	first := lm.Order[0]
	installed := []Model{{ID: "unsloth/" + first + "/AGENT", Variant: true}}
	total, missing = curatedInstallStatus(installed)
	if total != len(lm.Order) {
		t.Errorf("total: got %d, want %d", total, len(lm.Order))
	}
	if len(missing) != len(lm.Order)-1 {
		t.Fatalf("missing (one installed): got %d, want %d", len(missing), len(lm.Order)-1)
	}
	for i, m := range missing {
		wantKey := lm.Order[i+1]
		if m.Key != wantKey {
			t.Errorf("missing[%d]: got %q, want %q", i, m.Key, wantKey)
		}
	}

	// All installed: nothing missing.
	all := make([]Model, 0, len(lm.Order))
	for _, key := range lm.Order {
		all = append(all, Model{ID: "unsloth/" + key + "/AGENT", Variant: true})
	}
	_, missing = curatedInstallStatus(all)
	if len(missing) != 0 {
		t.Errorf("missing (all installed): got %d, want 0", len(missing))
	}
}

// TestLookupCuratedByAliasAndKey verifies every curated model resolves by both
// its alias and its key, case-insensitively, and that unknown names miss.
func TestLookupCuratedByAliasAndKey(t *testing.T) {
	lm, err := loadLaunchModels()
	if err != nil {
		t.Fatalf("loadLaunchModels: %v", err)
	}

	for key, m := range lm.Models {
		if m.Alias == "" {
			t.Errorf("%s: missing alias", key)
			continue
		}
		if got, ok := lookupCurated(m.Alias); !ok || got.Key != key {
			t.Errorf("lookupCurated(alias %q) = %q,%v; want %q", m.Alias, got.Key, ok, key)
		}
		if got, ok := lookupCurated(strings.ToUpper(key)); !ok || got.Key != key {
			t.Errorf("lookupCurated(KEY %q) = %q,%v; want %q", key, got.Key, ok, key)
		}
	}

	if _, ok := lookupCurated("not-a-real-alias"); ok {
		t.Error("expected no match for unknown alias")
	}
}

// TestResolveDefaultModelByAlias verifies --model accepts a curated alias and
// resolves it to the installed model (profile preferred), while an alias whose
// model is not installed returns a pull hint.
func TestResolveDefaultModelByAlias(t *testing.T) {
	lm, err := loadLaunchModels()
	if err != nil {
		t.Fatalf("loadLaunchModels: %v", err)
	}

	// Pick a curated model and pretend its AGENT profile is installed.
	key := lm.Order[0]
	alias := lm.Models[key].Alias
	installed := []Model{
		{ID: "other-model-Q8_0"},
		{ID: "unsloth/" + key + "/AGENT", Variant: true},
	}

	got, err := resolveDefaultModel(alias, installed)
	if err != nil {
		t.Fatalf("resolveDefaultModel(%q): %v", alias, err)
	}
	if !strings.Contains(got, key) || !strings.HasSuffix(got, "/AGENT") {
		t.Errorf("alias %q resolved to %q; want the installed AGENT profile of %q", alias, got, key)
	}

	// Same alias, but the model is not installed → error with a pull hint.
	_, err = resolveDefaultModel(alias, []Model{{ID: "unrelated-Q8_0"}})
	if err == nil {
		t.Fatalf("expected error for uninstalled curated alias %q", alias)
	}
	if !strings.Contains(err.Error(), "kronk model pull") {
		t.Errorf("error should include a pull hint; got: %v", err)
	}
}
