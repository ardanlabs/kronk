package moxierun

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
)

func TestConfigValidation(t *testing.T) {
	for _, test := range []struct {
		name      string
		address   string
		namespace string
		models    []string
		wantError bool
	}{
		{"enabled", "localhost:7233", "default", []string{"huggingface/model"}, false},
		{"no-models", "localhost:7233", "default", nil, true},
		{"no-address", "", "default", []string{"huggingface/model"}, true},
		{"no-namespace", "localhost:7233", "", []string{"huggingface/model"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := Config{Address: test.address, Namespace: test.namespace, Models: test.models}
			if err := validateConfig(cfg); (err != nil) != test.wantError {
				t.Fatalf("validation: %v, want error %t", err, test.wantError)
			}
		})
	}
}

func TestStorageRoot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root, err := storageRoot("")
	if err != nil || root != filepath.Join(os.Getenv("HOME"), ".moxie") {
		t.Fatalf("existing default changed: %q %v", root, err)
	}
	custom := filepath.Join(t.TempDir(), "relocated")
	if got, err := storageRoot(custom); err != nil || got != custom {
		t.Fatalf("explicit root: %q %v", got, err)
	}
	if _, err := storageRoot(" "); err == nil {
		t.Fatal("blank root accepted")
	}
}

func TestInitializationValidatesBeforeStorage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "moxie")
	log := logger.New(io.Discard, logger.LevelInfo, "moxie-test", nil)
	runtime := New(t.Context(), log, Config{Root: root, DatabaseURL: "invalid-database-url"})
	t.Cleanup(runtime.Close)
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("directory before first use: got %v, want no directory", err)
	}
	got, err := runtime.Resolve(t.Context())
	if err == nil {
		t.Fatal("expected configuration error")
	}
	if got != (Services{}) {
		t.Fatalf("Moxie services: got %+v, want no resources", got)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("directory after invalid configuration: got %v, want no directory", err)
	}
}
