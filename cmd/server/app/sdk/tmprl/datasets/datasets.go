// Package datasets provides dataset management and preparation-run operations to callers.
// Jobs own execution; store owns immutable files. This package owns their application-facing composition.
package datasets

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets/store"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/moxiedb"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/uploads"
)

// Service connects the host's dataset store and optional application job.
// Dataset management remains available when execution is disabled.
type Service struct {
	files *store.DatasetStore
	job   *training.Job
	db    *moxiedb.DB
	// Namespace is supplied by the host before serving requests. It defaults to
	// the job's namespace, or default when execution is disabled.
	Namespace string
	// Uploads, when supplied by the host, names each run's source document.
	Uploads *uploads.Store
	names   sync.Map // Immutable dataset and upload IDs to display names.
}

// New borrows an initialized store and optional job; it creates no storage or workers.
func New(files *store.DatasetStore, job *training.Job, database ...*moxiedb.DB) *Service {
	service := &Service{files: files, job: job, Namespace: "default"}
	if job != nil {
		service.Namespace = job.Namespace()
	}
	if len(database) > 0 {
		service.db = database[0]
	}
	return service
}

// RunsAvailable reports whether the host supplied an execution service.
func (s *Service) RunsAvailable() bool { return s.job != nil }

// State adds dataset-output identity and readiness to a job observation.
// It is not a persisted snapshot or an HTTP response schema.
type State struct {
	training.State
	JobID                  string
	AttemptID              string
	ObservationRevision    int64
	ObservedAt             time.Time
	PendingActions         []PendingAction
	DatasetAvailable       bool
	DatasetRecordCount     int
	SkippedRecordCount     int // Records not saved because the dataset already had their question.
	TrainingReady          bool
	PreparedDatasetID      string
	PreparedDatasetVersion int
	// Run identity known without the live request: the dataset it adds to,
	// the version it started from, and the uploaded source document.
	TargetDatasetID   string
	TargetDatasetName string
	BaseVersion       int
	SourceUploadID    string
	SourceDocument    string
}

func (s *Service) describe(ctx context.Context, run training.State) State {
	state := State{State: run}
	if prepare := run.Request.Prepare; prepare != nil {
		state.TargetDatasetID, state.BaseVersion, state.SourceUploadID = prepare.TargetDatasetID, prepare.BaseVersion, prepare.DatasetID
	}
	s.label(ctx, &state)
	if prepared := run.Artifacts.Preparation; prepared != nil {
		state.PreparedDatasetID, state.PreparedDatasetVersion = prepared.SavedDatasetID, prepared.SavedVersion
		state.DatasetAvailable = run.Request.Prepare != nil && prepared.DatasetID == run.Request.Prepare.DatasetID && prepared.RecordsPath != ""
		state.DatasetRecordCount = prepared.RecordCount
		state.TrainingReady = state.DatasetAvailable && prepared.TrainingReady
		if prepared.SavedDatasetID != "" {
			state.DatasetRecordCount, state.SkippedRecordCount = prepared.SavedRecordCount, prepared.SavedSkippedRecords
			state.TrainingReady = state.DatasetAvailable && prepared.SavedTrainingReady
		}
	}
	return state
}

// StartRun submits dataset work and returns its initial application state.
// Legacy combined and imported-JSONL submissions retain the job's validation and plan.
func (s *Service) StartRun(ctx context.Context, input training.Request) (State, error) {
	if !s.RunsAvailable() {
		return State{}, training.ErrUnavailable
	}
	run, err := s.job.Start(ctx, input)
	if err != nil {
		return State{}, err
	}
	return s.describe(ctx, training.Describe(run)), nil
}

// GetRun reports an exact execution with the saved dataset version's readiness.
func (s *Service) GetRun(ctx context.Context, e tmprl.Execution) (State, error) {
	if s.db != nil {
		return s.catalogRun(ctx, e)
	}
	if !s.RunsAvailable() {
		return State{}, training.ErrUnavailable
	}
	run, err := s.job.Get(ctx, e)
	if err != nil {
		return State{}, err
	}
	return s.describe(ctx, run), nil
}

// ListRuns preserves the job's ordering, filters and opaque pagination token.
// Shared history includes legacy training runs without guessing execution type in HTTP.
func (s *Service) ListRuns(ctx context.Context, options tmprl.ListOptions) ([]State, []byte, error) {
	return s.ListCatalogRuns(ctx, RunFilter{PageSize: options.PageSize, NextPageToken: options.NextPageToken, Statuses: options.Statuses})
}

// ContinuePreview sends reviewed preparation input through the owning job.
// It never grants the separate authorization to start training.
func (s *Service) ContinuePreview(ctx context.Context, e tmprl.Execution, decision tmprl.Decision) (tmprl.Decision, error) {
	if !s.RunsAvailable() {
		return tmprl.Decision{}, training.ErrUnavailable
	}
	return s.job.ContinuePreview(ctx, e, decision)
}

// StopRun acknowledges cooperative cancellation, not terminal state or output deletion.
func (s *Service) StopRun(ctx context.Context, e tmprl.Execution) error {
	if !s.RunsAvailable() {
		return training.ErrUnavailable
	}
	return s.job.Stop(ctx, e)
}

// OpenPreparedDataset reads the exact prepared output selected by the job.
// The caller closes the stream; supplied artifact paths never select a file.
func (s *Service) OpenPreparedDataset(ctx context.Context, e tmprl.Execution) (io.ReadCloser, error) {
	if !s.RunsAvailable() {
		return nil, training.ErrUnavailable
	}
	return s.job.OpenDataset(ctx, e)
}
