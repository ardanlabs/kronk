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
	mu      sync.RWMutex
	now     func() time.Time
	nextID  int
	active  bool
	cancel  context.CancelFunc
	current PreparationProgressResponse
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
	return ctx, operations.nextID, nil
}

func (operations *preparationOperations) update(id int, stage string, completed, total, accepted int) {
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if id != operations.nextID || !operations.active {
		return
	}
	operations.current.Stage = stage
	operations.current.Completed = completed
	operations.current.Total = total
	operations.current.AcceptedRecords = accepted
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
	for index, name := range names {
		operations.current.Documents[index] = PreparationDocumentProgressResponse{ID: ids[index], Name: name, Status: documentQueued, Stage: "Waiting"}
	}
	operations.current.UpdatedAt = operations.now().UTC()
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
	document.AcceptedRecords = acceptedRecords
	document.AcceptedFacts = acceptedFacts
	document.TargetFacts = targetFacts
	document.CompletedBatches = completedBatches
	document.TotalBatches = totalBatches
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
