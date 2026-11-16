package datasets

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/datasets/store"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/training"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/moxiedb"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
)

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

// PendingAction reports requested work separately from confirmed execution.
// Command payloads, source text and internal errors never cross this boundary.
type PendingAction struct {
	ID    string
	Kind  string
	State string
}

// RunFilter selects catalog rows before pagination. Tokens belong to the exact
// namespace, filters and page size, not to Temporal's independent visibility API.
type RunFilter struct {
	PageSize        int
	NextPageToken   []byte
	Mode            string
	TargetDatasetID string
	Statuses        []tmprl.Status
	Oldest          bool // Oldest lists runs oldest first; it requires the catalog database.
}

type catalogToken struct {
	Offset int    `json:"offset"`
	Filter string `json:"filter"`
}

// RunsAvailable reports whether the host supplied an execution service.
func (s *Service) RunsAvailable() bool { return s.job != nil }

// CatalogAvailable is independent of the worker/command service's availability.
func (s *Service) CatalogAvailable() bool { return s.db != nil || s.RunsAvailable() }

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

// ListRuns reads persisted observations without contacting a worker or
// Temporal. The file-only compatibility path retains runner visibility tokens.
func (s *Service) ListRuns(ctx context.Context, options RunFilter) ([]State, []byte, error) {
	if err := options.validate(); err != nil {
		return nil, nil, err
	}
	if s.db == nil {
		if !s.RunsAvailable() {
			return nil, nil, training.ErrUnavailable
		}
		if options.Oldest {
			return nil, nil, fmt.Errorf("%w: oldest-first order requires the catalog database", tmprl.ErrInvalidArgument)
		}
		runs, next, err := s.job.List(ctx, tmprl.ListOptions{PageSize: options.PageSize, NextPageToken: options.NextPageToken, Statuses: options.Statuses})
		if err != nil {
			return nil, nil, err
		}
		states := make([]State, 0, len(runs))
		for _, run := range runs {
			mode, target := "training", ""
			if run.Request.Prepare != nil {
				mode, target = "preparation_and_training", run.Request.Prepare.TargetDatasetID
				if run.Request.PreparationOnly {
					mode = "preparation"
				}
			}
			if (options.Mode == "" || options.Mode == mode) && (options.TargetDatasetID == "" || options.TargetDatasetID == target) {
				states = append(states, s.describe(ctx, run))
			}
		}
		return states, next, nil
	}
	statuses := make([]string, len(options.Statuses))
	for i, status := range options.Statuses {
		statuses[i] = string(status)
	}
	slices.Sort(statuses)
	filter := moxiedb.CatalogFilter{Namespace: s.namespace, Mode: options.Mode, TargetDatasetID: options.TargetDatasetID, Statuses: statuses, Limit: options.PageSize, Oldest: options.Oldest}
	encoded, _ := json.Marshal(filter)
	digest := fmt.Sprintf("%x", sha256.Sum256(encoded))
	if len(options.NextPageToken) != 0 {
		var token catalogToken
		if err := json.Unmarshal(options.NextPageToken, &token); err != nil || token.Offset < 0 || token.Filter != digest {
			return nil, nil, fmt.Errorf("%w: pagination token does not match this catalog query", tmprl.ErrInvalidArgument)
		}
		filter.Offset = token.Offset
	}
	// One lookahead row is sufficient; filtering occurs in SQL, before LIMIT.
	filter.Limit++
	entries, err := s.db.ListCatalog(ctx, filter)
	if err != nil {
		return nil, nil, err
	}
	var next []byte
	if len(entries) > options.PageSize {
		entries = entries[:options.PageSize]
		next, _ = json.Marshal(catalogToken{Offset: filter.Offset + options.PageSize, Filter: digest})
	}
	states := make([]State, 0, len(entries))
	for _, entry := range entries {
		state, err := s.fromCatalog(ctx, entry)
		if err != nil {
			return nil, nil, err
		}
		states = append(states, state)
	}
	return states, next, nil
}

func (f RunFilter) validate() error {
	if f.PageSize < 1 || f.PageSize > 100 {
		return fmt.Errorf("%w: page_size must be between 1 and 100", tmprl.ErrInvalidArgument)
	}
	switch f.Mode {
	case "", "preparation", "training", "preparation_and_training":
	default:
		return fmt.Errorf("%w: unsupported mode", tmprl.ErrInvalidArgument)
	}
	if f.TargetDatasetID != "" && !store.ValidID(f.TargetDatasetID) {
		return fmt.Errorf("%w: invalid target dataset ID", tmprl.ErrInvalidArgument)
	}
	for _, status := range f.Statuses {
		switch status {
		case tmprl.StatusQueued, tmprl.StatusRunning, tmprl.StatusRecovering, tmprl.StatusPauseRequested, tmprl.StatusPaused, tmprl.StatusWaiting,
			tmprl.StatusCompleted, tmprl.StatusStopped, tmprl.StatusFailed, "needs_improvement":
		default:
			return fmt.Errorf("%w: unsupported status", tmprl.ErrInvalidArgument)
		}
	}
	return nil
}

// ContinuePreview sends reviewed preparation input through the owning job.
// It never grants the separate authorization to start training.
func (s *Service) ContinuePreview(ctx context.Context, e tmprl.Execution, decision tmprl.Decision) (tmprl.Decision, error) {
	if !s.RunsAvailable() {
		return tmprl.Decision{}, training.ErrUnavailable
	}
	return s.job.ContinuePreview(ctx, e, decision)
}

// PreparationControl delegates durable controls; it never executes preparation
// or treats a command receipt as a saved computation checkpoint.
func (s *Service) PreparationControl(ctx context.Context, e tmprl.Execution, request tmprl.ControlRequest) (tmprl.ControlReceipt, error) {
	if !s.RunsAvailable() {
		return tmprl.ControlReceipt{}, training.ErrUnavailable
	}
	return s.job.PreparationControl(ctx, e, request)
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

func (s *Service) catalogRun(ctx context.Context, e tmprl.Execution) (State, error) {
	if err := e.Validate(); err != nil {
		return State{}, err
	}
	var live State
	var liveErr error
	if s.job != nil {
		run, err := s.job.Get(ctx, e)
		liveErr = err
		if err == nil {
			live = s.describe(ctx, run)
		}
	}
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	entry, err := s.db.GetCatalogExecution(ctx, s.namespace, e.WorkflowID, e.RunID)
	if errors.Is(err, moxiedb.ErrNotFound) {
		return State{}, errors.Join(training.ErrNotFound, liveErr)
	}
	if err != nil {
		return State{}, err
	}
	state, err := s.fromCatalog(ctx, entry)
	if err != nil {
		return State{}, err
	}
	if s.job != nil && liveErr == nil {
		live.JobID, live.AttemptID, live.ObservationRevision = state.JobID, state.AttemptID, state.ObservationRevision
		live.ObservedAt, live.PendingActions = state.ObservedAt, state.PendingActions
		return live, nil
	}
	state.StateStale, state.RemainingSeconds = true, 0
	return state, nil
}

func (s *Service) fromCatalog(ctx context.Context, entry moxiedb.CatalogEntry) (State, error) {
	a := entry.Attempt
	var observation training.CatalogObservation
	if len(a.Observation) != 0 {
		if err := json.Unmarshal(a.Observation, &observation); err != nil {
			return State{}, fmt.Errorf("reading job observation: %w", err)
		}
	}
	state := State{State: observation.State, JobID: entry.Job.JobID, AttemptID: a.AttemptID,
		ObservationRevision: a.ObservationRevision, ObservedAt: a.ObservedAt.Time,
		DatasetAvailable: observation.Prepared, DatasetRecordCount: observation.RecordCount, SkippedRecordCount: observation.SkippedRecords, TrainingReady: observation.TrainingReady}
	state.WorkflowID, state.RunID = a.WorkflowID, a.RunID.String
	job := entry.Job
	state.TargetDatasetID, state.BaseVersion, state.SourceUploadID = job.TargetDatasetID.String, int(job.BaseDatasetVersion.Int64), job.SourceUploadID.String
	s.label(ctx, &state)
	state.Status = tmprl.Status(a.ExecutionStatus.String)
	state.StartedAt, state.ClosedAt, state.UpdatedAt = a.StartedAt.Time, a.ClosedAt.Time, a.SourceUpdatedAt.Time
	state.Request.PreparationOnly = entry.Job.Mode == "preparation"
	state.StateStale = a.StateStale || !a.ObservedAt.Valid || s.job == nil || time.Since(a.ObservedAt.Time) > 15*time.Second
	if state.StateStale {
		state.RemainingSeconds = 0
	}
	state.PendingActions = make([]PendingAction, 0, len(entry.Pending))
	for _, intent := range entry.Pending {
		state.PendingActions = append(state.PendingActions, PendingAction{ID: intent.IntentID, Kind: intent.Kind, State: intent.State})
	}
	if a.ResultDatasetID.Valid {
		dataset, err := s.Get(ctx, a.ResultDatasetID.String, int(a.ResultDatasetVersion.Int64))
		if err != nil {
			return State{}, err
		}
		state.PreparedDatasetID, state.PreparedDatasetVersion = dataset.ID, dataset.Version
		state.DatasetAvailable, state.DatasetRecordCount, state.TrainingReady = true, dataset.RecordCount, dataset.TrainingReady
	}
	return state, nil
}

// label names the run's target dataset and source document. Both names are
// immutable, so each is looked up once; a failed lookup leaves only the ID.
func (s *Service) label(ctx context.Context, state *State) {
	if id := state.TargetDatasetID; id != "" {
		state.TargetDatasetName = s.name("dataset/"+id, func() (string, error) {
			if s.db == nil {
				dataset, err := s.files.Get(ctx, id, 0)
				return dataset.Name, err
			}
			all, err := s.db.ListDatasets(ctx)
			for _, dataset := range all {
				if dataset.ID == id {
					return dataset.Name, err
				}
			}
			return "", err
		})
	}
	if id := state.SourceUploadID; id != "" && s.uploads != nil {
		state.SourceDocument = s.name("upload/"+id, func() (string, error) {
			file, err := s.uploads.Get(ctx, id)
			return file.Name, err
		})
	}
}

func (s *Service) name(key string, lookup func() (string, error)) string {
	if name, ok := s.names.Load(key); ok {
		return name.(string)
	}
	name, err := lookup()
	if err != nil || name == "" {
		return ""
	}
	s.names.Store(key, name)
	return name
}
