package fineapp

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPreparationOperationReportsProgressAndStopsItsWork(t *testing.T) {
	operations := newPreparationOperations()
	now := time.Date(2026, time.September, 14, 18, 0, 0, 0, time.UTC)
	operations.now = func() time.Time { return now }

	ctx, id, err := operations.start(t.Context(), "build", "Preparing dataset build", 32)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := operations.start(t.Context(), "preview", "Generating preview", 2); err == nil {
		t.Fatal("concurrent preparation start: got nil error")
	}

	now = now.Add(2 * time.Second)
	operations.update(id, "Generating verified facts", 7, 32, 21)
	progress, exists := operations.currentStatus()
	if !exists {
		t.Fatal("current progress does not exist")
	}
	if progress.Status != preparationRunning || progress.Stage != "Generating verified facts" || progress.Completed != 7 || progress.Total != 32 || progress.AcceptedRecords != 21 {
		t.Fatalf("progress: got %+v", progress)
	}
	if !progress.StartedAt.Equal(time.Date(2026, time.September, 14, 18, 0, 0, 0, time.UTC)) || !progress.UpdatedAt.Equal(now) {
		t.Fatalf("progress timestamps: got %+v", progress)
	}

	if err := operations.stop(); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(context.Cause(ctx), context.Canceled) {
		t.Fatalf("operation context: got %v", context.Cause(ctx))
	}
	operations.finish(id, context.Canceled)
	progress, _ = operations.currentStatus()
	if progress.Status != preparationStopped || progress.Error != "" {
		t.Fatalf("stopped progress: got %+v", progress)
	}
	if err := operations.stop(); err == nil {
		t.Fatal("stopping completed operation: got nil error")
	}
}

func TestPreparationOperationPreservesFailure(t *testing.T) {
	operations := newPreparationOperations()
	_, id, err := operations.start(t.Context(), "preview", "Generating preview", 2)
	if err != nil {
		t.Fatal(err)
	}
	operations.finish(id, errors.New("generation failed"))

	progress, exists := operations.currentStatus()
	if !exists || progress.Status != preparationFailed || progress.Error != "generation failed" {
		t.Fatalf("failed progress: got %+v, exists %t", progress, exists)
	}
}
