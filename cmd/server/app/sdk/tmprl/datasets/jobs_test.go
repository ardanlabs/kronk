package datasets_test

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets/store"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/moxiedb"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
)

func TestCatalogHistoryWithoutWorker(t *testing.T) {
	db := catalogDB(t)
	files, err := store.NewDatasetStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := datasets.New(files, nil, db)
	service.Namespace = "catalog-test"
	if err := service.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	for i, mode := range []string{"preparation", "training", "preparation"} {
		id := fmt.Sprintf("job-%d", i)
		if err := db.CreateJob(t.Context(), moxiedb.Job{JobID: id, Mode: mode}); err != nil {
			t.Fatal(err)
		}
		a := moxiedb.Attempt{JobID: id, AttemptID: id + "/1", AttemptNumber: 1, TemporalNamespace: service.Namespace, WorkflowID: id, RunID: sql.NullString{String: id + "-run", Valid: true}}
		if err := db.CreateAttempt(t.Context(), a); err != nil {
			t.Fatal(err)
		}
		status := "completed"
		if i == 2 {
			status = "waiting"
		}
		if err := db.UpdateAttemptObservation(t.Context(), a.AttemptID, 0, moxiedb.AttemptObservation{ExecutionStatus: sql.NullString{String: status, Valid: true}, ObservedAt: sql.NullTime{Time: time.Now(), Valid: true}}); err != nil {
			t.Fatal(err)
		}
	}
	payload := json.RawMessage(`{"private":"must-not-be-public"}`)
	if err := db.BeginIntent(t.Context(), moxiedb.Intent{IntentID: "review", JobID: "job-2", AttemptID: "job-2/1", Kind: "preview_review", CheckpointID: sql.NullString{String: "preview", Valid: true}, Payload: payload, PayloadSHA256: fmt.Sprintf("%x", sha256.Sum256(payload))}); err != nil {
		t.Fatal(err)
	}
	filter := datasets.RunFilter{PageSize: 1, Mode: "preparation"}
	rows, next, err := service.ListCatalogRuns(t.Context(), filter)
	if err != nil || len(rows) != 1 || rows[0].WorkflowID != "job-2" || !rows[0].StateStale || rows[0].Status != "waiting" || len(next) == 0 || len(rows[0].PendingActions) != 1 || rows[0].PendingActions[0].Kind != "preview_review" {
		t.Fatalf("workerless page: %+v %q %v", rows, next, err)
	}
	filter.NextPageToken = next
	rows, next, err = service.ListCatalogRuns(t.Context(), filter)
	if err != nil || len(rows) != 1 || rows[0].WorkflowID != "job-0" || len(next) != 0 {
		t.Fatalf("second filtered page: %+v %q %v", rows, next, err)
	}
	filter.Mode = "training"
	if _, _, err := service.ListCatalogRuns(t.Context(), filter); !errors.Is(err, tmprl.ErrInvalidArgument) {
		t.Fatalf("cross-filter token accepted: %v", err)
	}
	filter = datasets.RunFilter{PageSize: 25, Statuses: []tmprl.Status{tmprl.StatusCompleted}, Mode: "preparation"}
	rows, _, err = service.ListCatalogRuns(t.Context(), filter)
	if err != nil || len(rows) != 1 || rows[0].WorkflowID != "job-0" {
		t.Fatalf("status and mode intersection: %+v %v", rows, err)
	}
	e := tmprl.Execution{WorkflowID: "job-2", RunID: "job-2-run"}
	detail, err := service.GetRun(t.Context(), e)
	if err != nil || detail.JobID != "job-2" || !detail.StateStale || detail.Status != "waiting" {
		t.Fatalf("workerless detail: %+v %v", detail, err)
	}
	if err := service.StopRun(t.Context(), e); !errors.Is(err, training.ErrUnavailable) {
		t.Fatalf("workerless command accepted: %v", err)
	}
	if _, err := service.PreparationControl(t.Context(), e, tmprl.ControlRequest{RequestID: "pause", Action: "pause"}); !errors.Is(err, training.ErrUnavailable) {
		t.Fatalf("workerless pause accepted: %v", err)
	}
	if _, err := service.GetRun(t.Context(), tmprl.Execution{WorkflowID: e.WorkflowID, RunID: "different"}); !errors.Is(err, training.ErrNotFound) {
		t.Fatalf("exact run ID ignored: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.ListCatalogRuns(t.Context(), datasets.RunFilter{PageSize: 25}); err == nil {
		t.Fatal("database outage silently fell back to file-only history")
	}
}

func TestCatalogExactDatasetOutput(t *testing.T) {
	db := catalogDB(t)
	files, err := store.NewDatasetStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := datasets.New(files, nil, db)
	if err := service.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	d, err := service.Create(t.Context(), "Catalog output")
	if err != nil {
		t.Fatal(err)
	}
	d, err = service.Add(t.Context(), d.ID, d.Version, addition("original", "c"))
	if err != nil {
		t.Fatal(err)
	}
	sources, err := uploads.New(uploads.Config{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	source, err := sources.Put(t.Context(), uploads.Input{Name: "harbor-manual.md"}, strings.NewReader("# Harbor"))
	if err != nil {
		t.Fatal(err)
	}
	service.Uploads = sources
	if err := db.CreateJob(t.Context(), moxiedb.Job{JobID: "saved-job", Mode: "preparation",
		SourceUploadID:  sql.NullString{String: source.ID, Valid: true},
		TargetDatasetID: sql.NullString{String: d.ID, Valid: true}, BaseDatasetVersion: sql.NullInt64{Int64: 1, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	a := moxiedb.Attempt{AttemptID: "saved-attempt", JobID: "saved-job", AttemptNumber: 1, TemporalNamespace: "default", WorkflowID: "saved-workflow", RunID: sql.NullString{String: "saved-run", Valid: true}}
	if err := db.CreateAttempt(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateAttemptObservation(t.Context(), a.AttemptID, 0, moxiedb.AttemptObservation{ExecutionStatus: sql.NullString{String: "completed", Valid: true}, ObservedAt: sql.NullTime{Time: time.Now(), Valid: true}, ResultDatasetID: sql.NullString{String: d.ID, Valid: true}, ResultDatasetVersion: sql.NullInt64{Int64: int64(d.Version), Valid: true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Add(t.Context(), d.ID, d.Version, addition("newer", "d")); err != nil {
		t.Fatal(err)
	}
	e := tmprl.Execution{WorkflowID: a.WorkflowID, RunID: a.RunID.String}
	state, err := service.GetRun(t.Context(), e)
	if err != nil || !state.DatasetAvailable || state.PreparedDatasetVersion != 2 || state.DatasetRecordCount != 1 {
		t.Fatalf("catalog selected head instead of exact result: %+v %v", state, err)
	}
	// The run names what it adds to and what it read, from the SQL job row.
	if state.TargetDatasetID != d.ID || state.TargetDatasetName != "Catalog output" || state.BaseVersion != 1 || state.SourceDocument != "harbor-manual.md" {
		t.Fatalf("run identity: target %q %q, base %d, source %q", state.TargetDatasetID, state.TargetDatasetName, state.BaseVersion, state.SourceDocument)
	}
	listed, _, err := service.ListCatalogRuns(t.Context(), datasets.RunFilter{PageSize: 25, Mode: "preparation"})
	if err != nil || len(listed) != 1 || listed[0].TargetDatasetName != "Catalog output" || listed[0].SourceDocument != "harbor-manual.md" || listed[0].BaseVersion != 1 {
		t.Fatalf("listed run identity: %+v %v", listed, err)
	}
	// A restarted process rediscovers the same exact result without
	// republishing it.
	restarted := datasets.New(files, nil, db)
	if err := restarted.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	state, err = restarted.GetRun(t.Context(), e)
	if err != nil || !state.DatasetAvailable || state.PreparedDatasetID != d.ID || state.PreparedDatasetVersion != 2 {
		t.Fatalf("restart lost exact result: %+v %v", state, err)
	}
	// Without an uploads store the source stays unnamed; the read still succeeds.
	if state.TargetDatasetName != "Catalog output" || state.SourceDocument != "" {
		t.Fatalf("restart identity: %q %q", state.TargetDatasetName, state.SourceDocument)
	}
	if versions, err := db.ListVersions(t.Context(), d.ID); err != nil || len(versions) != 3 {
		t.Fatalf("restart changed catalog versions: %d %v", len(versions), err)
	}
	all, err := restarted.List(t.Context())
	if err != nil || len(all) != 1 {
		t.Fatalf("restart duplicated datasets: %d %v", len(all), err)
	}
	path := filepath.Join(files.Path(), d.ID, "2.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(original, ' '), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.GetRun(t.Context(), e); err == nil {
		t.Fatal("changed saved output advertised as available")
	}
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".missing"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetRun(t.Context(), e); err == nil {
		t.Fatal("missing saved output advertised as available")
	}
}
