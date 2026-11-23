package moxiedb_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/moxiedb"
	"github.com/jackc/pgx/v5/pgconn"
)

//go:embed migrations/001_init.sql
var migration []byte

//go:embed migrations/002_deletions.sql
var deletionMigration []byte

func TestIsUnavailable(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{"connection", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}, true},
		{"truncated-data", io.ErrUnexpectedEOF, false},
		{"missing-file", &os.PathError{Op: "open", Path: "missing.json", Err: os.ErrNotExist}, false},
		{"shutdown", &pgconn.PgError{Code: "57P01"}, true},
		{"restarting", &pgconn.PgError{Code: "57P03"}, true},
		{"connection-failure", &pgconn.PgError{Code: "08006"}, true},
		{"capacity", &pgconn.PgError{Code: "53300"}, true},
		{"constraint", &pgconn.PgError{Code: "23505"}, false},
		{"missing-schema", &pgconn.PgError{Code: "42P01"}, false},
		{"canceled", context.Canceled, false},
		{"success", nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.err
			if err != nil {
				err = fmt.Errorf("catalog query: %w", err)
			}
			if got := moxiedb.IsUnavailable(err); got != test.want {
				t.Errorf("IsUnavailable: got %t, want %t", got, test.want)
			}
		})
	}
}

func TestOpenInvalidConfiguration(t *testing.T) {
	const marker = "password-marker-must-not-leak"
	for _, input := range []string{"", " ", "postgres://user:" + marker + "@localhost:invalid-port/database"} {
		db, err := moxiedb.Open(t.Context(), input)
		if err == nil {
			db.Close()
			t.Fatal("expected invalid configuration error")
		}
		if strings.Contains(err.Error(), marker) {
			t.Fatal("malformed configuration exposed credentials")
		}
		if err.Error() != "moxiedb: invalid connection configuration" {
			t.Fatalf("unexpected safe error: %v", err)
		}
	}
}

// Tests opt in explicitly: either create a disposable container or use an explicitly
// supplied test database. Even the latter gets a unique, disposable schema.
func testDB(t *testing.T) (*moxiedb.DB, *sql.DB) {
	t.Helper()
	u := os.Getenv("MOXIEDB_TEST_URL")
	if u == "" && os.Getenv("MOXIEDB_TEST_DOCKER") != "1" {
		t.Skip("set MOXIEDB_TEST_DOCKER=1 or MOXIEDB_TEST_URL to an isolated test database")
	}
	if u == "" {
		out, err := exec.Command("docker", "run", "--rm", "-d", "-e", "POSTGRES_PASSWORD=test", "-p", "127.0.0.1::5432", "postgres:16-alpine").Output()
		if err != nil {
			t.Fatalf("start disposable postgres: %v: %s", err, out)
		}
		id := strings.TrimSpace(string(out))
		t.Cleanup(func() {
			if out, err := exec.Command("docker", "rm", "-f", "-v", id).CombinedOutput(); err != nil {
				t.Errorf("remove disposable postgres: %v: %s", err, out)
			}
		})
		out, err = exec.Command("docker", "port", id, "5432/tcp").CombinedOutput()
		if err != nil {
			t.Fatalf("disposable postgres port: %v: %s", err, out)
		}
		u = "postgres://postgres:test@" + strings.TrimSpace(string(out)) + "/postgres?sslmode=disable"
	}
	admin, err := sql.Open("pgx", u)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	for {
		if err = admin.PingContext(ctx); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(err)
		case <-time.After(100 * time.Millisecond):
		}
	}
	schema := fmt.Sprintf("moxiedb_test_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	q.Set("search_path", schema)
	parsed.RawQuery = q.Encode()
	setup, err := sql.Open("pgx", parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = setup.Close() })
	install, err := setup.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer install.Rollback()
	for i, sql := range [][]byte{migration, deletionMigration} {
		if _, err := install.ExecContext(ctx, string(sql)); err != nil {
			t.Fatal(err)
		}
		if _, err := install.ExecContext(ctx, "INSERT INTO schema_migrations(version,checksum) VALUES ($1,$2)", i+1, fmt.Sprintf("%x", sha256.Sum256(sql))); err != nil {
			t.Fatal(err)
		}
	}
	if err := install.Commit(); err != nil {
		t.Fatal(err)
	}
	db, err := moxiedb.Open(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, setup
}

func TestSchema(t *testing.T) {
	db, setup := testDB(t)
	if err := db.CheckSchema(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := setup.ExecContext(t.Context(), "UPDATE schema_migrations SET checksum = repeat('0',64)"); err != nil {
		t.Fatal(err)
	}
	if err := db.CheckSchema(t.Context()); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("checksum mismatch: %v", err)
	}
	if _, err := setup.ExecContext(t.Context(), "DELETE FROM schema_migrations"); err != nil {
		t.Fatal(err)
	}
	if err := db.CheckSchema(t.Context()); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("missing migration: %v", err)
	}
	if _, err := setup.ExecContext(t.Context(), "INSERT INTO schema_migrations(version,checksum) VALUES (2,repeat('0',64))"); err != nil {
		t.Fatal(err)
	}
	if err := db.CheckSchema(t.Context()); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("future migration: %v", err)
	}
	if _, err := setup.ExecContext(t.Context(), "DELETE FROM schema_migrations"); err != nil {
		t.Fatal(err)
	}
	for i, sql := range [][]byte{migration, deletionMigration} {
		if _, err := setup.ExecContext(t.Context(), "INSERT INTO schema_migrations(version,checksum) VALUES ($1,$2)", i+1, fmt.Sprintf("%x", sha256.Sum256(sql))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := setup.ExecContext(t.Context(), "DROP TABLE catalog_initialization"); err != nil {
		t.Fatal(err)
	}
	if err := db.CheckSchema(t.Context()); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("damaged schema: %v", err)
	}
}

func fixture(t *testing.T, db *moxiedb.DB, n int, base int) (moxiedb.Dataset, moxiedb.Version, []moxiedb.Document) {
	t.Helper()
	p := moxiedb.Publication{ID: fmt.Sprintf("p%d", n), DatasetID: "data", Kind: "import", InputDigest: strings.Repeat("a", 64), BaseVersion: base}
	if err := db.BeginPublication(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	d := moxiedb.Dataset{ID: "data", Name: "Data", CurrentVersion: n}
	v := moxiedb.Version{DatasetID: d.ID, Version: n, SnapshotPath: fmt.Sprintf("data/v%d.jsonl", n), SnapshotSHA256: strings.Repeat("b", 64), SnapshotBytes: 100, RecordCount: 1, TrainingReady: true, PublicationID: p.ID}
	docs := []moxiedb.Document{{ID: "doc", Name: "Doc", SourceSHA256: strings.Repeat("c", 64), Position: 0, PageStart: 1, PageEnd: 2, RecordCount: 1}}
	return d, v, docs
}

func TestDatasets(t *testing.T) {
	db, _ := testDB(t)
	ctx := t.Context()
	d, v, docs := fixture(t, db, 1, 0)
	intent, err := db.GetPublication(ctx, "p1")
	if err != nil || intent.TargetVersion != 0 || intent.SnapshotPath != "" || intent.SnapshotSHA256 != "" {
		t.Fatalf("unknown outcome: %+v %v", intent, err)
	}
	if err := db.CommitVersion(ctx, d, v, docs); err != nil {
		t.Fatal(err)
	}
	if err := db.CommitVersion(ctx, d, v, docs); err != nil {
		t.Fatalf("replay: %v", err)
	}
	p, err := db.GetPublication(ctx, "p1")
	if err != nil || p.State != "committed" {
		t.Fatalf("publication: %+v %v", p, err)
	}
	if err := db.BeginPublication(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := db.BeginPublication(ctx, intent); err != nil {
		t.Fatalf("pre-file request replay after commit: %v", err)
	}
	p.InputDigest = strings.Repeat("d", 64)
	if err := db.BeginPublication(ctx, p); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("intent identity: %v", err)
	}
	v.SnapshotSHA256 = strings.Repeat("e", 64)
	if err := db.CommitVersion(ctx, d, v, docs); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("hash conflict: %v", err)
	}
	v.SnapshotSHA256 = strings.Repeat("b", 64)
	changed := append([]moxiedb.Document{}, docs...)
	changed[0].Name = "Changed"
	if err := db.CommitVersion(ctx, d, v, changed); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("documents conflict: %v", err)
	}
	d2, v2, docs2 := fixture(t, db, 2, 1)
	if err := db.CommitVersion(ctx, d2, v2, docs2); err != nil {
		t.Fatal(err)
	}
	if err := db.CommitVersion(ctx, d, v, docs); err != nil {
		t.Fatalf("old replay: %v", err)
	}
	head, err := db.GetVersion(ctx, "data", 0)
	if err != nil || head.Version != 2 {
		t.Fatalf("head regression: %+v %v", head, err)
	}
	versions, err := db.ListVersions(ctx, "data")
	if err != nil || len(versions) != 2 || versions[0].Version != 2 {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	list, err := db.ListDatasets(ctx)
	if err != nil || len(list) != 1 || list[0].CurrentVersion != 2 {
		t.Fatalf("list: %+v %v", list, err)
	}
	if _, err := db.GetVersion(ctx, "missing", 0); !errors.Is(err, moxiedb.ErrNotFound) {
		t.Fatal(err)
	}
	d3, v3, docs3 := fixture(t, db, 3, 1)
	if err := db.CommitVersion(ctx, d3, v3, docs3); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("stale base: %v", err)
	}
	if err := db.MarkPublication(ctx, "p3", "needs_attention", "stale_base"); err != nil {
		t.Fatal(err)
	}
	pending, err := db.PendingPublications(ctx)
	if err != nil || len(pending) != 1 || pending[0].State != "needs_attention" {
		t.Fatalf("pending: %+v %v", pending, err)
	}
	if err := db.MarkPublication(ctx, "p3", "conflict", "settled"); err != nil {
		t.Fatal(err)
	}
	pending, err = db.PendingPublications(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("settled conflicts in pending: %+v %v", pending, err)
	}
	if err := db.MarkPublication(ctx, "p1", "conflict", "late"); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatal(err)
	}
	if err := db.MarkCatalogReady(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.MarkCatalogReady(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestJobs(t *testing.T) {
	db, _ := testDB(t)
	ctx := t.Context()
	d, v, docs := fixture(t, db, 1, 0)
	if err := db.CommitVersion(ctx, d, v, docs); err != nil {
		t.Fatal(err)
	}
	j := moxiedb.Job{JobID: "job", Mode: "preparation", SourceUploadID: sql.NullString{String: "upload", Valid: true}}
	if err := db.CreateJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateJob(ctx, j); err != nil {
		t.Fatalf("job replay: %v", err)
	}
	changed := j
	changed.SourceUploadID.String = "other"
	if err := db.CreateJob(ctx, changed); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("changed job input: %v", err)
	}
	training := moxiedb.Job{JobID: "training", Mode: "training", TargetDatasetID: sql.NullString{String: d.ID, Valid: true}, BaseDatasetVersion: sql.NullInt64{Int64: 1, Valid: true}}
	if err := db.CreateJob(ctx, training); err != nil {
		t.Fatal(err)
	}
	invalid := training
	invalid.JobID = "invalid"
	invalid.BaseDatasetVersion = sql.NullInt64{}
	if err := db.CreateJob(ctx, invalid); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("unpaired target: %v", err)
	}
	invalid.BaseDatasetVersion = sql.NullInt64{Int64: 99, Valid: true}
	if err := db.CreateJob(ctx, invalid); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("unknown target: %v", err)
	}
	a := moxiedb.Attempt{AttemptID: "a1", JobID: j.JobID, AttemptNumber: 1, TemporalNamespace: "ns", WorkflowID: "workflow"}
	if err := db.CreateAttempt(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateAttempt(ctx, a); err != nil {
		t.Fatalf("attempt replay: %v", err)
	}
	initial, err := db.GetAttempt(ctx, a.AttemptID)
	if err != nil || initial.RunID.Valid || initial.ExecutionStatus.Valid || initial.Observation != nil || initial.ObservationRevision != 0 {
		t.Fatalf("nullable attempt: %+v %v", initial, err)
	}
	bad := a
	bad.WorkflowID = "different"
	if err := db.CreateAttempt(ctx, bad); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("changed attempt input: %v", err)
	}
	bad = a
	bad.AttemptID = "duplicate-number"
	if err := db.CreateAttempt(ctx, bad); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("attempt number: %v", err)
	}
	bad = a
	bad.AttemptID = "wrong-job"
	bad.JobID = training.JobID
	bad.PreviousAttemptID = sql.NullString{String: a.AttemptID, Valid: true}
	if err := db.CreateAttempt(ctx, bad); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("cross-job lineage: %v", err)
	}
	bad = a
	bad.AttemptID = "self"
	bad.AttemptNumber = 2
	bad.PreviousAttemptID = sql.NullString{String: "self", Valid: true}
	if err := db.CreateAttempt(ctx, bad); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("self lineage: %v", err)
	}
	if err := db.LinkAttemptRun(ctx, a.AttemptID, "run1"); err != nil {
		t.Fatal(err)
	}
	if err := db.LinkAttemptRun(ctx, a.AttemptID, "run1"); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateAttempt(ctx, a); err != nil {
		t.Fatalf("pre-run replay: %v", err)
	}
	if err := db.LinkAttemptRun(ctx, a.AttemptID, "different"); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("run relink: %v", err)
	}
	a2 := a
	a2.AttemptID = "a2"
	a2.AttemptNumber = 2
	a2.PreviousAttemptID = sql.NullString{String: a.AttemptID, Valid: true}
	if err := db.CreateAttempt(ctx, a2); err != nil {
		t.Fatal(err)
	}
	if err := db.LinkAttemptRun(ctx, a2.AttemptID, "run1"); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("exact execution uniqueness: %v", err)
	}
	if err := db.LinkAttemptRun(ctx, a2.AttemptID, "run2"); err != nil {
		t.Fatal(err)
	}
	other := a
	other.AttemptID = "other-ns"
	other.JobID = training.JobID
	other.TemporalNamespace = "other-ns"
	other.RunID = sql.NullString{String: "run1", Valid: true}
	if err := db.CreateAttempt(ctx, other); err != nil {
		t.Fatalf("namespace identity: %v", err)
	}
	started := time.Unix(100, 0).UTC()
	o := moxiedb.AttemptObservation{ExecutionStatus: sql.NullString{String: "custom_runner_status", Valid: true}, StartedAt: sql.NullTime{Time: started, Valid: true}, ObservedAt: sql.NullTime{Time: started.Add(time.Second), Valid: true}, Observation: []byte(`{"progress":1}`), ResultDatasetID: sql.NullString{String: d.ID, Valid: true}, ResultDatasetVersion: sql.NullInt64{Int64: 1, Valid: true}, StateStale: true}
	if err := db.UpdateAttemptObservation(ctx, a.AttemptID, 0, o); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateAttemptObservation(ctx, a.AttemptID, 0, o); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("revision guard: %v", err)
	}
	got, err := db.GetAttempt(ctx, a.AttemptID)
	if err != nil || got.ObservationRevision != 1 || !got.StateStale || got.RunID.String != "run1" || got.ResultDatasetVersion.Int64 != 1 || !got.StartedAt.Time.Equal(started) {
		t.Fatalf("observation: %+v %v", got, err)
	}
	for _, payload := range []string{`{`, `{"big":"` + strings.Repeat("x", moxiedb.MaxPayloadBytes) + `"}`} {
		bad := o
		bad.Observation = []byte(payload)
		if err := db.UpdateAttemptObservation(ctx, a.AttemptID, 1, bad); !errors.Is(err, moxiedb.ErrConflict) {
			t.Fatalf("observation payload bound: %v", err)
		}
	}
	badObservation := o
	badObservation.ErrorCode = sql.NullString{String: strings.Repeat("x", 129), Valid: true}
	if err := db.UpdateAttemptObservation(ctx, a.AttemptID, 1, badObservation); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("error code bound: %v", err)
	}
	badObservation = o
	badObservation.ErrorMessage = sql.NullString{String: strings.Repeat("x", 2049), Valid: true}
	if err := db.UpdateAttemptObservation(ctx, a.AttemptID, 1, badObservation); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("error message bound: %v", err)
	}
	badObservation = o
	badObservation.ResultDatasetVersion.Int64 = 99
	if err := db.UpdateAttemptObservation(ctx, a.AttemptID, 1, badObservation); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("result version FK: %v", err)
	}
	if err := db.UpdateAttemptObservation(ctx, a2.AttemptID, 0, o); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateAttemptObservation(ctx, a.AttemptID, 1, moxiedb.AttemptObservation{Observation: []byte(`null`)}); err != nil {
		t.Fatal(err)
	}
	got, err = db.GetAttempt(ctx, a.AttemptID)
	if err != nil || string(got.Observation) != "null" {
		t.Fatalf("JSON null: %+v %v", got, err)
	}
	if err := db.UpdateAttemptObservation(ctx, a.AttemptID, 2, moxiedb.AttemptObservation{}); err != nil {
		t.Fatal(err)
	}
	got, err = db.GetAttempt(ctx, a.AttemptID)
	if err != nil || got.Observation != nil || got.ExecutionStatus.Valid {
		t.Fatalf("SQL NULL: %+v %v", got, err)
	}
	results := make(chan error, 2)
	for range 2 {
		go func() { results <- db.UpdateAttemptObservation(ctx, a.AttemptID, 3, o) }()
	}
	wins, conflicts := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			wins++
		case errors.Is(err, moxiedb.ErrConflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("concurrent observation CAS: %d wins %d conflicts", wins, conflicts)
	}
	got, err = db.GetAttempt(ctx, a.AttemptID)
	if err != nil || got.ObservationRevision != 4 {
		t.Fatalf("concurrent revision: %+v %v", got, err)
	}
}

func TestCatalog(t *testing.T) {
	db, setup := testDB(t)
	ctx := t.Context()
	d, v, docs := fixture(t, db, 1, 0)
	if err := db.CommitVersion(ctx, d, v, docs); err != nil {
		t.Fatal(err)
	}
	text := func(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }
	jobs := []moxiedb.Job{
		{JobID: "a-preparation", Mode: "preparation"},
		{JobID: "b-training", Mode: "training", TargetDatasetID: text(d.ID), BaseDatasetVersion: sql.NullInt64{Int64: 1, Valid: true}},
		{JobID: "c-combined", Mode: "preparation_and_training"},
		{JobID: "d-unlinked", Mode: "preparation"},
		{JobID: "e-other-namespace", Mode: "preparation"},
		{JobID: "f-no-attempt", Mode: "preparation"},
	}
	for _, j := range jobs {
		if err := db.CreateJob(ctx, j); err != nil {
			t.Fatal(err)
		}
	}
	// Deliberate timestamp ties exercise the public ordering contract, independently
	// of timing between inserts. This modifies only this test's disposable schema.
	if _, err := setup.ExecContext(ctx, `UPDATE jobs SET created_at='2026-01-01T00:00:00Z'`); err != nil {
		t.Fatal(err)
	}
	attempts := []moxiedb.Attempt{
		{AttemptID: "a-old", JobID: jobs[0].JobID, AttemptNumber: 1, TemporalNamespace: "ns", WorkflowID: "shared", RunID: text("old")},
		{AttemptID: "a-new", JobID: jobs[0].JobID, AttemptNumber: 2, PreviousAttemptID: text("a-old"), TemporalNamespace: "ns", WorkflowID: "shared", RunID: text("new")},
		{AttemptID: "b", JobID: jobs[1].JobID, AttemptNumber: 1, TemporalNamespace: "ns", WorkflowID: "training", RunID: text("training-run")},
		{AttemptID: "c", JobID: jobs[2].JobID, AttemptNumber: 1, TemporalNamespace: "ns", WorkflowID: "combined", RunID: text("combined-run")},
		{AttemptID: "d", JobID: jobs[3].JobID, AttemptNumber: 1, TemporalNamespace: "ns", WorkflowID: "unlinked"},
		{AttemptID: "e", JobID: jobs[4].JobID, AttemptNumber: 1, TemporalNamespace: "elsewhere", WorkflowID: "shared", RunID: text("old")},
	}
	for _, a := range attempts {
		if err := db.CreateAttempt(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	for id, status := range map[string]string{"a-old": "completed", "a-new": "running", "b": "completed", "c": "paused", "e": "running"} {
		if err := db.UpdateAttemptObservation(ctx, id, 0, moxiedb.AttemptObservation{ExecutionStatus: text(status), Observation: []byte(`{"progress":1}`), StateStale: true}); err != nil {
			t.Fatal(err)
		}
	}
	// Both catalog APIs must associate pending intents with the selected attempt,
	// never another attempt of the same job or another namespace.
	for _, item := range []struct{ id, attempt, state string }{
		{"old-pending", "a-old", "pending"},
		{"new-pending", "a-new", "pending"},
		{"new-attention", "a-new", "needs_attention"},
		{"new-accepted", "a-new", "accepted"},
		{"new-rejected", "a-new", "rejected"},
		{"elsewhere-pending", "e", "pending"},
	} {
		a, err := db.GetAttempt(ctx, item.attempt)
		if err != nil {
			t.Fatal(err)
		}
		i := moxiedb.Intent{IntentID: item.id, JobID: a.JobID, AttemptID: a.AttemptID, Kind: "stop", Payload: []byte(`{"reason":"test"}`), PayloadSHA256: strings.Repeat("a", 64)}
		if err := db.BeginIntent(ctx, i); err != nil {
			t.Fatal(err)
		}
		if item.state != "pending" {
			if err := db.SettleIntent(ctx, i.IntentID, item.state, sql.NullString{}); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, tc := range []struct {
		name   string
		filter moxiedb.CatalogFilter
		want   string
	}{
		{"all", moxiedb.CatalogFilter{Namespace: "ns"}, "d,c,b,a-new"},
		{"empty status slice", moxiedb.CatalogFilter{Namespace: "ns", Statuses: []string{}}, "d,c,b,a-new"},
		{"latest not historical", moxiedb.CatalogFilter{Namespace: "ns", Statuses: []string{"completed"}}, "b"},
		{"multiple statuses", moxiedb.CatalogFilter{Namespace: "ns", Statuses: []string{"running", "paused"}}, "c,a-new"},
		{"unobserved only", moxiedb.CatalogFilter{Namespace: "ns", Statuses: []string{""}}, "d"},
		{"preparation only", moxiedb.CatalogFilter{Namespace: "ns", Mode: "preparation"}, "d,a-new"},
		{"training only", moxiedb.CatalogFilter{Namespace: "ns", Mode: "training"}, "b"},
		{"combined only", moxiedb.CatalogFilter{Namespace: "ns", Mode: "preparation_and_training"}, "c"},
		{"target only", moxiedb.CatalogFilter{Namespace: "ns", TargetDatasetID: d.ID}, "b"},
		{"mode and target intersect", moxiedb.CatalogFilter{Namespace: "ns", Mode: "preparation", TargetDatasetID: d.ID}, ""},
		{"all filters", moxiedb.CatalogFilter{Namespace: "ns", Mode: "training", TargetDatasetID: d.ID, Statuses: []string{"completed"}, Limit: 1}, "b"},
		{"status before pagination", moxiedb.CatalogFilter{Namespace: "ns", Statuses: []string{"running"}, Limit: 1}, "a-new"},
		{"mode before pagination", moxiedb.CatalogFilter{Namespace: "ns", Mode: "training", Limit: 1}, "b"},
		{"target before pagination", moxiedb.CatalogFilter{Namespace: "ns", TargetDatasetID: d.ID, Limit: 1}, "b"},
		{"page one", moxiedb.CatalogFilter{Namespace: "ns", Limit: 2}, "d,c"},
		{"page two", moxiedb.CatalogFilter{Namespace: "ns", Limit: 2, Offset: 2}, "b,a-new"},
		{"oldest first", moxiedb.CatalogFilter{Namespace: "ns", Oldest: true}, "a-new,b,c,d"},
		{"oldest page two after filter", moxiedb.CatalogFilter{Namespace: "ns", Mode: "preparation", Oldest: true, Limit: 1, Offset: 1}, "d"},
		{"offset after filter", moxiedb.CatalogFilter{Namespace: "ns", Statuses: []string{"running", "paused"}, Limit: 1, Offset: 1}, "a-new"},
		{"past end", moxiedb.CatalogFilter{Namespace: "ns", Offset: 4}, ""},
		{"namespace isolation", moxiedb.CatalogFilter{Namespace: "elsewhere"}, "e"},
		{"unknown namespace", moxiedb.CatalogFilter{Namespace: "missing"}, ""},
		{"unknown status", moxiedb.CatalogFilter{Namespace: "ns", Statuses: []string{"unknown"}}, ""},
		{"maximum limit", moxiedb.CatalogFilter{Namespace: "ns", Limit: 1000}, "d,c,b,a-new"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entries, err := db.ListCatalog(ctx, tc.filter)
			if err != nil {
				t.Fatal(err)
			}
			ids := []string{}
			for _, e := range entries {
				ids = append(ids, e.Attempt.AttemptID)
				if e.Job.JobID != e.Attempt.JobID || e.Job.CreatedAt.IsZero() {
					t.Fatalf("job metadata: %+v", e)
				}
				if e.Attempt.AttemptID == "a-new" && (len(e.Pending) != 2 || e.Pending[0].IntentID != "new-pending" || e.Pending[1].IntentID != "new-attention" || e.Pending[1].State != "needs_attention" || len(e.Pending[0].Payload) == 0) {
					t.Fatalf("selected attempt intents: %+v", e.Pending)
				}
				if e.Attempt.AttemptID == "d" && (e.Attempt.RunID.Valid || e.Attempt.ExecutionStatus.Valid || e.Attempt.Observation != nil || len(e.Pending) != 0) {
					t.Fatalf("unlinked/unobserved state: %+v", e)
				}
			}
			if got := strings.Join(ids, ","); got != tc.want {
				t.Fatalf("attempts %q, want %q", got, tc.want)
			}
		})
	}
	for _, f := range []moxiedb.CatalogFilter{{}, {Namespace: "ns", Limit: -1}, {Namespace: "ns", Limit: 1001}, {Namespace: "ns", Offset: -1}} {
		if _, err := db.ListCatalog(ctx, f); !errors.Is(err, moxiedb.ErrConflict) {
			t.Fatalf("invalid filter %+v: %v", f, err)
		}
	}
	for _, tc := range []struct{ namespace, workflow, run, attempt string }{
		{"ns", "shared", "old", "a-old"},
		{"ns", "shared", "new", "a-new"},
		{"elsewhere", "shared", "old", "e"},
	} {
		e, err := db.GetCatalogExecution(ctx, tc.namespace, tc.workflow, tc.run)
		if err != nil || e.Attempt.AttemptID != tc.attempt || !e.Attempt.StateStale || e.Attempt.ObservationRevision != 1 || len(e.Attempt.Observation) == 0 {
			t.Fatalf("exact %+v: %+v %v", tc, e, err)
		}
		if tc.attempt == "a-old" && (len(e.Pending) != 1 || e.Pending[0].IntentID != "old-pending") {
			t.Fatalf("historical pending: %+v", e.Pending)
		}
		if tc.attempt == "a-new" && len(e.Pending) != 2 {
			t.Fatalf("latest pending: %+v", e.Pending)
		}
	}
	for _, tc := range []struct {
		namespace, workflow, run string
		want                     error
	}{
		{"", "shared", "old", moxiedb.ErrConflict},
		{"ns", "shared", "", moxiedb.ErrConflict},
		{"ns", "unlinked", "", moxiedb.ErrConflict},
		{"ns", "shared", "missing", moxiedb.ErrNotFound},
		{"missing", "shared", "old", moxiedb.ErrNotFound},
		{"ns", "missing", "old", moxiedb.ErrNotFound},
		{"ns", "", "old", moxiedb.ErrNotFound},
		{"ns", "unlinked", "missing", moxiedb.ErrNotFound},
	} {
		if _, err := db.GetCatalogExecution(ctx, tc.namespace, tc.workflow, tc.run); !errors.Is(err, tc.want) {
			t.Fatalf("exact %+v: %v", tc, err)
		}
	}
	if _, err := setup.ExecContext(ctx, `UPDATE jobs SET created_at='2026-01-02T00:00:00Z' WHERE job_id=$1`, jobs[0].JobID); err != nil {
		t.Fatal(err)
	}
	entries, err := db.ListCatalog(ctx, moxiedb.CatalogFilter{Namespace: "ns", Limit: 1})
	if err != nil || len(entries) != 1 || entries[0].Attempt.AttemptID != "a-new" {
		t.Fatalf("created_at takes precedence over job_id: %+v %v", entries, err)
	}
	// Latest means highest number globally, not the latest matching namespace.
	if err := db.CreateAttempt(ctx, moxiedb.Attempt{AttemptID: "a-third", JobID: jobs[0].JobID, AttemptNumber: 3, PreviousAttemptID: text("a-new"), TemporalNamespace: "elsewhere", WorkflowID: "shared"}); err != nil {
		t.Fatal(err)
	}
	entries, err = db.ListCatalog(ctx, moxiedb.CatalogFilter{Namespace: "ns", Statuses: []string{"running"}})
	if err != nil || len(entries) != 0 {
		t.Fatalf("no historical namespace fallback: %+v %v", entries, err)
	}
}

func TestCatalogDefaultPage(t *testing.T) {
	db, _ := testDB(t)
	ctx := t.Context()
	for n := range 101 {
		id := fmt.Sprintf("job-%03d", n)
		if err := db.CreateJob(ctx, moxiedb.Job{JobID: id, Mode: "preparation"}); err != nil {
			t.Fatal(err)
		}
		if err := db.CreateAttempt(ctx, moxiedb.Attempt{AttemptID: id, JobID: id, AttemptNumber: 1, TemporalNamespace: "ns", WorkflowID: id}); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ limit, offset, count int }{{0, 0, 100}, {0, 100, 1}, {1000, 0, 101}} {
		entries, err := db.ListCatalog(ctx, moxiedb.CatalogFilter{Namespace: "ns", Limit: tc.limit, Offset: tc.offset})
		if err != nil || len(entries) != tc.count {
			t.Fatalf("page %+v: got %d entries, %v", tc, len(entries), err)
		}
	}
}

func TestJobIntents(t *testing.T) {
	db, _ := testDB(t)
	ctx := t.Context()
	for _, id := range []string{"job", "other"} {
		if err := db.CreateJob(ctx, moxiedb.Job{JobID: id, Mode: "preparation_and_training"}); err != nil {
			t.Fatal(err)
		}
	}
	a := moxiedb.Attempt{AttemptID: "attempt", JobID: "job", AttemptNumber: 1, TemporalNamespace: "ns", WorkflowID: "workflow"}
	if err := db.CreateAttempt(ctx, a); err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"choice":"accept"}`)
	i := moxiedb.Intent{IntentID: "intent", JobID: a.JobID, AttemptID: a.AttemptID, Kind: "preview_review", CheckpointID: sql.NullString{String: "checkpoint", Valid: true}, Payload: payload, PayloadSHA256: fmt.Sprintf("%x", sha256.Sum256(payload))}
	if err := db.BeginIntent(ctx, i); err != nil {
		t.Fatal(err)
	}
	if err := db.BeginIntent(ctx, i); err != nil {
		t.Fatalf("intent replay: %v", err)
	}
	got, err := db.GetIntent(ctx, i.IntentID)
	if err != nil || got.State != "pending" || got.CreatedAt.IsZero() || got.ErrorCode.Valid {
		t.Fatalf("intent: %+v %v", got, err)
	}
	for _, change := range []moxiedb.Intent{
		{IntentID: i.IntentID, JobID: i.JobID, AttemptID: i.AttemptID, Kind: i.Kind, CheckpointID: i.CheckpointID, Payload: []byte(`{"choice":"reject"}`), PayloadSHA256: i.PayloadSHA256},
		{IntentID: i.IntentID, JobID: i.JobID, AttemptID: i.AttemptID, Kind: i.Kind, CheckpointID: i.CheckpointID, Payload: i.Payload, PayloadSHA256: strings.Repeat("a", 64)},
	} {
		if err := db.BeginIntent(ctx, change); !errors.Is(err, moxiedb.ErrConflict) {
			t.Fatalf("changed command input: %v", err)
		}
	}
	bad := i
	bad.IntentID = "cross-job"
	bad.JobID = "other"
	if err := db.BeginIntent(ctx, bad); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("cross-job intent: %v", err)
	}
	for _, kind := range []string{"preview_review", "authorize_training"} {
		bad = i
		bad.IntentID = kind
		bad.Kind = kind
		bad.CheckpointID = sql.NullString{}
		if err := db.BeginIntent(ctx, bad); !errors.Is(err, moxiedb.ErrConflict) {
			t.Fatalf("missing checkpoint for %s: %v", kind, err)
		}
	}
	bad = i
	bad.IntentID = "invalid-digest"
	bad.PayloadSHA256 = "not-hex"
	if err := db.BeginIntent(ctx, bad); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("digest constraint: %v", err)
	}
	for n, payload := range []string{`{`, `{"large":"` + strings.Repeat("x", moxiedb.MaxPayloadBytes) + `"}`} {
		bad = i
		bad.IntentID = fmt.Sprintf("payload%d", n)
		bad.Payload = []byte(payload)
		if err := db.BeginIntent(ctx, bad); !errors.Is(err, moxiedb.ErrConflict) {
			t.Fatalf("payload bound: %v", err)
		}
	}
	second := i
	second.IntentID = "second"
	second.Kind = "stop"
	second.CheckpointID = sql.NullString{}
	if err := db.BeginIntent(ctx, second); err != nil {
		t.Fatal(err)
	}
	pending, err := db.PendingIntents(ctx)
	if err != nil || len(pending) != 2 || pending[0].IntentID != i.IntentID {
		t.Fatalf("intent ordering: %+v %v", pending, err)
	}
	if err := db.SettleIntent(ctx, i.IntentID, "accepted", sql.NullString{}); err != nil {
		t.Fatal(err)
	}
	if err := db.SettleIntent(ctx, i.IntentID, "accepted", sql.NullString{}); err != nil {
		t.Fatalf("settlement replay: %v", err)
	}
	if err := db.SettleIntent(ctx, i.IntentID, "rejected", sql.NullString{}); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("settled intent changed: %v", err)
	}
	if err := db.BeginIntent(ctx, i); err != nil {
		t.Fatalf("command replay after acceptance: %v", err)
	}
	if err := db.SettleIntent(ctx, second.IntentID, "needs_attention", sql.NullString{String: "unavailable", Valid: true}); err != nil {
		t.Fatal(err)
	}
	pending, err = db.PendingIntents(ctx)
	if err != nil || len(pending) != 1 || pending[0].IntentID != second.IntentID || pending[0].State != "needs_attention" {
		t.Fatalf("recoverable intent filter: %+v %v", pending, err)
	}
	if err := db.SettleIntent(ctx, second.IntentID, "rejected", sql.NullString{String: strings.Repeat("x", 129), Valid: true}); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("intent code bound: %v", err)
	}
	if err := db.SettleIntent(ctx, second.IntentID, "rejected", sql.NullString{String: "invalid", Valid: true}); err != nil {
		t.Fatal(err)
	}
	pending, err = db.PendingIntents(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("settled intent filter: %+v %v", pending, err)
	}
	if _, err := db.GetIntent(ctx, "missing"); !errors.Is(err, moxiedb.ErrNotFound) {
		t.Fatal(err)
	}
}

func TestPublicationConstraintsAndRollback(t *testing.T) {
	db, _ := testDB(t)
	ctx := t.Context()
	d, v, docs := fixture(t, db, 1, 0)
	docs = append(docs, docs[0])
	if err := db.CommitVersion(ctx, d, v, docs); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("duplicate document: %v", err)
	}
	if _, err := db.GetVersion(ctx, d.ID, 1); !errors.Is(err, moxiedb.ErrNotFound) {
		t.Fatalf("partial version survived rollback: %v", err)
	}
	list, err := db.ListDatasets(ctx)
	if err != nil || len(list) != 0 {
		t.Fatalf("partial head survived rollback: %+v %v", list, err)
	}
	p, err := db.GetPublication(ctx, v.PublicationID)
	if err != nil || p.State != "pending" {
		t.Fatalf("intent after rollback: %+v %v", p, err)
	}
	for i, path := range []string{"/absolute.jsonl", "../parent.jsonl", "x/../parent.jsonl", `C:\\file.jsonl`, `x\\file.jsonl`, ""} {
		bad := v
		bad.SnapshotPath = path
		if err := db.CommitVersion(ctx, d, bad, docs[:1]); !errors.Is(err, moxiedb.ErrConflict) {
			t.Fatalf("unsafe path %d %q accepted: %v", i, path, err)
		}
	}
	bad := v
	bad.SnapshotSHA256 = "not-a-digest"
	if err := db.CommitVersion(ctx, d, bad, docs[:1]); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("bad digest: %v", err)
	}
	badName := d
	badName.Name = strings.Repeat("é", 101)
	if err := db.CommitVersion(ctx, badName, v, docs[:1]); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("200-byte name bound: %v", err)
	}
	if err := db.CommitVersion(ctx, d, v, docs[:1]); err != nil {
		t.Fatalf("retry after rollback: %v", err)
	}
	if err := db.CommitVersion(ctx, d, v, nil); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("missing replay documents: %v", err)
	}
	d2, v2, docs2 := fixture(t, db, 2, 0)
	v2.SnapshotPath = v.SnapshotPath
	if err := db.CommitVersion(ctx, d2, v2, docs2); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("snapshot path uniqueness: %v", err)
	}
	v2.SnapshotPath = "data/v2.jsonl"
	contribution := docs2[0]
	contribution.ID = "different-document"
	contribution.Position = 1
	if err := db.CommitVersion(ctx, d2, v2, append(docs2, contribution)); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("duplicate contribution: %v", err)
	}
	docs2[0].Name = strings.Repeat("long-filename-", 30) + ".pdf"
	if err := db.CommitVersion(ctx, d2, v2, docs2); err != nil {
		t.Fatalf("retry after contribution conflict with long document filename: %v", err)
	}
}

func TestConcurrentPublication(t *testing.T) {
	db, _ := testDB(t)
	ctx := t.Context()
	d, v, docs := fixture(t, db, 1, 0)
	results := make(chan error, 8)
	for range 8 {
		go func() { results <- db.CommitVersion(ctx, d, v, docs) }()
	}
	for range 8 {
		if err := <-results; err != nil {
			t.Fatalf("concurrent replay: %v", err)
		}
	}
	d2, v2, docs2 := fixture(t, db, 2, 1)
	d3, v3, docs3 := fixture(t, db, 3, 1)
	d3.CurrentVersion, v3.Version = 2, 2
	go func() { results <- db.CommitVersion(ctx, d2, v2, docs2) }()
	go func() { results <- db.CommitVersion(ctx, d3, v3, docs3) }()
	successes, conflicts := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, moxiedb.ErrConflict):
			conflicts++
		default:
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("competing heads: %d successes %d conflicts", successes, conflicts)
	}
	head, err := db.GetVersion(ctx, d.ID, 0)
	if err != nil || (head.Version != 2 && head.Version != 3) {
		t.Fatalf("head: %+v %v", head, err)
	}
	versions, err := db.ListVersions(ctx, d.ID)
	if err != nil || len(versions) != 2 {
		t.Fatalf("versions after competition: %+v %v", versions, err)
	}
}

func TestAddFromOlderRequestedBase(t *testing.T) {
	db, _ := testDB(t)
	ctx := t.Context()
	d, v, docs := fixture(t, db, 1, 0)
	if err := db.CommitVersion(ctx, d, v, docs); err != nil {
		t.Fatal(err)
	}
	d, v, docs = fixture(t, db, 2, 0)
	if err := db.CommitVersion(ctx, d, v, docs); err != nil {
		t.Fatalf("historical import with absent base: %v", err)
	}
	p := moxiedb.Publication{ID: "add-from-v1", DatasetID: d.ID, Kind: "add", InputDigest: strings.Repeat("d", 64), BaseVersion: 1}
	if err := db.BeginPublication(ctx, p); err != nil {
		t.Fatal(err)
	}
	d.CurrentVersion, v.Version = 3, 3
	v.PublicationID, v.SnapshotPath = p.ID, "data/v3.jsonl"
	if err := db.CommitVersion(ctx, d, v, docs); err != nil {
		t.Fatalf("add requested at base1 after head2: %v", err)
	}
	got, err := db.GetPublication(ctx, p.ID)
	if err != nil || got.BaseVersion != 1 || got.TargetVersion != 3 {
		t.Fatalf("requested base identity: %+v %v", got, err)
	}
	if err := db.BeginPublication(ctx, p); err != nil {
		t.Fatal(err)
	}
	head, err := db.GetVersion(ctx, d.ID, 0)
	if err != nil || head.Version != 3 {
		t.Fatalf("head: %+v %v", head, err)
	}
}

func TestDeletionIdentity(t *testing.T) {
	db, _ := testDB(t)
	ctx := t.Context()
	d := moxiedb.Deletion{ID: "delete", Namespace: "ns", WorkflowID: "prep", RunID: "r1", DatasetID: "data", DatasetVersion: 1, Manifest: []byte(`{"executions":[{"namespace":"ns","workflow_id":"train","run_id":"r2"}],"snapshots":[]}`)}
	if err := db.BeginDeletion(ctx, d); err != nil {
		t.Fatal(err)
	}
	replay := d
	replay.Manifest = []byte(`{ "snapshots": [], "executions": [{"run_id":"r2","workflow_id":"train","namespace":"ns"}] }`)
	if err := db.BeginDeletion(ctx, replay); err != nil {
		t.Fatalf("JSONB replay: %v", err)
	}
	for _, change := range []func(*moxiedb.Deletion){
		func(d *moxiedb.Deletion) { d.ID = "another" },
		func(d *moxiedb.Deletion) { d.Namespace = "other" },
		func(d *moxiedb.Deletion) { d.WorkflowID = "other" },
		func(d *moxiedb.Deletion) { d.RunID = "other" },
		func(d *moxiedb.Deletion) { d.DatasetID = "other" },
		func(d *moxiedb.Deletion) { d.DatasetVersion = 2 },
		func(d *moxiedb.Deletion) { d.Manifest = []byte(`{}`) },
		func(d *moxiedb.Deletion) { d.State = "completed" },
		func(d *moxiedb.Deletion) { d.Manifest = []byte(`[]`) },
		func(d *moxiedb.Deletion) {
			d.Manifest = []byte(`{"executions":[{"namespace":"ns","workflow_id":"train"}]}`)
		},
	} {
		bad := d
		change(&bad)
		if err := db.BeginDeletion(ctx, bad); !errors.Is(err, moxiedb.ErrConflict) {
			t.Fatalf("identity/state accepted %+v: %v", bad, err)
		}
	}
	competing := d
	competing.ID = "competing"
	competing.RunID = "different"
	if err := db.BeginDeletion(ctx, competing); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("two pending dataset deletions: %v", err)
	}
	for _, key := range [][3]string{{"ns", "prep", "r1"}, {"ns", "train", "r2"}, {"other", "train", "r2"}, {"ns", "train", "other"}} {
		deleted, err := db.IsDeletedExecution(ctx, key[0], key[1], key[2])
		want := key[0] == "ns" && key[2] != "other"
		if err != nil || deleted != want {
			t.Fatalf("execution %v: %t %v", key, deleted, err)
		}
	}
	if err := db.SetDeletionError(ctx, d.ID, "files_unavailable"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetDeletionError(ctx, d.ID, "secret detail /path"); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("non-code error: %v", err)
	}
	stored, err := db.GetDeletion(ctx, "ns", "prep", "r1")
	if err != nil || stored.ErrorCode != "files_unavailable" || stored.State != "pending" || stored.CompletedAt.Valid {
		t.Fatalf("stored: %+v %v", stored, err)
	}
	if err := db.CompleteDeletion(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.CompleteDeletion(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.SetDeletionError(ctx, d.ID, "late_error"); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("completed mutation: %v", err)
	}
	if err := db.BeginDeletion(ctx, d); err != nil {
		t.Fatalf("completed identity replay: %v", err)
	}
	pending, err := db.PendingDeletions(ctx)
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending: %+v %v", pending, err)
	}
	stored, err = db.GetDeletion(ctx, "ns", "prep", "r1")
	if err != nil || stored.State != "completed" || stored.ErrorCode != "" || !stored.CompletedAt.Valid {
		t.Fatalf("completion: %+v %v", stored, err)
	}
	if deleted, err := db.IsDeletedExecution(ctx, "ns", "train", "r2"); err != nil || !deleted {
		t.Fatalf("completed execution fence: %t %v", deleted, err)
	}
}

func TestApplyDeletion(t *testing.T) {
	db, setup := testDB(t)
	ctx := t.Context()
	d, v1, docs := fixture(t, db, 1, 0)
	if err := db.CommitVersion(ctx, d, v1, docs); err != nil {
		t.Fatal(err)
	}
	d, v2, docs := fixture(t, db, 2, 1)
	if err := db.CommitVersion(ctx, d, v2, docs); err != nil {
		t.Fatal(err)
	}
	// An existing job's foreign key must survive removal of its input snapshot.
	j := moxiedb.Job{JobID: "existing", Mode: "training", TargetDatasetID: sql.NullString{String: d.ID, Valid: true}, BaseDatasetVersion: sql.NullInt64{Int64: 2, Valid: true}}
	if err := db.CreateJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	a := moxiedb.Attempt{AttemptID: "existing/1", JobID: j.JobID, AttemptNumber: 1, TemporalNamespace: "ns", WorkflowID: "unselected", RunID: sql.NullString{String: "r", Valid: true}}
	if err := db.CreateAttempt(ctx, a); err != nil {
		t.Fatal(err)
	}
	i := moxiedb.Intent{IntentID: "authorize", JobID: j.JobID, AttemptID: a.AttemptID, Kind: "submit", Payload: []byte(`{}`), PayloadSHA256: strings.Repeat("a", 64)}
	operation := moxiedb.Deletion{ID: "delete", Namespace: "ns", WorkflowID: "prep", RunID: "r", DatasetID: d.ID, Manifest: []byte(`{}`)}
	if err := db.BeginDeletion(ctx, operation); err != nil {
		t.Fatal(err)
	}
	j.JobID = "new"
	if err := db.CreateJob(ctx, j); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("new job during deletion: %v", err)
	}
	if err := db.BeginIntent(ctx, i); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("new intent during deletion: %v", err)
	}
	if err := db.ApplyDeletion(ctx, operation.ID, []int{2, 999}); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("missing version must rollback: %v", err)
	}
	got, err := db.GetVersion(ctx, d.ID, 1)
	if err != nil || got.SnapshotSHA256 != v1.SnapshotSHA256 {
		t.Fatalf("partial rewrite escaped rollback: %+v %v", got, err)
	}
	if err := db.ApplyDeletion(ctx, operation.ID, []int{2}); err != nil {
		t.Fatal(err)
	}
	if err := db.ApplyDeletion(ctx, operation.ID, []int{2}); err != nil {
		t.Fatalf("apply replay: %v", err)
	}
	got, err = db.GetVersion(ctx, d.ID, 0)
	if err != nil || got.Version != 1 || got.SnapshotSHA256 != v1.SnapshotSHA256 || got.RecordCount != v1.RecordCount {
		t.Fatalf("newest live: %+v %v", got, err)
	}
	pub, err := db.GetPublication(ctx, v1.PublicationID)
	if err != nil || pub.SnapshotSHA256 != v1.SnapshotSHA256 || pub.State != "committed" {
		t.Fatalf("publication checksum: %+v %v", pub, err)
	}
	if _, err := db.GetVersion(ctx, d.ID, 2); !errors.Is(err, moxiedb.ErrNotFound) {
		t.Fatalf("tombstone visible: %v", err)
	}
	list, err := db.ListDatasets(ctx)
	if err != nil || len(list) != 1 || list[0].CurrentVersion != 1 {
		t.Fatalf("list newest live: %+v %v", list, err)
	}
	versions, err := db.ListVersions(ctx, d.ID)
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions: %+v %v", versions, err)
	}
	if err := db.ApplyDeletion(ctx, operation.ID, []int{1}); err != nil {
		t.Fatal(err)
	}
	list, err = db.ListDatasets(ctx)
	if err != nil || len(list) != 0 {
		t.Fatalf("deleted dataset listed: %+v %v", list, err)
	}
	meta, err := db.GetDataset(ctx, d.ID)
	if err != nil || meta.CurrentVersion != 2 {
		t.Fatalf("high water metadata: %+v %v", meta, err)
	}
	if next, err := db.NextVersion(ctx, d.ID); err != nil || next != 3 {
		t.Fatalf("next version: %d %v", next, err)
	}
	var count int
	if err := setup.QueryRowContext(ctx, `SELECT count(*) FROM dataset_version_documents`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("documents remain: %d %v", count, err)
	}
	if err := db.CompleteDeletion(ctx, operation.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.ApplyDeletion(ctx, operation.ID, nil); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("completed mutation: %v", err)
	}
	if err := db.CreateJob(ctx, j); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("deleted input accepted: %v", err)
	}
	if err := db.BeginIntent(ctx, i); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("deleted version intent: %v", err)
	}
	d.CurrentVersion = 2
	if err := db.CommitVersion(ctx, d, v2, docs); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("tombstone publication replay: %v", err)
	}
	// Existing preparation may allocate above all tombstones and revive metadata.
	d, v3, docs := fixture(t, db, 3, 2)
	if err := db.CommitVersion(ctx, d, v3, docs); err != nil {
		t.Fatalf("pending preparation revival: %v", err)
	}
	list, err = db.ListDatasets(ctx)
	if err != nil || len(list) != 1 || list[0].CurrentVersion != 3 {
		t.Fatalf("revived: %+v %v", list, err)
	}
	if _, err := db.GetCatalogExecution(ctx, "ns", "unselected", "r"); err != nil {
		t.Fatalf("live FK job lost: %v", err)
	}
}

func TestDeleteExecutionCatalog(t *testing.T) {
	db, _ := testDB(t)
	ctx := t.Context()
	d, v, docs := fixture(t, db, 1, 0)
	if err := db.CommitVersion(ctx, d, v, docs); err != nil {
		t.Fatal(err)
	}
	j := moxiedb.Job{JobID: "job", Mode: "training", TargetDatasetID: sql.NullString{String: d.ID, Valid: true}, BaseDatasetVersion: sql.NullInt64{Int64: 1, Valid: true}}
	if err := db.CreateJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	a := moxiedb.Attempt{AttemptID: "a1", JobID: j.JobID, AttemptNumber: 1, TemporalNamespace: "ns", WorkflowID: "workflow", RunID: sql.NullString{String: "old", Valid: true}}
	if err := db.CreateAttempt(ctx, a); err != nil {
		t.Fatal(err)
	}
	b := a
	b.AttemptID = "a2"
	b.AttemptNumber = 2
	b.PreviousAttemptID = sql.NullString{String: a.AttemptID, Valid: true}
	b.RunID.String = "new"
	if err := db.CreateAttempt(ctx, b); err != nil {
		t.Fatal(err)
	}
	other := a
	other.AttemptID = "a3"
	other.AttemptNumber = 3
	other.TemporalNamespace = "elsewhere"
	other.RunID.String = "other"
	if err := db.CreateAttempt(ctx, other); err != nil {
		t.Fatal(err)
	}
	all, err := db.DatasetExecutions(ctx, d.ID)
	if err != nil || len(all) != 3 || all[0].Attempt.RunID.String != "old" || all[2].Attempt.TemporalNamespace != "elsewhere" {
		t.Fatalf("all dataset attempts: %+v %v", all, err)
	}
	i := moxiedb.Intent{IntentID: "intent", JobID: j.JobID, AttemptID: a.AttemptID, Kind: "submit", Payload: []byte(`{}`), PayloadSHA256: strings.Repeat("a", 64)}
	if err := db.BeginIntent(ctx, i); err != nil {
		t.Fatal(err)
	}
	op := moxiedb.Deletion{ID: "delete-old", Namespace: "ns", WorkflowID: "workflow", RunID: "old", Manifest: []byte(`{"executions":[{"namespace":"ns","workflow_id":"workflow","run_id":"new"}]}`)}
	if err := db.DeleteExecutionCatalog(ctx, "ns", "workflow", "old"); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("unregistered deletion: %v", err)
	}
	if err := db.BeginDeletion(ctx, op); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("active attempts accepted for deletion: %v", err)
	}
	for _, id := range []string{a.AttemptID, b.AttemptID} {
		if err := db.UpdateAttemptObservation(ctx, id, 0, moxiedb.AttemptObservation{ExecutionStatus: sql.NullString{String: "completed", Valid: true}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SettleIntent(ctx, i.IntentID, "accepted", sql.NullString{}); err != nil {
		t.Fatal(err)
	}
	if err := db.BeginDeletion(ctx, op); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteExecutionCatalog(ctx, "ns", "workflow", "old"); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("lineage should fail closed: %v", err)
	}
	if _, err := db.GetIntent(ctx, i.IntentID); err != nil {
		t.Fatalf("failed deletion lost intent: %v", err)
	}
	if err := db.BeginIntent(ctx, i); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("deleted execution intent replay: %v", err)
	}
	if err := db.CreateAttempt(ctx, a); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("deleted execution attempt replay: %v", err)
	}
	preRun := a
	preRun.RunID = sql.NullString{}
	if err := db.CreateAttempt(ctx, preRun); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("pre-run replay bypassed deleted execution: %v", err)
	}
	if err := db.LinkAttemptRun(ctx, b.AttemptID, "new"); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("selected training resurrection: %v", err)
	}
	if err := db.DeleteExecutionCatalog(ctx, "ns", "workflow", "new"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteExecutionCatalog(ctx, "ns", "workflow", "old"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteExecutionCatalog(ctx, "ns", "workflow", "old"); err != nil {
		t.Fatalf("delete replay: %v", err)
	}
	if _, err := db.GetAttempt(ctx, other.AttemptID); err != nil {
		t.Fatalf("sibling removed: %v", err)
	}
	if _, err := db.GetIntent(ctx, i.IntentID); !errors.Is(err, moxiedb.ErrNotFound) {
		t.Fatalf("selected intent retained: %v", err)
	}
	if err := db.CreateAttempt(ctx, a); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("deleted execution recreated: %v", err)
	}
}

func TestConcurrentDeletions(t *testing.T) {
	db, _ := testDB(t)
	ctx := t.Context()
	start := make(chan struct{})
	results := make(chan error, 8)
	for n := range 8 {
		go func() {
			<-start
			results <- db.BeginDeletion(ctx, moxiedb.Deletion{ID: fmt.Sprintf("delete%d", n), Namespace: "ns", WorkflowID: "workflow", RunID: fmt.Sprintf("run%d", n), DatasetID: "data", Manifest: []byte(`{}`)})
		}()
	}
	close(start)
	successes := 0
	for range 8 {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, moxiedb.ErrConflict):
		default:
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("pending winners: %d", successes)
	}
	pending, err := db.PendingDeletions(ctx)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending: %+v %v", pending, err)
	}
}

func TestSchemaMigrationSequence(t *testing.T) {
	db, setup := testDB(t)
	ctx := t.Context()
	// Runtime checks never install a missing migration.
	if _, err := setup.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version=2`); err != nil {
		t.Fatal(err)
	}
	if err := db.CheckSchema(ctx); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("001-only accepted: %v", err)
	}
	var count int
	if err := setup.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("runtime mutated migrations: %d %v", count, err)
	}
	if _, err := setup.ExecContext(ctx, `INSERT INTO schema_migrations(version,checksum) VALUES (2,$1)`, fmt.Sprintf("%x", sha256.Sum256(deletionMigration))); err != nil {
		t.Fatal(err)
	}
	if _, err := setup.ExecContext(ctx, `INSERT INTO schema_migrations(version,checksum) VALUES (3,repeat('a',64))`); err != nil {
		t.Fatal(err)
	}
	if err := db.CheckSchema(ctx); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("future accepted: %v", err)
	}
	if _, err := setup.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version=3; ALTER TABLE datasets DROP COLUMN deleted_at`); err != nil {
		t.Fatal(err)
	}
	if err := db.CheckSchema(ctx); !errors.Is(err, moxiedb.ErrConflict) {
		t.Fatalf("missing deletion marker accepted: %v", err)
	}
}
