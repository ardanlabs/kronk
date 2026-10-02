package fineapp

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/prepare"
)

const (
	preparationRunning   = "running"
	preparationStopping  = "stopping"
	preparationCompleted = "completed"
	preparationStopped   = "stopped"
	preparationFailed    = "failed"
	documentQueued       = "queued"
	documentExtracting   = "extracting"
	documentProcessing   = "processing"
	documentCompleted    = "completed"
	documentFailed       = "failed"
	documentStopped      = "stopped"
)

type preparationOperations struct {
	mu             sync.RWMutex
	now            func() time.Time
	nextID         int
	active         bool
	cancel         context.CancelFunc
	current        PreparationProgressResponse
	phaseStartedAt []time.Time
	phaseNames     []string
}

func newPreparationOperations() *preparationOperations {
	return &preparationOperations{now: time.Now}
}

func (operations *preparationOperations) start(parent context.Context, operation, stage string, total int) (context.Context, int, error) {
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if operations.active {
		return nil, 0, errors.New("another dataset preparation operation is already active")
	}

	ctx, cancel := context.WithCancel(parent)
	now := operations.now().UTC()
	operations.nextID++
	operations.active = true
	operations.cancel = cancel
	operations.current = PreparationProgressResponse{
		Operation: operation,
		Status:    preparationRunning,
		Stage:     stage,
		Total:     total,
		StartedAt: now,
		UpdatedAt: now,
	}
	operations.phaseStartedAt = nil
	operations.phaseNames = nil
	return ctx, operations.nextID, nil
}

func (operations *preparationOperations) update(id int, stage string, completed, total, accepted int) {
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if id != operations.nextID || !operations.active {
		return
	}
	operations.current.Stage = stage
	operations.current.Phase = ""
	operations.current.Completed = completed
	operations.current.Total = total
	operations.current.AcceptedRecords = accepted
	operations.current.CompletedChunks = 0
	operations.current.TotalChunks = 0
	operations.current.PlannedRecords = 0
	operations.current.EstimatedRemainingSeconds = 0
	operations.current.UpdatedAt = operations.now().UTC()
}

func (operations *preparationOperations) initializeDocuments(id int, names []string) {
	ids := make([]string, len(names))
	for index := range names {
		ids[index] = fmt.Sprintf("%03d", index+1)
	}
	operations.initializeDocumentDetails(id, ids, names)
}

func (operations *preparationOperations) initializeDocumentDetails(id int, ids, names []string) {
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if id != operations.nextID || !operations.active || len(ids) != len(names) {
		return
	}
	operations.current.Documents = make([]PreparationDocumentProgressResponse, len(names))
	operations.phaseStartedAt = make([]time.Time, len(names))
	operations.phaseNames = make([]string, len(names))
	for index, name := range names {
		operations.current.Documents[index] = PreparationDocumentProgressResponse{ID: ids[index], Name: name, Status: documentQueued, Stage: "Waiting"}
	}
	operations.current.UpdatedAt = operations.now().UTC()
}

func (operations *preparationOperations) updateBuildProgress(id, index int, progress prepare.FullBuildProgress) {
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if id != operations.nextID || !operations.active || index < 0 || index >= len(operations.current.Documents) {
		return
	}

	now := operations.now().UTC()
	if operations.phaseNames[index] != progress.Phase {
		operations.phaseNames[index] = progress.Phase
		operations.phaseStartedAt[index] = now
	}
	completed, total := progress.AcceptedRecords, progress.PlannedRecords
	estimateCompleted, estimateTotal := completed, total
	switch progress.Phase {
	case "analysis":
		completed, total = progress.CompletedChunks, progress.TotalChunks
		estimateCompleted, estimateTotal = completed, total
	case "generation":
		if progress.CompletedBatches > 0 && progress.TotalBatches > 0 {
			estimateCompleted, estimateTotal = progress.CompletedBatches, progress.TotalBatches
		}
	case "finalization":
		total = 1
		completed = 0
		if progress.Stage == "Dataset preparation complete" {
			completed = 1
		}
		estimateCompleted, estimateTotal = completed, total
	}
	estimatedRemaining := estimateRemainingSeconds(operations.phaseStartedAt[index], now, estimateCompleted, estimateTotal)
	document := &operations.current.Documents[index]
	document.Status = documentProcessing
	document.Stage = progress.Stage
	document.Phase = progress.Phase
	document.AcceptedRecords = progress.AcceptedRecords
	document.AcceptedFacts = progress.AcceptedFacts
	document.CompletedBatches = progress.CompletedBatches
	document.TotalBatches = progress.TotalBatches
	document.CompletedChunks = progress.CompletedChunks
	document.TotalChunks = progress.TotalChunks
	document.PlannedRecords = progress.PlannedRecords
	document.EstimatedRemainingSeconds = estimatedRemaining
	operations.current.Stage = progress.Stage
	operations.current.Phase = progress.Phase
	operations.current.Completed = completed
	operations.current.Total = total
	operations.current.AcceptedRecords = progress.AcceptedRecords
	operations.current.CompletedChunks = progress.CompletedChunks
	operations.current.TotalChunks = progress.TotalChunks
	operations.current.PlannedRecords = progress.PlannedRecords
	operations.current.EstimatedRemainingSeconds = estimatedRemaining
	operations.current.UpdatedAt = now
}

func (operations *preparationOperations) updatePreviewProgress(id, index int, progress prepare.PreviewProgress) {
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if id != operations.nextID || !operations.active || index < 0 || index >= len(operations.current.Documents) {
		return
	}

	now := operations.now().UTC()
	if operations.phaseNames[index] != progress.Phase {
		operations.phaseNames[index] = progress.Phase
		operations.phaseStartedAt[index] = now
	}
	totalRecords := max(progress.TotalRecords, 0)
	completedRecords := min(max(progress.CompletedRecords, 0), totalRecords)
	estimatedRemaining := estimateRemainingSeconds(operations.phaseStartedAt[index], now, completedRecords, totalRecords)
	document := &operations.current.Documents[index]
	document.Status = documentProcessing
	document.Stage = progress.Stage
	document.Phase = progress.Phase
	document.AcceptedRecords = completedRecords
	document.PlannedRecords = totalRecords
	document.EstimatedRemainingSeconds = estimatedRemaining
	operations.current.Stage = progress.Stage
	operations.current.Phase = progress.Phase
	operations.current.Completed = completedRecords
	operations.current.Total = totalRecords
	operations.current.AcceptedRecords = completedRecords
	operations.current.PlannedRecords = totalRecords
	operations.current.EstimatedRemainingSeconds = estimatedRemaining
	operations.current.UpdatedAt = now
}

func estimateRemainingSeconds(started, now time.Time, completed, total int) int {
	if started.IsZero() || completed <= 0 || total <= completed {
		return 0
	}
	elapsed := now.Sub(started)
	if elapsed < time.Second {
		return 0
	}
	return max(1, int((elapsed*time.Duration(total-completed)/time.Duration(completed)).Round(time.Second)/time.Second))
}

func (operations *preparationOperations) updateDocumentWork(id, index int, status, stage string, acceptedRecords, acceptedFacts, targetFacts, completedBatches, totalBatches int, workErr error) {
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if id != operations.nextID || !operations.active || index < 0 || index >= len(operations.current.Documents) {
		return
	}

	document := &operations.current.Documents[index]
	document.Status = status
	document.Stage = stage
	document.Phase = ""
	document.AcceptedRecords = acceptedRecords
	document.AcceptedFacts = acceptedFacts
	document.TargetFacts = targetFacts
	document.CompletedBatches = completedBatches
	document.TotalBatches = totalBatches
	document.CompletedChunks = 0
	document.TotalChunks = 0
	document.PlannedRecords = 0
	document.EstimatedRemainingSeconds = 0
	if workErr != nil {
		document.Status = documentFailed
		document.Error = workErr.Error()
	}
	completed := 0
	accepted := 0
	for _, candidate := range operations.current.Documents {
		if candidate.Status == documentCompleted {
			completed++
		}
		accepted += candidate.AcceptedRecords
	}
	operations.current.Completed = completed
	operations.current.Total = len(operations.current.Documents)
	operations.current.AcceptedRecords = accepted
	operations.current.Stage = stage
	operations.current.Phase = ""
	operations.current.CompletedChunks = 0
	operations.current.TotalChunks = 0
	operations.current.PlannedRecords = 0
	operations.current.EstimatedRemainingSeconds = 0
	operations.current.UpdatedAt = operations.now().UTC()
}

func (operations *preparationOperations) updateDocument(id int, progress prepare.ExtractionProgress) {
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if id != operations.nextID || !operations.active || progress.Index < 0 || progress.Index >= len(operations.current.Documents) {
		return
	}

	document := &operations.current.Documents[progress.Index]
	document.Stage = progress.Stage
	document.Cached = progress.Cached
	switch {
	case progress.Error != "":
		document.Status = documentFailed
		document.Error = progress.Error
	case progress.Completed:
		document.Status = documentCompleted
	case document.Status == documentQueued:
		document.Status = documentExtracting
	}
	completed := 0
	for _, candidate := range operations.current.Documents {
		if candidate.Status == documentCompleted {
			completed++
		}
	}
	operations.current.Completed = completed
	operations.current.Total = len(operations.current.Documents)
	operations.current.Stage = progress.Stage
	operations.current.UpdatedAt = operations.now().UTC()
}

func (operations *preparationOperations) finish(id int, err error) {
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if id != operations.nextID || !operations.active {
		return
	}

	status := preparationCompleted
	switch {
	case errors.Is(err, context.Canceled):
		status = preparationStopped
		operations.current.Stage = "Stopped"
		operations.current.Phase = ""
		operations.current.EstimatedRemainingSeconds = 0
		for index := range operations.current.Documents {
			if operations.current.Documents[index].Status != documentCompleted {
				operations.current.Documents[index].Status = documentStopped
				operations.current.Documents[index].Stage = "Stopped"
			}
		}
	case err != nil:
		status = preparationFailed
	}
	operations.current.Status = status
	if err != nil && !errors.Is(err, context.Canceled) {
		operations.current.Error = err.Error()
	}
	operations.current.UpdatedAt = operations.now().UTC()
	operations.active = false
	operations.cancel = nil
}

func (operations *preparationOperations) currentStatus() (PreparationProgressResponse, bool) {
	operations.mu.RLock()
	defer operations.mu.RUnlock()
	current := operations.current
	current.Documents = slices.Clone(current.Documents)
	return current, operations.nextID > 0
}

func (operations *preparationOperations) stop() error {
	operations.mu.Lock()
	if !operations.active || operations.cancel == nil {
		operations.mu.Unlock()
		return errors.New("no dataset preparation operation is active")
	}
	operations.current.Status = preparationStopping
	operations.current.UpdatedAt = operations.now().UTC()
	cancel := operations.cancel
	operations.mu.Unlock()
	cancel()
	return nil
}
