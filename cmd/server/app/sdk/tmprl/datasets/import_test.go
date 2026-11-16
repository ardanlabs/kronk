package datasets_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets/store"
	optimization "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/moxiedb"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
)

// A caller must explicitly supply a disposable PostgreSQL database. Each test
// installs the real migration in its own schema; runtime initialization does not.
func catalogDB(t *testing.T) *moxiedb.DB {
	t.Helper()
	connection := os.Getenv("MOXIEDB_TEST_URL")
	if connection == "" {
		t.Skip("set MOXIEDB_TEST_URL to an isolated PostgreSQL test database")
	}
	admin, err := sql.Open("pgx", connection)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schema := "datasets_test_" + strings.ToLower(rand.Text())
	if _, err := admin.ExecContext(t.Context(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })
	u, err := url.Parse(connection)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	setup, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer setup.Close()
	migration, err := os.ReadFile(filepath.Join("..", "moxiedb", "migrations", "001_init.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := setup.ExecContext(t.Context(), string(migration)); err != nil {
		t.Fatal(err)
	}
	if _, err := setup.ExecContext(t.Context(), "INSERT INTO schema_migrations(version,checksum) VALUES (1,$1)", fmt.Sprintf("%x", sha256.Sum256(migration))); err != nil {
		t.Fatal(err)
	}
	db, err := moxiedb.Open(t.Context(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.CheckSchema(t.Context()); err != nil {
		t.Fatal(err)
	}
	return db
}

func addition(id, hash string) store.DatasetAddition {
	return store.DatasetAddition{PublicationID: "run-" + id,
		Source:  uploads.File{ID: id, Name: id + ".txt", SHA256: strings.Repeat(hash, 64)},
		Records: []optimization.Record{{Prompt: "When does " + id + " open?", Target: "Dawn.", Evidence: id + " opens at dawn.", SourceDocument: id}},
	}
}

func TestCatalogImportRelocationAndIntegrity(t *testing.T) {
	db, ctx := catalogDB(t), t.Context()
	root := filepath.Join(t.TempDir(), "moxie")
	files, err := store.NewDatasetStore(filepath.Join(root, "preparation-datasets"))
	if err != nil {
		t.Fatal(err)
	}
	dataset, err := files.Create(ctx, "Harbor")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := files.Add(ctx, dataset.ID, 1, addition("harbor", "a")); err != nil {
		t.Fatal(err)
	}
	if _, err := files.Remove(ctx, dataset.ID, 2, "run-harbor"); err != nil {
		t.Fatal(err)
	}
	service := datasets.New(files, nil, db)
	for range 2 {
		if err := service.Initialize(ctx); err != nil {
			t.Fatal(err)
		}
	}
	versions, err := db.ListVersions(ctx, dataset.ID)
	if err != nil || len(versions) != 3 {
		t.Fatalf("imported history: %+v %v", versions, err)
	}
	if versions[0].SnapshotPath != "preparation-datasets/"+dataset.ID+"/3.json" {
		t.Fatalf("nonportable SQL path: %q", versions[0].SnapshotPath)
	}
	if rows, err := service.Records(ctx, dataset.ID, 2); err != nil || len(rows) != 1 || rows[0].SourceDocument != "harbor" {
		t.Fatalf("historical records: %+v %v", rows, err)
	}
	if rows, err := service.Records(ctx, dataset.ID, 0); err != nil || len(rows) != 0 {
		t.Fatalf("removed contribution returned: %+v %v", rows, err)
	}
	if err := os.WriteFile(filepath.Join(files.Path(), ".index.yaml"), []byte("broken: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(t.TempDir(), "relocated")
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	files, err = store.NewDatasetStore(filepath.Join(moved, "preparation-datasets"))
	if err != nil {
		t.Fatal(err)
	}
	service = datasets.New(files, nil, db)
	if err := service.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if rows, err := service.List(ctx); err != nil || len(rows) != 1 || rows[0].Version != 3 {
		t.Fatalf("relocated catalog: %+v %v", rows, err)
	}
	path := filepath.Join(files.Path(), dataset.ID, "2.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := service.Initialize(ctx); err == nil {
		t.Fatal("missing published historical file accepted")
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(data), "Dawn.", "Dusk.")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := service.Initialize(ctx); err == nil {
		t.Fatal("changed snapshot accepted")
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := service.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogPublicationAndRecovery(t *testing.T) {
	db, ctx := catalogDB(t), t.Context()
	files, err := store.NewDatasetStore(filepath.Join(t.TempDir(), "preparation-datasets"))
	if err != nil {
		t.Fatal(err)
	}
	service := datasets.New(files, nil, db)
	if err := service.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	dataset, err := service.Create(ctx, "Harbor")
	if err != nil {
		t.Fatal(err)
	}
	first := addition("harbor", "a")
	for range 2 {
		saved, err := service.Add(ctx, dataset.ID, 1, first)
		if err != nil || saved.Version != 2 {
			t.Fatalf("addition replay: %+v %v", saved, err)
		}
	}
	var wg sync.WaitGroup
	for i, hash := range []string{"b", "c"} {
		wg.Go(func() {
			if _, err := service.Add(ctx, dataset.ID, 1, addition(fmt.Sprintf("source%d", i), hash)); err != nil {
				t.Errorf("concurrent addition: %v", err)
			}
		})
	}
	wg.Wait()
	head, err := service.Get(ctx, dataset.ID, 0)
	if err != nil || head.Version != 4 || head.RecordCount != 3 {
		t.Fatalf("lost concurrent contribution: %+v %v", head, err)
	}
	if _, err := service.Remove(ctx, dataset.ID, 2, "run-harbor"); !errors.Is(err, store.ErrDatasetConflict) {
		t.Fatalf("stale removal: %v", err)
	}
	if saved, err := service.Remove(ctx, dataset.ID, 4, "run-harbor"); err != nil || saved.Version != 5 || saved.RecordCount != 2 {
		t.Fatalf("removal: %+v %v", saved, err)
	}
	if rows, err := service.Records(ctx, dataset.ID, 2); err != nil || len(rows) != 1 {
		t.Fatalf("history changed: %+v %v", rows, err)
	}

	// Simulate loss of the SQL receipt after the complete immutable file exists.
	next := addition("interrupted", "d")
	data, _ := json.Marshal(next)
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	operation := "add/" + next.PublicationID
	if err := db.BeginPublication(ctx, moxiedb.Publication{ID: operation, DatasetID: dataset.ID, Kind: "add", BaseVersion: 5, InputDigest: digest}); err != nil {
		t.Fatal(err)
	}
	if _, err := files.Add(ctx, dataset.ID, 5, next); err != nil {
		t.Fatal(err)
	}
	if head, err := service.Get(ctx, dataset.ID, 0); err != nil || head.Version != 5 {
		t.Fatalf("unpublished head exposed: %+v %v", head, err)
	}
	if err := service.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if saved, err := service.Add(ctx, dataset.ID, 5, next); err != nil || saved.Version != 6 {
		t.Fatalf("interrupted addition duplicated: %+v %v", saved, err)
	}
	index := filepath.Join(files.Path(), ".index.yaml")
	if err := os.Remove(index); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(index, 0o700); err != nil {
		t.Fatal(err)
	}
	last := addition("index-failure", "e")
	saved, err := service.Add(ctx, dataset.ID, 6, last)
	if err == nil || saved.Version != 7 || !strings.Contains(err.Error(), "is saved; index refresh failed") {
		t.Fatalf("partial success misreported: %+v %v", saved, err)
	}
	if v, err := db.GetVersion(ctx, dataset.ID, 0); err != nil || v.Version != 7 {
		t.Fatalf("index failure lost committed catalog: %+v %v", v, err)
	}
	if err := os.Remove(index); err != nil {
		t.Fatal(err)
	}
	if saved, err := service.Add(ctx, dataset.ID, 6, last); err != nil || saved.Version != 7 {
		t.Fatalf("index recovery duplicated publication: %+v %v", saved, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, "Must not fall back"); err == nil {
		t.Fatal("SQL outage silently accepted a file-only write")
	}
	if rows, err := files.List(ctx); err != nil || len(rows) != 1 || rows[0].Version != 7 {
		t.Fatalf("outage changed files: %+v %v", rows, err)
	}
}

func TestInterruptedCreateRemoveAndUnresolvedIntent(t *testing.T) {
	db, ctx := catalogDB(t), t.Context()
	files, err := store.NewDatasetStore(filepath.Join(t.TempDir(), "preparation-datasets"))
	if err != nil {
		t.Fatal(err)
	}
	service := datasets.New(files, nil, db)
	data, _ := json.Marshal("Harbor")
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	p := moxiedb.Publication{ID: "create/interrupted", DatasetID: "dataset-interrupted", Kind: "create", InputDigest: digest}
	if err := db.BeginPublication(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := files.CreateOperation(ctx, p.DatasetID, "Harbor", p.ID, digest); err != nil {
		t.Fatal(err)
	}
	if err := service.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if saved, err := service.Get(ctx, p.DatasetID, 0); err != nil || saved.Version != 1 {
		t.Fatalf("creation recovery: %+v %v", saved, err)
	}
	if _, err := service.Add(ctx, p.DatasetID, 1, addition("harbor", "a")); err != nil {
		t.Fatal(err)
	}
	removeInput, _ := json.Marshal([]any{p.DatasetID, 2, "run-harbor"})
	removeDigest := fmt.Sprintf("%x", sha256.Sum256(removeInput))
	remove := moxiedb.Publication{ID: "remove/interrupted", DatasetID: p.DatasetID, Kind: "remove", BaseVersion: 2, InputDigest: removeDigest}
	if err := db.BeginPublication(ctx, remove); err != nil {
		t.Fatal(err)
	}
	if _, err := files.RemoveOperation(ctx, p.DatasetID, 2, "run-harbor", remove.ID, removeDigest); err != nil {
		t.Fatal(err)
	}
	if err := service.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	if saved, err := service.Get(ctx, p.DatasetID, 0); err != nil || saved.Version != 3 || saved.RecordCount != 0 {
		t.Fatalf("removal recovery: %+v %v", saved, err)
	}
	if records, err := service.Records(ctx, p.DatasetID, 2); err != nil || len(records) != 1 {
		t.Fatalf("recovered removal lost history: %+v %v", records, err)
	}
	p.ID, p.DatasetID = "create/no-file", "dataset-no-file"
	if err := db.BeginPublication(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := service.Initialize(ctx); err == nil {
		t.Fatal("unresolved publication was hidden")
	}
	pending, err := db.GetPublication(ctx, p.ID)
	if err != nil || pending.State != "needs_attention" {
		t.Fatalf("uncertain outcome: %+v %v", pending, err)
	}
	if _, err := service.Get(ctx, "dataset-interrupted", 1); err != nil {
		t.Fatalf("known published data lost: %v", err)
	}
}
