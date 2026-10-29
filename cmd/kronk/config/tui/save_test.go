package tui

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ardanlabs/kronk/cmd/kronk/config/configfile"
	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
)

func TestSaveToAnotherFileWritesTheWholeEntry(t *testing.T) {
	a := newApp(context.Background(), nil, true)
	a.path = filepath.Join(t.TempDir(), "source.yaml")
	a.section = configfile.SectionModels
	a.entryID = "a/b"
	a.existing = map[string]any{
		"context-window":                 32768,
		"sampling-parameters.max_tokens": 8192,
	}
	a.changes = nil

	dest := filepath.Join(t.TempDir(), "other.yaml")
	msg, ok := a.save(dest)().(savedMsg)
	if !ok || msg.err != nil {
		t.Fatalf("save = %#v", msg)
	}
	if msg.path != dest {
		t.Fatalf("saved to %q, want %q", msg.path, dest)
	}

	doc, err := modelconfig.Load(dest)
	if err != nil {
		t.Fatal(err)
	}

	mc, ok := doc.Models["a/b"]
	if !ok {
		t.Fatal("the entry was not written")
	}
	if mc.PtrContextWindow == nil || *mc.PtrContextWindow != 32768 {
		t.Fatalf("context-window = %v", mc.PtrContextWindow)
	}
	if mc.Sampling.MaxTokens != 8192 {
		t.Fatalf("max_tokens = %d", mc.Sampling.MaxTokens)
	}
}
