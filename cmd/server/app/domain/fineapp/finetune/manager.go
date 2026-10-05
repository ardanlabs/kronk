package finetune

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	runStateFile         = "run.json"
	legacyRunStateSchema = 1
	runStateSchema       = 2
)

var (
	// ErrRunActive reports a conflicting start or resume request.
	ErrRunActive = errors.New("another training run is already active")
	// ErrRunNotResumable reports a run without a durable resumable checkpoint.
	ErrRunNotResumable = errors.New("training run is not resumable")
	// ErrNoActiveRun reports a stop request with no active worker.
	ErrNoActiveRun = errors.New("no training run is active")
)

// RunInfo is the durable state shown by Current Run and History.
type RunInfo struct {
	Schema             int            `json:"schema"`
	ID                 string         `json:"id"`
	DatasetID          string         `json:"dataset_id"`
	Dataset            string         `json:"dataset"`
	ModelID            string         `json:"model_id"`
	Model              string         `json:"model"`
	Output             string         `json:"output"`
	Runtime            Runtime        `json:"runtime"`
	Training           TrainingConfig `json:"training"`
	Status             Status         `json:"status"`
	Progress           Progress       `json:"progress"`
	Error              string         `json:"error,omitempty"`
	Resumable          bool           `json:"resumable,omitempty"`
	Adapter            string         `json:"adapter,omitempty"`
	Report             string         `json:"report,omitempty"`
	SelectedEpoch      int            `json:"selected_epoch,omitempty"`
	SelectedCheckpoint string         `json:"selected_checkpoint,omitempty"`
	StartedAt          time.Time      `json:"started_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	FinishedAt         time.Time      `json:"finished_at"`
}

// UpdateFunc receives persisted snapshots as a managed run progresses.
type UpdateFunc func(RunInfo)

// Manager owns one local training run and its durable history. The MVP permits
// one active run because it targets one local accelerator.
type Manager struct {
	mu      sync.RWMutex
	root    string
	backend Backend
	now     func() time.Time
	write   func(RunInfo) error

	current        RunInfo
	hasCurrent     bool
	active         bool
	stopping       bool
	cancel         context.CancelFunc
	persistenceErr error
}

// NewManager creates the application-owned run directory.
func NewManager(root string, backend Backend) (*Manager, error) {
	if root == "" {
		return nil, errors.New("run directory is required")
	}
	if backend == nil {
		return nil, errors.New("training backend is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve run directory %q: %w", root, err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("create run directory %q: %w", absolute, err)
	}
	if err := os.Chmod(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("secure run directory %q: %w", absolute, err)
	}
	return &Manager{root: absolute, backend: backend, now: time.Now, write: writeRunInfo}, nil
}

// Start validates and starts one background run. Manager owns Output so UI
// code never creates or names run directories.
func (manager *Manager) Start(ctx context.Context, request Request, update UpdateFunc) (RunInfo, error) {
	if request.Output != "" {
		return RunInfo{}, errors.New("managed runs must not set an output directory")
	}
	if request.Dataset == "" || request.Model == "" {
		return RunInfo{}, errors.New("dataset and model are required")
	}

	manager.mu.Lock()
	if manager.persistenceErr != nil {
		manager.mu.Unlock()
		return RunInfo{}, fmt.Errorf("previous training result is not durable: %w", manager.persistenceErr)
	}
	if manager.active {
		manager.mu.Unlock()
		return RunInfo{}, ErrRunActive
	}
	now := manager.now()
	id, output, err := manager.nextOutput(now)
	if err != nil {
		manager.mu.Unlock()
		return RunInfo{}, err
	}
	request.Output = output
	if err := manager.backend.Preflight(ctx, request); err != nil {
		manager.mu.Unlock()
		return RunInfo{}, err
	}
	if err := os.Mkdir(output, 0o700); err != nil {
		manager.mu.Unlock()
		return RunInfo{}, fmt.Errorf("create run output %q: %w", output, err)
	}
	runContext, cancel := context.WithCancel(ctx)
	initial := RunInfo{
		Schema: runStateSchema, ID: id,
		DatasetID: resourceDatasetID(request), Dataset: request.Dataset,
		ModelID: resourceModelID(request), Model: request.Model, Output: output,
		Runtime: request.Runtime, Training: request.Training,
		Status: StatusPreparing, Progress: Progress{Stage: StageImporting, Message: "Preparing training run"},
		StartedAt: now, UpdatedAt: now,
	}
	if err := manager.write(initial); err != nil {
		cancel()
		_ = os.Remove(output)
		manager.mu.Unlock()
		return RunInfo{}, err
	}
	manager.current = initial
	manager.hasCurrent = true
	manager.active = true
	manager.cancel = cancel
	manager.mu.Unlock()

	go manager.execute(runContext, request, update, initial)
	return initial, nil
}

// Resume continues a failed or stopped run in its existing output directory.
// The backend decides which durable phase can be resumed.
func (manager *Manager) Resume(ctx context.Context, id string, update UpdateFunc) (RunInfo, error) {
	if id == "" || filepath.Base(id) != id {
		return RunInfo{}, errors.New("training run ID is invalid")
	}

	manager.mu.Lock()
	if manager.persistenceErr != nil {
		manager.mu.Unlock()
		return RunInfo{}, fmt.Errorf("previous training result is not durable: %w", manager.persistenceErr)
	}
	if manager.active {
		manager.mu.Unlock()
		return RunInfo{}, ErrRunActive
	}
	info, err := readRunInfo(filepath.Join(manager.root, id))
	if err != nil {
		manager.mu.Unlock()
		return RunInfo{}, err
	}
	if info.Status != StatusFailed && info.Status != StatusStopped && info.Status != StatusNeedsImprovement {
		manager.mu.Unlock()
		return RunInfo{}, fmt.Errorf("%w: only an incomplete training run can be resumed", ErrRunNotResumable)
	}
	if !info.Resumable || info.SelectedCheckpoint == "" {
		manager.mu.Unlock()
		return RunInfo{}, fmt.Errorf("%w: this run has no completed training checkpoint", ErrRunNotResumable)
	}
	request := Request{
		DatasetID: info.DatasetID, Dataset: info.Dataset,
		ModelID: info.ModelID, Model: info.Model, Output: info.Output,
		Runtime: info.Runtime, Training: info.Training,
	}
	if err := manager.backend.Preflight(ctx, request); err != nil {
		manager.mu.Unlock()
		return RunInfo{}, err
	}

	runContext, cancel := context.WithCancel(ctx)
	now := manager.now()
	info.Status = StatusPreparing
	info.Progress = Progress{Stage: StageImporting, Message: "Resuming from the selected checkpoint"}
	info.Error = ""
	info.Adapter = ""
	info.Report = ""
	info.FinishedAt = time.Time{}
	info.UpdatedAt = now
	if err := manager.write(info); err != nil {
		cancel()
		manager.mu.Unlock()
		return RunInfo{}, err
	}
	manager.current = info
	manager.hasCurrent = true
	manager.active = true
	manager.cancel = cancel
	manager.mu.Unlock()

	go manager.execute(runContext, request, update, info)
	return info, nil
}

// Stop requests cancellation. A second run is accepted only after the worker
// has persisted its terminal state.
func (manager *Manager) Stop() error {
	manager.mu.Lock()
	if !manager.active || manager.cancel == nil || manager.stopping {
		manager.mu.Unlock()
		return ErrNoActiveRun
	}
	manager.stopping = true
	cancel := manager.cancel
	manager.mu.Unlock()
	cancel()
	return nil
}

// Current returns the newest in-memory snapshot, including a terminal run.
func (manager *Manager) Current() (RunInfo, bool) {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.current, manager.hasCurrent
}

// History returns durable runs newest first.
func (manager *Manager) History() ([]RunInfo, error) {
	roots, err := manager.runRoots()
	if err != nil {
		return nil, err
	}
	runs := make([]RunInfo, 0, len(roots))
	for _, root := range roots {
		info, err := readRunInfo(root)
		if err != nil {
			return nil, err
		}
		runs = append(runs, info)
	}
	sort.Slice(runs, func(left, right int) bool {
		if runs[left].StartedAt.Equal(runs[right].StartedAt) {
			return runs[left].ID > runs[right].ID
		}
		return runs[left].StartedAt.After(runs[right].StartedAt)
	})
	return runs, nil
}

// Recover marks runs interrupted by an application exit as failed.
func (manager *Manager) Recover() ([]RunInfo, error) {
	manager.mu.RLock()
	active := manager.active
	manager.mu.RUnlock()
	if active {
		return nil, errors.New("cannot recover history while training is active")
	}
	roots, err := manager.runRoots()
	if err != nil {
		return nil, err
	}
	for _, root := range roots {
		info, err := readRunInfo(root)
		if err != nil {
			return nil, err
		}
		if info.Status != StatusPreparing && info.Status != StatusTraining && info.Status != StatusEvaluating {
			continue
		}
		if recovery, ok := manager.backend.(RecoveryBackend); ok {
			request := Request{
				DatasetID: info.DatasetID, Dataset: info.Dataset,
				ModelID: info.ModelID, Model: info.Model, Output: info.Output,
				Runtime: info.Runtime, Training: info.Training,
			}
			result, found, recoveryErr := recovery.Recover(context.Background(), request)
			if recoveryErr == nil && found {
				if err := validateResultPaths(info.Output, result); err != nil {
					recoveryErr = err
				} else {
					info.Resumable = result.Resumable
					info.SelectedEpoch = result.SelectedEpoch
					info.SelectedCheckpoint = result.SelectedCheckpoint
				}
			}
			if recoveryErr != nil {
				info.Resumable = false
				info.SelectedEpoch = 0
				info.SelectedCheckpoint = ""
				info.Error = fmt.Sprintf("recover durable training checkpoint: %v", recoveryErr)
			}
		}
		now := manager.now()
		info.Status = StatusFailed
		if info.Error == "" {
			info.Error = "training ended before Moxie could record a final result"
		}
		info.UpdatedAt = now
		info.FinishedAt = now
		if err := manager.write(info); err != nil {
			return nil, err
		}
	}
	return manager.History()
}

func (manager *Manager) execute(ctx context.Context, request Request, update UpdateFunc, initial RunInfo) {
	if update != nil {
		update(initial)
	}
	var progressErr error
	result, runErr := manager.backend.Run(ctx, request, func(progress Progress) {
		manager.mu.Lock()
		if progressErr != nil {
			manager.mu.Unlock()
			return
		}
		if err := validateProgress(progress); err != nil {
			progressErr = err
			cancel := manager.cancel
			manager.mu.Unlock()
			if cancel != nil {
				cancel()
			}
			return
		}
		current := manager.current
		if progress.Resumable {
			if err := validateResultPaths(request.Output, Result{SelectedCheckpoint: progress.SelectedCheckpoint}); err != nil {
				progressErr = err
				cancel := manager.cancel
				manager.mu.Unlock()
				if cancel != nil {
					cancel()
				}
				return
			}
			current.Resumable = true
			current.SelectedEpoch = progress.SelectedEpoch
			current.SelectedCheckpoint = progress.SelectedCheckpoint
		}
		current.Progress = progress
		current.Status = statusForStage(progress.Stage)
		current.UpdatedAt = manager.now()
		if err := manager.write(current); err != nil {
			progressErr = err
			cancel := manager.cancel
			manager.mu.Unlock()
			if cancel != nil {
				cancel()
			}
			return
		}
		manager.current = current
		manager.mu.Unlock()
		if update != nil {
			update(current)
		}
	})
	if progressErr != nil {
		runErr = progressErr
	}
	if err := validateResult(result); err != nil && runErr == nil {
		runErr = err
	}
	if err := validateResultPaths(request.Output, result); err != nil {
		result.Adapter = ""
		result.Report = ""
		result.SelectedCheckpoint = ""
		if runErr == nil {
			runErr = err
		}
	}

	manager.mu.Lock()
	finished := manager.finishInfo(manager.current, result, runErr)
	manager.persistenceErr = nil
	if err := manager.write(finished); err != nil {
		finished.Status = StatusFailed
		finished.Adapter = ""
		finished.Error = fmt.Sprintf("persist terminal training state: %v", err)
		finished.UpdatedAt = manager.now()
		finished.FinishedAt = finished.UpdatedAt
		if retryErr := manager.write(finished); retryErr != nil {
			manager.persistenceErr = fmt.Errorf("%v; persist failure state: %w", err, retryErr)
			finished.Error = manager.persistenceErr.Error()
		}
	}
	manager.current = finished
	manager.active = false
	manager.stopping = false
	manager.cancel = nil
	manager.mu.Unlock()
	if update != nil {
		update(finished)
	}
}

func (manager *Manager) finishInfo(info RunInfo, result Result, runErr error) RunInfo {
	info.Adapter = result.Adapter
	info.Report = result.Report
	info.SelectedEpoch = result.SelectedEpoch
	info.SelectedCheckpoint = result.SelectedCheckpoint
	info.Status = result.Status
	info.Error = result.Error
	info.Resumable = result.Resumable
	switch {
	case runErr == nil && info.Status == "":
		info.Status = StatusCompleted
	case errors.Is(runErr, context.Canceled):
		info.Status = StatusStopped
	case runErr != nil:
		info.Status = StatusFailed
	}
	if runErr != nil && info.Error == "" {
		info.Error = runErr.Error()
	}
	if info.Status != StatusCompleted {
		info.Adapter = ""
	}
	now := manager.now()
	info.UpdatedAt = now
	if info.FinishedAt.IsZero() {
		info.FinishedAt = now
	}
	if info.Status == StatusCompleted {
		info.Progress = Progress{Stage: StageComplete, Completed: 1, Total: 1, Message: "Training run completed"}
	}
	return info
}

func (manager *Manager) nextOutput(now time.Time) (string, string, error) {
	base := "run-" + now.UTC().Format("20060102-150405.000000000")
	for suffix := 1; ; suffix++ {
		id := base
		if suffix > 1 {
			id += "-" + strconv.Itoa(suffix)
		}
		output := filepath.Join(manager.root, id)
		_, err := os.Lstat(output)
		switch {
		case errors.Is(err, os.ErrNotExist):
			return id, output, nil
		case err != nil:
			return "", "", fmt.Errorf("inspect run output %q: %w", output, err)
		}
	}
}

func (manager *Manager) runRoots() ([]string, error) {
	entries, err := os.ReadDir(manager.root)
	if err != nil {
		return nil, fmt.Errorf("read run directory %q: %w", manager.root, err)
	}
	var roots []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		root := filepath.Join(manager.root, entry.Name())
		if _, err := os.Stat(filepath.Join(root, runStateFile)); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("inspect run state in %q: %w", root, err)
		}
		roots = append(roots, root)
	}
	return roots, nil
}

func readRunInfo(root string) (RunInfo, error) {
	data, err := os.ReadFile(filepath.Join(root, runStateFile))
	if err != nil {
		return RunInfo{}, fmt.Errorf("read run state in %q: %w", root, err)
	}
	var info RunInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return RunInfo{}, fmt.Errorf("decode run state in %q: %w", root, err)
	}
	if err := validateRunInfo(root, &info); err != nil {
		return RunInfo{}, fmt.Errorf("run state in %q is invalid: %w", root, err)
	}
	return info, nil
}

func validateRunInfo(root string, info *RunInfo) error {
	if info.Schema != legacyRunStateSchema && info.Schema != runStateSchema {
		return fmt.Errorf("unsupported schema %d", info.Schema)
	}
	if info.ID == "" || filepath.Base(root) != info.ID || info.Output != root {
		return errors.New("run identity does not match its directory")
	}
	if info.Dataset == "" || info.Model == "" {
		return errors.New("dataset and model paths are required")
	}
	if info.DatasetID == "" {
		info.DatasetID = filepath.Base(filepath.Dir(info.Dataset))
	}
	if info.ModelID == "" {
		info.ModelID = strings.TrimSuffix(filepath.Base(info.Model), filepath.Ext(info.Model))
	}
	if info.DatasetID == "" || info.ModelID == "" {
		return errors.New("dataset and model identities are required")
	}
	if info.Training != (TrainingConfig{}) {
		if err := info.Training.Validate(); err != nil {
			return fmt.Errorf("training configuration: %w", err)
		}
	}
	if err := validateProgress(info.Progress); err != nil {
		return err
	}
	switch info.Status {
	case StatusPreparing, StatusTraining, StatusEvaluating, StatusCompleted, StatusNeedsImprovement, StatusFailed, StatusStopped:
	default:
		return fmt.Errorf("unknown status %q", info.Status)
	}
	if info.StartedAt.IsZero() || info.UpdatedAt.Before(info.StartedAt) {
		return errors.New("run timestamps are inconsistent")
	}
	terminal := info.Status == StatusCompleted || info.Status == StatusNeedsImprovement || info.Status == StatusFailed || info.Status == StatusStopped
	if terminal == info.FinishedAt.IsZero() {
		return errors.New("run completion timestamp does not match its status")
	}
	if !info.FinishedAt.IsZero() && info.FinishedAt.Before(info.StartedAt) {
		return errors.New("run completion precedes its start")
	}
	if info.Resumable && (info.SelectedEpoch <= 0 || info.SelectedCheckpoint == "") {
		return errors.New("resumable run has no selected checkpoint")
	}
	if err := validateResultPaths(root, Result{Adapter: info.Adapter, Report: info.Report, SelectedCheckpoint: info.SelectedCheckpoint}); err != nil {
		return err
	}
	info.Schema = runStateSchema
	return nil
}

func resourceDatasetID(request Request) string {
	if request.DatasetID != "" {
		return request.DatasetID
	}
	return filepath.Base(filepath.Dir(request.Dataset))
}

func resourceModelID(request Request) string {
	if request.ModelID != "" {
		return request.ModelID
	}
	return strings.TrimSuffix(filepath.Base(request.Model), filepath.Ext(request.Model))
}

func writeRunInfo(info RunInfo) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("encode run state: %w", err)
	}
	temporary, err := os.CreateTemp(info.Output, ".run-*.json")
	if err != nil {
		return fmt.Errorf("create run state in %q: %w", info.Output, err)
	}
	temporaryPath := temporary.Name()
	keep := false
	defer func() {
		_ = temporary.Close()
		if !keep {
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write run state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync run state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close run state: %w", err)
	}
	if err := os.Chmod(temporaryPath, 0o600); err != nil {
		return fmt.Errorf("secure run state: %w", err)
	}
	if err := os.Rename(temporaryPath, filepath.Join(info.Output, runStateFile)); err != nil {
		return fmt.Errorf("publish run state: %w", err)
	}
	keep = true
	return nil
}

func statusForStage(stage Stage) Status {
	switch stage {
	case StageTraining:
		return StatusTraining
	case StageFinalEvaluation, StagePublishing, StageComplete:
		return StatusEvaluating
	default:
		return StatusPreparing
	}
}

func validateProgress(progress Progress) error {
	switch progress.Stage {
	case StageImporting, StageValidating, StageTokenizing, StageTraining,
		StageFinalEvaluation, StagePublishing, StageComplete:
	default:
		return fmt.Errorf("training backend reported unknown stage %q", progress.Stage)
	}
	if progress.Completed < 0 || progress.Total < 0 || progress.Completed > progress.Total {
		return fmt.Errorf("training backend reported invalid progress %d of %d", progress.Completed, progress.Total)
	}
	if progress.Resumable && (progress.SelectedEpoch <= 0 || progress.SelectedCheckpoint == "") {
		return errors.New("training backend reported resumable progress without a selected checkpoint")
	}
	return nil
}

func validateResultPaths(root string, result Result) error {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve managed run path: %w", err)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve managed run: %w", err)
	}
	artifacts := []struct {
		label     string
		path      string
		directory bool
	}{
		{"adapter", result.Adapter, false},
		{"report", result.Report, false},
		{"selected checkpoint", result.SelectedCheckpoint, true},
	}
	for _, artifact := range artifacts {
		if artifact.path == "" {
			continue
		}
		absolutePath, err := filepath.Abs(artifact.path)
		if err != nil {
			return fmt.Errorf("resolve training backend %s path: %w", artifact.label, err)
		}
		relative, err := filepath.Rel(absoluteRoot, absolutePath)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return fmt.Errorf("training backend returned %s outside the managed run", artifact.label)
		}
		info, err := os.Lstat(artifact.path)
		if err != nil {
			return fmt.Errorf("training backend returned missing %s: %w", artifact.label, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || artifact.directory != info.IsDir() || (!artifact.directory && !info.Mode().IsRegular()) {
			kind := "regular file"
			if artifact.directory {
				kind = "directory"
			}
			return fmt.Errorf("training backend returned %s that is not a %s", artifact.label, kind)
		}
		realPath, err := filepath.EvalSymlinks(artifact.path)
		if err != nil {
			return fmt.Errorf("resolve training backend %s: %w", artifact.label, err)
		}
		relative, err = filepath.Rel(realRoot, realPath)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return fmt.Errorf("training backend returned %s outside the managed run", artifact.label)
		}
	}
	return nil
}

func validateResult(result Result) error {
	switch result.Status {
	case "", StatusCompleted, StatusNeedsImprovement, StatusFailed, StatusStopped:
	default:
		return fmt.Errorf("training backend returned invalid terminal status %q", result.Status)
	}
	if result.Status == StatusCompleted && result.Error != "" {
		return errors.New("training backend returned a completed result with an error")
	}
	if result.Status == StatusCompleted && result.Adapter == "" {
		return errors.New("training backend returned a completed result without an adapter")
	}
	return nil
}
