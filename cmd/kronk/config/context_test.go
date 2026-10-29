package config

import (
	"os"
	"strings"
	"testing"
)

func contextChanges(t *testing.T, ctx string, rope string, existing map[string]any) map[string]change {
	t.Helper()

	f := sampleFacts() // trained on 40960
	qs := questions()
	a := newAnswers(qs)
	a.prefill(f, existing)
	*a.sel["context-window"] = ctx
	*a.sel[keyRopeScaling] = rope

	changes, err := diff(qs, f, a, existing)
	if err != nil {
		t.Fatal(err)
	}

	byKey := map[string]change{}
	for _, c := range changes {
		if _, dup := byKey[c.key]; dup {
			t.Fatalf("%s changed twice: %+v", c.key, changes)
		}
		byKey[c.key] = c
	}
	return byKey
}

func TestLongContextLowered(t *testing.T) {
	existing := map[string]any{"rope-scaling-type": "yarn", "rope-freq-scale": 0.5, "yarn-orig-ctx": 32768}

	got := contextChanges(t, "131072", "", existing)
	if c := got["context-window"]; c.new != 40960 {
		t.Fatalf("context-window = %+v, want 40960", c)
	}
	for _, key := range []string{"rope-scaling-type", "rope-freq-scale", "yarn-orig-ctx"} {
		if !got[key].del {
			t.Errorf("%s must be removed when the context is lowered: %+v", key, got[key])
		}
	}
}

func TestLongContextYaRN(t *testing.T) {
	got := contextChanges(t, "131072", "yarn", nil)

	want := map[string]any{"context-window": 131072, "rope-scaling-type": "yarn", "rope-freq-scale": 0.3125, "yarn-orig-ctx": 40960}
	for key, v := range want {
		if got[key].new != v {
			t.Errorf("%s = %v, want %v", key, got[key].new, v)
		}
	}
}

func TestLongContextHidden(t *testing.T) {
	f := sampleFacts()
	a := newAnswers(questions())

	for _, ctx := range []string{"", "8192", "40960"} {
		*a.sel["context-window"] = ctx
		if a.applies(f, a.byKey[keyRopeScaling]) {
			t.Errorf("context %q: the YaRN question must be skipped", ctx)
		}
	}
	*a.sel["context-window"] = "131072"
	if !a.applies(f, a.byKey[keyRopeScaling]) {
		t.Error("a context above the training context must ask about YaRN")
	}
}

func TestContextNote(t *testing.T) {
	f := sampleFacts()
	f.Analysis.Recommended.ContextWindow = 32768
	if note := contextNote(f); !strings.Contains(note, "32768") || !strings.Contains(note, "40960") {
		t.Fatalf("note = %q", note)
	}

	f.Analysis.Recommended.ContextWindow = 40960
	if note := contextNote(f); note != "" {
		t.Fatalf("no note when AutoTune uses the full training context, got %q", note)
	}
}

// TestContextWarningPrefix keeps reviewWarnings in step with the SDK text it
// filters.
func TestContextWarningPrefix(t *testing.T) {
	src, err := os.ReadFile("../../../sdk/tools/models/analyze.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `"`+sdkContextWarning+`%d`) {
		t.Fatalf("the SDK no longer starts the warning with %q; update sdkContextWarning", sdkContextWarning)
	}

	got := reviewWarnings([]string{sdkContextWarning + "32768 (training context: 40960)", "other"})
	if len(got) != 1 || got[0] != "other" {
		t.Fatalf("reviewWarnings = %v", got)
	}
}
