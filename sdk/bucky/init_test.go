package bucky_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/tools/backend"
	"github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
	"github.com/ardanlabs/kronk/sdk/tools/bucky/models"
)

func TestInitRoundTrip(t *testing.T) {
	if err := bucky.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	b, ok := backend.Get(backend.KindWhisper)
	if !ok {
		t.Fatalf("Get %q: not registered", backend.KindWhisper)
	}
	if b.Kind != backend.KindWhisper {
		t.Fatalf("Kind: got %q, want %q", b.Kind, backend.KindWhisper)
	}

	lm, err := b.NewLibs()
	if err != nil {
		t.Fatalf("NewLibs: %v", err)
	}
	if _, ok := lm.(*libs.Libs); !ok {
		t.Fatalf("NewLibs returned %T, want *libs.Libs", lm)
	}

	tmp := t.TempDir()
	cat, err := b.NewCatalog(tmp)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	if _, ok := cat.(*models.Models); !ok {
		t.Fatalf("NewCatalog returned %T, want *models.Models", cat)
	}
	if got, want := filepath.Base(cat.Path()), "bucky-models"; got != want {
		t.Fatalf("catalog Path basename: got %q, want %q", got, want)
	}
}

func TestLibsCombinations(t *testing.T) {
	tmp := t.TempDir()

	l, err := libs.New(
		libs.WithBasePath(tmp),
		libs.WithArch("arm64"),
		libs.WithOS("darwin"),
		libs.WithProcessor("metal"),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if !l.IsSupported("arm64", "darwin", "metal") {
		t.Errorf("IsSupported(arm64, darwin, metal): got false, want true")
	}
	if l.IsSupported("arm64", "darwin", "cuda") {
		t.Errorf("IsSupported(arm64, darwin, cuda): got true, want false")
	}

	combos := l.SupportedCombinations()
	if len(combos) == 0 {
		t.Fatalf("SupportedCombinations: empty")
	}

	wantPath := filepath.Join(tmp, "bucky-libraries", "darwin", "arm64", "metal")
	if l.LibsPath() != wantPath {
		t.Errorf("LibsPath: got %q, want %q", l.LibsPath(), wantPath)
	}

	if l.ReadOnly() {
		t.Errorf("ReadOnly: got true, want false for fresh root")
	}

	if _, err := l.InstalledVersion(); err == nil {
		t.Errorf("InstalledVersion on empty path: got nil, want error")
	}

	list, err := l.List()
	if err != nil {
		t.Fatalf("List on empty root: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("List: got %d, want 0", len(list))
	}
}

func TestModelsLifecycle(t *testing.T) {
	tmp := t.TempDir()

	m, err := models.NewWithPaths(tmp)
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}

	// Stage a fake whisper model file so BuildIndex / FullPath /
	// Remove can run without network.
	fake := filepath.Join(m.Path(), "ggml-tiny.bin")
	if err := os.WriteFile(fake, []byte("not-a-real-model"), 0o644); err != nil {
		t.Fatalf("seed fake model: %v", err)
	}

	log := func(_ context.Context, _ string, _ ...any) {}

	if err := m.BuildIndex(log, false); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	mp, err := m.FullPath("tiny")
	if err != nil {
		t.Fatalf("FullPath(tiny): %v", err)
	}
	if len(mp.ModelFiles) != 1 || filepath.Base(mp.ModelFiles[0]) != "ggml-tiny.bin" {
		t.Fatalf("FullPath(tiny): got %+v", mp)
	}

	// Short-name resolution should accept ggml-<name>.bin too.
	if _, err := m.FullPath("ggml-tiny.bin"); err != nil {
		t.Errorf("FullPath(ggml-tiny.bin): %v", err)
	}

	if err := m.Remove(mp, log); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, err := os.Stat(fake); !os.IsNotExist(err) {
		t.Fatalf("after Remove: got err=%v, want IsNotExist", err)
	}

	if _, err := m.FullPath("tiny"); err == nil {
		t.Errorf("FullPath after Remove: got nil, want error")
	}
}

func TestSupportedModels(t *testing.T) {
	names := models.SupportedModels()
	if len(names) == 0 {
		t.Fatalf("SupportedModels: empty")
	}
	want := map[string]bool{"tiny": true, "base.en": true, "large-v3": true}
	got := map[string]bool{}
	for _, n := range names {
		got[n] = true
	}
	for k := range want {
		if !got[k] {
			t.Errorf("SupportedModels missing %q", k)
		}
	}
}
