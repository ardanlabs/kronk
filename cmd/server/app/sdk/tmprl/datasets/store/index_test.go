package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets/store"
	"go.yaml.in/yaml/v3"
)

func TestIndexRebuildRelocationAndCorruption(t *testing.T) {
	ctx := t.Context()
	root := filepath.Join(t.TempDir(), "datasets")
	files, err := store.New(store.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	dataset, err := files.Create(ctx, "Harbor")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := files.Add(ctx, dataset.ID, 1, documentAddition("source-a", "a", "When does the harbor open?"))
	if err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(root, ".index.yaml")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	var index struct {
		Datasets map[string]struct {
			Latest   int `yaml:"latest_file_version"`
			Versions map[int]struct {
				Snapshot, SHA256 string
				Size             int64 `yaml:"size_bytes"`
			} `yaml:"versions"`
		} `yaml:"datasets"`
	}
	if err := yaml.Unmarshal(data, &index); err != nil {
		t.Fatal(err)
	}
	entry := index.Datasets[dataset.ID]
	if entry.Latest != 2 || len(entry.Versions) != 2 || entry.Versions[2].Snapshot != dataset.ID+"/2.json" || len(entry.Versions[2].SHA256) != 64 || entry.Versions[2].Size <= 0 || strings.Contains(string(data), root) {
		t.Fatalf("invalid portable inventory: %s", data)
	}
	if err := os.WriteFile(indexPath, []byte("broken: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "moved")
	if err := os.Rename(root, destination); err != nil {
		t.Fatal(err)
	}
	files, err = store.New(store.Config{Root: destination})
	if err != nil {
		t.Fatal(err)
	}
	info, err := files.Lookup(ctx, saved.ID, saved.Version)
	if err != nil || info.SHA256 != entry.Versions[2].SHA256 || info.Dataset.RecordCount != 1 {
		t.Fatalf("relocated snapshot: %+v %v", info, err)
	}
	if err := os.WriteFile(filepath.Join(destination, dataset.ID, "2.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := files.BuildIndex(ctx); err == nil {
		t.Fatal("corrupt snapshot accepted as inventory")
	}
	if _, err := files.Get(ctx, dataset.ID, 1); err != nil {
		t.Fatalf("previous version lost: %v", err)
	}
}

func TestCreationReceiptRecovery(t *testing.T) {
	files, err := store.New(store.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	first, err := files.CreateOperation(t.Context(), "dataset-recovery", "Harbor", "create-op", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	again, err := files.CreateOperation(t.Context(), first.ID, "Harbor", "create-op", strings.Repeat("a", 64))
	if err != nil || again.Version != 1 || !again.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("creation duplicated: %+v %v", again, err)
	}
	if _, err := files.CreateOperation(t.Context(), first.ID, "Changed", "create-op", strings.Repeat("b", 64)); err == nil {
		t.Fatal("conflicting creation accepted")
	}
}

func TestForeignSnapshotsRejected(t *testing.T) {
	files, err := store.New(store.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	foreign := t.TempDir()
	if err := os.Symlink(foreign, filepath.Join(files.Path(), "dataset-foreign")); err != nil {
		t.Fatal(err)
	}
	if err := files.BuildIndex(t.Context()); err == nil {
		t.Fatal("foreign dataset silently indexed or hidden")
	}
	if _, err := files.Inspect(t.Context(), "dataset-foreign", 1); err == nil {
		t.Fatal("foreign path accepted")
	}
}
