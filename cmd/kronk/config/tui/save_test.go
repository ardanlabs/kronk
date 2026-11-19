package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/kronk/config/configfile"
	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
	"github.com/ardanlabs/kronk/sdk/tools/models"
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

func TestExistingModelSettingsSurviveEditing(t *testing.T) {
	a := newApp(t.Context(), nil, true)
	a.path = filepath.Join(t.TempDir(), "model_config.yaml")
	const original = `version: 2
models:
  owner/model/CODE:
    # Keep manually tuned values and comments.
    context-window: 65536
    rope-scaling-type: yarn
    rope-freq-scale: 0.6
    yarn-orig-ctx: 24000
    nthreads: 3
    nthreads-batch: 7
    cache-type-k: q8_0
    cache-type-v: f8_e4m3
    ngpu-layers: 4
    image-batch-token-capacity: 1536
    sampling-parameters:
      temperature: 0.37
      seed: 29
    adapters:
      - path: /srv/lora/style.gguf
        scale: 0.6
`
	if err := os.WriteFile(a.path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := modelconfig.Load(a.path)
	if err != nil {
		t.Fatal(err)
	}
	a.doc, a.entryID, a.section = doc, "owner/model/CODE", configfile.SectionModels
	a.choice = source.Choice{ID: "owner/model", Installed: true}
	a.startQuestions(source.Facts{
		ID: a.choice.ID, Endpoint: source.EndpointChat,
		HardwareErr: errors.New("hardware detection unavailable"),
		Info:        models.ModelInfo{HasProjection: true},
		Analysis:    models.Analysis{Model: models.ModelFacts{TrainingContext: 32768}},
	})
	for key, want := range map[string]string{
		"nthreads": "3", "nthreads-batch": "7", "cache-type-v": "f8_e4m3",
		"sampling-parameters.temperature": "0.37", "image-batch-token-capacity": "1536",
		"adapters": "/srv/lora/style.gguf=0.6",
	} {
		if got := a.ans.Value(key); got != want {
			t.Errorf("existing %s = %q, want %q", key, got, want)
		}
	}
	a.openAt(stepQuestions, "nthreads-batch")
	if view := a.View().Content; !strings.Contains(view, "batch processing") || !strings.Contains(view, "7") {
		t.Fatalf("existing batch threads not displayed:\n%s", view)
	}
	a.questionsDone()
	if len(a.changes) != 0 || len(a.problems) != 0 {
		t.Fatalf("unchanged edit: changes=%+v problems=%v", a.changes, a.problems)
	}
	msg := a.save(a.path)().(savedMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	data, err := os.ReadFile(a.path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatalf("unchanged save rewrote existing settings:\n%s", data)
	}
}
