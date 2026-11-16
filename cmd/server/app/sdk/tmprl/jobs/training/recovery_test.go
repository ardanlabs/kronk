package training_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets"
	job "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training/stages/preparation"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/moxiedb"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
)

// Milestone 3B.3: real process crashes. The worker runs in a separate process
// (this test binary re-executed) and is killed with SIGKILL at exact points, so
// no deferred cleanup, drain, or orderly worker shutdown runs. A new process
// then recovers from the durable stores: Temporal, PostgreSQL, and the run
// workspace on disk. Model and extraction calls are deterministic stand-ins;
// these tests prove recovery, not model quality.

const workerEnv = "MOXIE_RECOVERY_WORKER"

// TestRecoveryWorkerProcess is the child process entry point. It skips unless
// the parent selected worker mode through the environment.
func TestRecoveryWorkerProcess(t *testing.T) {
	var cfg workerConfig
	if err := json.Unmarshal([]byte(os.Getenv(workerEnv)), &cfg); err != nil || cfg.TaskQueue == "" {
		t.Skip("child worker process only")
	}
	ctx := context.Background()
	c, err := client.DialContext(ctx, client.Options{HostPort: cfg.Address, Namespace: cfg.Namespace})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	db, err := moxiedb.Open(ctx, cfg.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	files, err := uploads.New(uploads.Config{Root: cfg.Uploads})
	if err != nil {
		t.Fatal(err)
	}
	gate := gates(cfg.Gates)
	j, err := job.New(c, job.Config{
		TaskQueue: cfg.TaskQueue, Namespace: cfg.Namespace, Uploads: files, MoxieDB: db, StorageRoot: cfg.Storage, Trainer: &trainer{},
		Extractor: gatedExtractor{gate},
		Completer: preparationCompleter(func(ctx context.Context, request providers.CompletionRequest) (providers.Completion, error) {
			prompt := request.Messages[1].Content
			switch {
			case strings.Contains(prompt, "[RECORD 1]"):
				gate.reach(ctx, "verify")
			case strings.Contains(prompt, "Locked target:"):
			default:
				if strings.Contains(prompt, "[EVIDENCE 1]") {
					gate.reach(ctx, "generate")
				}
			}
			return (completer{}).Complete(ctx, request)
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	w := j.Jobs().NewWorker()
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.Gates, "ready"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	select {} // Runs until the parent kills this process.
}

type workerConfig struct {
	Address, Namespace, TaskQueue, Database, Uploads, Storage, Gates string
}

// gates lets the parent hold the worker at a named point and count how many
// times it was reached. Each arrival appends to "<name>.count". "<name>.hold"
// contains N: arrival N and later block there until the hold is removed, so
// the parent can stop at an exact point and kill the process while held.
type gates string

func (g gates) reach(ctx context.Context, name string) {
	f, err := os.OpenFile(filepath.Join(string(g), name+".count"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.WriteString(".")
	info, _ := f.Stat()
	_ = f.Close()
	arrival := int(info.Size())
	for {
		data, err := os.ReadFile(filepath.Join(string(g), name+".hold"))
		if err != nil {
			return
		}
		var from int
		if _, err := fmt.Sscan(string(data), &from); err != nil || arrival < from {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// gatedExtractor stands in for Docling on a PDF upload. Conversion converts the
// uploaded Markdown bytes, after an optional hold to model a slow conversion.
type gatedExtractor struct{ gates }

func (e gatedExtractor) ExtractionProfile() string { return "recovery-test-v1" }

func (e gatedExtractor) Convert(ctx context.Context, path string, _ providers.DocumentOptions) (providers.Document, error) {
	e.reach(ctx, "extract")
	data, err := os.ReadFile(path)
	if err != nil {
		return providers.Document{}, err
	}
	structured, err := json.Marshal(map[string]any{"texts": []map[string]string{{"text": string(data)}}})
	return providers.Document{Markdown: string(data), StructuredJSON: structured}, err
}

// recoveryHarness owns the durable stores shared by every worker process and
// the parent's client-side application objects.
type recoveryHarness struct {
	t       *testing.T
	env     *Environment
	cfg     workerConfig
	db      *moxiedb.DB
	setup   *sql.DB // Same isolated schema, for test-only fault injection.
	service *datasets.Service
	j       *job.Job
	upload  uploads.File
	worker  *exec.Cmd
}

func newRecoveryHarness(t *testing.T) *recoveryHarness {
	t.Helper()
	connection := os.Getenv("MOXIEDB_TEST_URL")
	address := os.Getenv("TEMPORAL_ADDRESS")
	if connection == "" || address == "" {
		t.Skip("set MOXIEDB_TEST_URL and TEMPORAL_ADDRESS to isolated test services")
	}
	db, setup := newCatalogDatabase(t)
	var schema string
	if err := setup.QueryRowContext(t.Context(), "SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatal(err)
	}
	env := newTestEnvironment(t)
	root := t.TempDir()
	h := &recoveryHarness{t: t, env: env, db: db, setup: setup, cfg: workerConfig{
		Address: address, Namespace: env.Namespace, TaskQueue: env.TaskQueue,
		Database: withSearchPath(t, connection, schema),
		Uploads:  filepath.Join(root, "uploads"), Storage: filepath.Join(root, "moxie"), Gates: filepath.Join(root, "gates"),
	}}
	if err := os.MkdirAll(h.cfg.Gates, 0o700); err != nil {
		t.Fatal(err)
	}
	files, err := uploads.New(uploads.Config{Root: h.cfg.Uploads})
	if err != nil {
		t.Fatal(err)
	}
	_, text := source(t, true)
	// A PDF routes extraction through the (gated) extractor instead of the
	// in-process Markdown path, so extraction itself can be interrupted.
	if h.upload, err = files.Put(t.Context(), uploads.Input{Name: "manual.pdf"}, strings.NewReader(text)); err != nil {
		t.Fatal(err)
	}
	// The parent is a client only: it never registers a worker, so every step
	// runs in a child process that can be killed.
	// Its completer is required only to validate submissions.
	if h.j, err = job.New(env.Client, job.Config{TaskQueue: env.TaskQueue, Namespace: env.Namespace, Uploads: files, MoxieDB: db, StorageRoot: h.cfg.Storage, Trainer: &trainer{}, Completer: completer{}, Log: env.Log}); err != nil {
		t.Fatal(err)
	}
	h.service = datasets.New(datasets.Config{Files: h.j.Datasets(), Job: h.j, DB: db})
	if err := h.service.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.kill)
	return h
}

// awaitStatus polls the application read path (Temporal plus SQL catalog).
func awaitStatus(t *testing.T, ctx context.Context, j *job.Job, e tmprl.Execution, done func(job.State) bool) job.State {
	t.Helper()
	for {
		state, err := j.Get(ctx, e)
		if err == nil && done(state) {
			return state
		}
		select {
		case <-ctx.Done():
			t.Fatalf("waiting for status: %+v %v: %v", state.Status, err, ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func pendingDataset(t *testing.T, ctx context.Context, db *moxiedb.DB) string {
	t.Helper()
	pending, err := db.PendingPublications(ctx)
	if err != nil || len(pending) != 1 {
		t.Fatalf("expected one pending publication: %+v %v", pending, err)
	}
	return pending[0].DatasetID
}

func withSearchPath(t *testing.T, connection, schema string) string {
	t.Helper()
	separator := "?"
	if strings.Contains(connection, "?") {
		separator = "&"
	}
	return connection + separator + "search_path=" + schema
}

// start launches a fresh worker process and waits until it polls.
func (h *recoveryHarness) start(ctx context.Context) {
	h.t.Helper()
	_ = os.Remove(filepath.Join(h.cfg.Gates, "ready"))
	config, err := json.Marshal(h.cfg)
	if err != nil {
		h.t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRecoveryWorkerProcess$", "-test.count=1", "-test.timeout=0")
	cmd.Env = append(os.Environ(), workerEnv+"="+string(config))
	if testing.Verbose() {
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	}
	if err := cmd.Start(); err != nil {
		h.t.Fatal(err)
	}
	h.worker = cmd
	h.waitFor(ctx, "worker ready", func() bool {
		_, err := os.Stat(filepath.Join(h.cfg.Gates, "ready"))
		return err == nil
	})
}

// kill sends SIGKILL: no defers, drain, or worker shutdown run.
func (h *recoveryHarness) kill() {
	if h.worker == nil || h.worker.Process == nil {
		return
	}
	_ = h.worker.Process.Signal(syscall.SIGKILL)
	_ = h.worker.Wait()
	h.worker = nil
}

// hold blocks arrival number from (counting from 1 across processes) and every
// later arrival at name; from 0 releases the gate.
func (h *recoveryHarness) hold(name string, from int) {
	h.t.Helper()
	path := filepath.Join(h.cfg.Gates, name+".hold")
	if from == 0 {
		_ = os.Remove(path)
		return
	}
	if err := os.WriteFile(path, []byte(fmt.Sprint(from)), 0o600); err != nil {
		h.t.Fatal(err)
	}
}

func (h *recoveryHarness) count(name string) int {
	data, _ := os.ReadFile(filepath.Join(h.cfg.Gates, name+".count"))
	return len(data)
}

func (h *recoveryHarness) waitFor(ctx context.Context, what string, done func() bool) {
	h.t.Helper()
	for !done() {
		select {
		case <-ctx.Done():
			h.t.Fatalf("waiting for %s: %v", what, ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// replayClean fails when any workflow task failed. Temporal retries a
// nondeterministic replay until it happens to match, so a run can still close
// normally while recovery was broken.
func (h *recoveryHarness) replayClean(ctx context.Context, e tmprl.Execution) {
	h.t.Helper()
	history := h.env.Client.GetWorkflowHistory(ctx, e.WorkflowID, e.RunID, false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
	for history.HasNext() {
		event, err := history.Next()
		if err != nil {
			h.t.Fatal(err)
		}
		if event.GetEventType() == enumspb.EVENT_TYPE_WORKFLOW_TASK_FAILED {
			attributes := event.GetWorkflowTaskFailedEventAttributes()
			h.t.Fatalf("workflow task failed during recovery (%s): %s", attributes.GetCause(), attributes.GetFailure().GetMessage())
		}
	}
}

func (h *recoveryHarness) workspace(e tmprl.Execution, file string) string {
	return filepath.Join(h.cfg.Storage, "training-jobs", e.RunID, "steps", "preparation", file)
}

// checkpoint reads the saved preparation cursor from disk.
func (h *recoveryHarness) checkpoint(e tmprl.Execution, file string) (records, next, verified int) {
	h.t.Helper()
	data, err := os.ReadFile(h.workspace(e, file))
	if err != nil {
		return 0, 0, 0
	}
	var state struct {
		Records  []json.RawMessage `json:"records"`
		Next     int               `json:"next"`
		Verified int               `json:"verified"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		h.t.Fatal(err)
	}
	return len(state.Records), state.Next, state.Verified
}

func (h *recoveryHarness) submit(ctx context.Context, id string, max int) tmprl.Execution {
	h.t.Helper()
	destination, err := h.service.Create(ctx, "Recovery "+id)
	if err != nil {
		h.t.Fatal(err)
	}
	run, err := h.service.StartRun(ctx, job.Request{ID: id + "-" + h.env.TaskQueue, PreparationOnly: true, Prepare: &preparation.PreparationRequest{
		DatasetID: h.upload.ID, Model: "generator", Goal: preparation.GoalQuestionAnswer, MaxQuestions: max,
		TargetDatasetID: destination.ID, BaseVersion: destination.Version,
	}})
	if err != nil {
		h.t.Fatal(err)
	}
	return run.Execution
}

func (h *recoveryHarness) review(ctx context.Context, e tmprl.Execution) string {
	h.t.Helper()
	waiting := waitForCheckpoint(h.t, ctx, h.j, e, "preview-review")
	payload, err := json.Marshal(preparation.Review{Records: waiting.Artifacts.Preview.Preview})
	if err != nil {
		h.t.Fatal(err)
	}
	if _, err := h.service.ContinuePreview(ctx, e, tmprl.Decision{CheckpointID: waiting.Checkpoint.ID, SubmissionID: "review", Payload: payload}); err != nil {
		h.t.Fatal(err)
	}
	return waiting.Checkpoint.ID
}

// finish waits for completion and checks the published dataset: exactly one
// version added, the expected record count, unique facts, accepted review kept,
// and no training.
func (h *recoveryHarness) finish(ctx context.Context, e tmprl.Execution, records int) tmprl.Run[job.Request, job.Artifacts] {
	h.t.Helper()
	completed, err := h.j.Jobs().Wait(ctx, e)
	prepared := completed.Artifacts.Preparation
	if err != nil || completed.Status != tmprl.StatusCompleted || prepared == nil || prepared.RecordCount != records || prepared.SavedVersion != 2 || len(completed.Stages) != 1 {
		h.t.Fatalf("recovered run did not complete exactly once: status %s, prepared %+v, stages %d, %v", completed.Status, prepared, len(completed.Stages), err)
	}
	h.replayClean(ctx, e)
	if len(completed.Decisions) != 1 || completed.Decisions[0].SubmissionID != "review" {
		h.t.Fatalf("accepted review repeated or lost: %+v", completed.Decisions)
	}
	versions, err := h.db.ListVersions(ctx, prepared.SavedDatasetID)
	if err != nil || len(versions) != 2 {
		h.t.Fatalf("publication duplicated or missing: %+v %v", versions, err)
	}
	file, err := h.service.OpenPreparedDataset(ctx, e)
	if err != nil {
		h.t.Fatal(err)
	}
	defer file.Close()
	// Evaluation questions deliberately share a fact with its training example;
	// a replayed batch would repeat the same prompt.
	seen := make(map[string]bool)
	var rows int
	decoder := json.NewDecoder(file)
	for decoder.More() {
		var r struct {
			Prompt string `json:"prompt"`
		}
		if err := decoder.Decode(&r); err != nil {
			h.t.Fatal(err)
		}
		if seen[r.Prompt] {
			h.t.Fatalf("duplicate example after recovery: %q", r.Prompt)
		}
		seen[r.Prompt] = true
		rows++
	}
	if rows != records {
		h.t.Fatalf("published %d records, want %d", rows, records)
	}
	return completed
}

// unlimitedRecords is the uninterrupted output for the 20-device source with no
// example limit: 20 generated examples plus 30 evaluation questions.
const unlimitedRecords = 50

// TestRecoveryCrashMatrix kills the worker process at each interruption point
// and requires a new process to continue from durable progress.
func TestRecoveryCrashMatrix(t *testing.T) {
	if os.Getenv(workerEnv) != "" {
		t.Skip("parent test only")
	}

	t.Run("extraction in flight", func(t *testing.T) {
		h := newRecoveryHarness(t)
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
		defer cancel()
		h.hold("extract", 1)
		h.start(ctx)
		e := h.submit(ctx, "extract", 12)
		h.waitFor(ctx, "extraction started", func() bool { return h.count("extract") == 1 })
		h.kill()
		if _, err := os.Stat(h.workspace(e, "extraction.json")); err == nil {
			t.Fatal("interrupted conversion was saved as complete")
		}
		// Approved policy: an interrupted conversion runs again, once.
		h.hold("extract", 0)
		h.start(ctx)
		h.review(ctx, e)
		h.finish(ctx, e, 12)
		if got := h.count("extract"); got != 2 {
			t.Fatalf("extraction ran %d times, want the interrupted run plus one retry", got)
		}
	})

	t.Run("waiting for preview review", func(t *testing.T) {
		h := newRecoveryHarness(t)
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
		defer cancel()
		h.start(ctx)
		e := h.submit(ctx, "review", 12)
		before := waitForCheckpoint(t, ctx, h.j, e, "preview-review")
		h.kill()
		h.start(ctx)
		after := waitForCheckpoint(t, ctx, h.j, e, "preview-review")
		if after.Checkpoint.ID != before.Checkpoint.ID || len(after.Decisions) != 0 || after.Artifacts.Preview == nil ||
			fmt.Sprint(after.Artifacts.Preview.Preview) != fmt.Sprint(before.Artifacts.Preview.Preview) {
			t.Fatalf("restart changed or approved the preview: before %+v after %+v", before.Checkpoint, after)
		}
		if h.count("extract") != 1 {
			t.Fatalf("restart repeated completed extraction: %d", h.count("extract"))
		}
		h.review(ctx, e)
		h.finish(ctx, e, 12)
	})

	t.Run("generation after two batches with one in flight", func(t *testing.T) {
		h := newRecoveryHarness(t)
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
		defer cancel()
		// Generation requests: 1 for Preview, then one per 5-passage batch.
		// Hold request 4, the third full batch, after two batches are saved.
		h.hold("generate", 4)
		h.start(ctx)
		e := h.submit(ctx, "generate", 0)
		h.review(ctx, e)
		h.waitFor(ctx, "third batch in flight", func() bool { return h.count("generate") == 4 })
		calls := h.count("generate")
		savedRecords, savedNext, _ := h.checkpoint(e, "steps.json")
		if savedNext != 10 {
			t.Fatalf("held at the wrong point: %d passages saved, want 10", savedNext)
		}
		h.kill()
		if records, next, _ := h.checkpoint(e, "steps.json"); records != savedRecords || next != savedNext || next < 10 {
			t.Fatalf("crash lost saved batches: %d records at %d, had %d at %d", records, next, savedRecords, savedNext)
		}
		h.hold("generate", 0)
		h.start(ctx)
		h.finish(ctx, e, unlimitedRecords)
		// Only the in-flight batch repeats: 1 Preview + 4 full batches + 1 repeat.
		if got := h.count("generate"); got != calls+(20-savedNext)/5 {
			t.Fatalf("restart repeated saved generation: %d requests, want %d", got, calls+(20-savedNext)/5)
		}
	})

	t.Run("verification in flight", func(t *testing.T) {
		h := newRecoveryHarness(t)
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
		defer cancel()
		// Verification requests: 1 for Preview, then full verification batches.
		h.hold("verify", 2)
		h.start(ctx)
		e := h.submit(ctx, "verify", 12)
		h.review(ctx, e)
		h.waitFor(ctx, "full verification in flight", func() bool { return h.count("verify") == 2 })
		h.kill()
		if _, err := os.Stat(h.workspace(e, "result.json")); err == nil {
			t.Fatal("unverified output was committed")
		}
		if datasets, err := h.service.List(ctx); err != nil || len(datasets) != 1 || datasets[0].Version != 1 {
			t.Fatalf("unverified output was published: %+v %v", datasets, err)
		}
		h.hold("verify", 0)
		h.start(ctx)
		h.finish(ctx, e, 12)
	})

	t.Run("dataset file written before catalog commit", func(t *testing.T) {
		h := newRecoveryHarness(t)
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
		defer cancel()
		// Block the catalog insert after the snapshot file is written: the
		// parent holds an advisory lock that the insert trigger waits for.
		lock, err := h.setup.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Close()
		if _, err := lock.ExecContext(ctx, "SELECT pg_advisory_lock(3303)"); err != nil {
			t.Fatal(err)
		}
		if _, err := h.setup.ExecContext(ctx, `CREATE FUNCTION hold_publication() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN PERFORM pg_advisory_xact_lock(3303); RETURN NEW; END $$;
CREATE TRIGGER hold_publication BEFORE INSERT ON dataset_versions FOR EACH ROW WHEN (NEW.version = 2) EXECUTE FUNCTION hold_publication();`); err != nil {
			t.Fatal(err)
		}
		h.start(ctx)
		e := h.submit(ctx, "publish", 12)
		h.review(ctx, e)
		h.waitFor(ctx, "snapshot written, catalog insert blocked", func() bool {
			pending, err := h.db.PendingPublications(ctx)
			_, statErr := os.Stat(h.workspace(e, "result.json"))
			return err == nil && len(pending) == 1 && statErr == nil
		})
		h.kill()
		if versions, err := h.db.ListVersions(ctx, pendingDataset(t, ctx, h.db)); err != nil || len(versions) != 1 {
			t.Fatalf("crashed publication reached the catalog: %+v %v", versions, err)
		}
		if _, err := lock.ExecContext(ctx, "SELECT pg_advisory_unlock(3303)"); err != nil {
			t.Fatal(err)
		}
		if _, err := h.setup.ExecContext(ctx, "DROP TRIGGER hold_publication ON dataset_versions"); err != nil {
			t.Fatal(err)
		}
		// The killed worker's catalog transaction rolled back; its snapshot
		// file and pending publication remain. The retried step reconciles the
		// same publication once.
		h.start(ctx)
		h.finish(ctx, e, 12)
		if pending, err := h.db.PendingPublications(ctx); err != nil || len(pending) != 0 {
			t.Fatalf("publication left pending: %+v %v", pending, err)
		}
	})

	t.Run("stop just before crash", func(t *testing.T) {
		h := newRecoveryHarness(t)
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
		defer cancel()
		h.hold("generate", 3)
		h.start(ctx)
		e := h.submit(ctx, "stop", 0)
		h.review(ctx, e)
		h.waitFor(ctx, "batch in flight", func() bool { return h.count("generate") == 3 })
		saved, next, _ := h.checkpoint(e, "steps.json")
		if err := h.service.StopRun(ctx, e); err != nil {
			t.Fatal(err)
		}
		h.kill()
		h.hold("generate", 0)
		h.start(ctx)
		stopped := awaitStatus(t, ctx, h.j, e, func(s job.State) bool { return s.Status.Terminal() })
		if stopped.Status != tmprl.StatusStopped {
			t.Fatalf("Stop before crash was not honored: %s", stopped.Status)
		}
		h.replayClean(ctx, e)
		time.Sleep(time.Second) // A restarted worker must not resume stopped work.
		if got := h.count("generate"); got != 3 {
			t.Fatalf("stopped run kept generating after restart: %d requests", got)
		}
		if records, n, _ := h.checkpoint(e, "steps.json"); records != saved || n != next {
			t.Fatalf("Stop lost saved progress: %d records at %d, had %d at %d", records, n, saved, next)
		}
	})

	t.Run("repeated crashes", func(t *testing.T) {
		h := newRecoveryHarness(t)
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
		defer cancel()
		// Crash during two consecutive generation batches. Every crash consumes
		// one of the plan's three worker-recovery attempts; the second recovery
		// must keep everything the first one saved.
		h.hold("generate", 3)
		h.start(ctx)
		e := h.submit(ctx, "repeat", 0)
		h.review(ctx, e)
		progress := 0
		for crash, request := range []int{3, 5} {
			h.waitFor(ctx, "batch in flight", func() bool { return h.count("generate") == request })
			h.kill()
			records, next, _ := h.checkpoint(e, "steps.json")
			if next <= progress || records < next {
				t.Fatalf("crash %d moved saved progress backwards: next %d (was %d), %d records", crash+1, next, progress, records)
			}
			progress = next
			// The restarted worker repeats the held batch (one more arrival),
			// saves it, then holds on the following batch.
			h.hold("generate", request+2)
			h.start(ctx)
		}
		h.hold("generate", 0)
		completed := h.finish(ctx, e, unlimitedRecords)
		if completed.SessionAttempt != 3 {
			t.Fatalf("each crash should consume one recovery attempt: %d", completed.SessionAttempt)
		}
	})
}
