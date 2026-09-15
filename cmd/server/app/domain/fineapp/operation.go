package fineapp

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	preparationRunning   = "running"
	preparationStopping  = "stopping"
	preparationCompleted = "completed"
	preparationStopped   = "stopped"
	preparationFailed    = "failed"
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
	return operations.current, operations.nextID > 0
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
