package fineapp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/prepare"
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

func TestPreparationOperationReportsPhaseProgressAndETA(t *testing.T) {
	operations := newPreparationOperations()
	now := time.Date(2026, time.September, 14, 18, 0, 0, 0, time.UTC)
	operations.now = func() time.Time { return now }
	_, id, err := operations.start(t.Context(), "build", "Preparing", 1)
	if err != nil {
		t.Fatal(err)
	}
	operations.initializeDocuments(id, []string{"guide.pdf"})
	operations.updateBuildProgress(id, 0, prepare.FullBuildProgress{Phase: "analysis", Stage: "Analyzing", CompletedChunks: 1, TotalChunks: 4})
	now = now.Add(10 * time.Second)
	operations.updateBuildProgress(id, 0, prepare.FullBuildProgress{Phase: "analysis", Stage: "Analyzing", CompletedChunks: 2, TotalChunks: 4})
	progress, _ := operations.currentStatus()
	if progress.Phase != "analysis" || progress.Completed != 2 || progress.Total != 4 || progress.EstimatedRemainingSeconds != 10 {
		t.Fatalf("analysis progress = %+v", progress)
	}

	now = now.Add(time.Second)
	operations.updateBuildProgress(id, 0, prepare.FullBuildProgress{Phase: "generation", Stage: "Generating", AcceptedRecords: 100, PlannedRecords: 2_000})
	progress, _ = operations.currentStatus()
	if progress.Completed != 100 || progress.Total != 2_000 || progress.EstimatedRemainingSeconds != 0 {
		t.Fatalf("initial generation progress = %+v", progress)
	}
	now = now.Add(20 * time.Second)
	operations.updateBuildProgress(id, 0, prepare.FullBuildProgress{Phase: "generation", Stage: "Generating", AcceptedRecords: 200, PlannedRecords: 2_000, CompletedBatches: 5, TotalBatches: 10})
	progress, _ = operations.currentStatus()
	if progress.EstimatedRemainingSeconds != 20 {
		t.Fatalf("generation ETA = %d, want 20 based on completed work batches", progress.EstimatedRemainingSeconds)
	}
}

func TestPreparationOperationReportsPreviewRecordProgressAndETA(t *testing.T) {
	operations := newPreparationOperations()
	now := time.Date(2026, time.September, 14, 18, 0, 0, 0, time.UTC)
	operations.now = func() time.Time { return now }
	_, id, err := operations.start(t.Context(), "preview", "Preparing", 1)
	if err != nil {
		t.Fatal(err)
	}
	operations.initializeDocuments(id, []string{"guide.pdf"})
	operations.updatePreviewProgress(id, 0, prepare.PreviewProgress{Phase: "generation", Stage: "Generating preview records", TotalRecords: 5})
	now = now.Add(12 * time.Second)
	operations.updatePreviewProgress(id, 0, prepare.PreviewProgress{Phase: "generation", Stage: "Generating preview records", CompletedRecords: 1, TotalRecords: 5})
	progress, _ := operations.currentStatus()
	if progress.Phase != "generation" || progress.Completed != 1 || progress.Total != 5 || progress.EstimatedRemainingSeconds != 48 {
		t.Fatalf("preview progress = %+v", progress)
	}
	if len(progress.Documents) != 1 || progress.Documents[0].AcceptedRecords != 1 || progress.Documents[0].PlannedRecords != 5 {
		t.Fatalf("preview document progress = %+v", progress.Documents)
	}
	operations.updatePreviewProgress(id, 0, prepare.PreviewProgress{Phase: "generation", Stage: "Verifying generated records", CompletedRecords: 7, TotalRecords: 5})
	progress, _ = operations.currentStatus()
	if progress.Completed != 5 || progress.Total != 5 || progress.Documents[0].AcceptedRecords != 5 {
		t.Fatalf("preview progress exceeded requested records: %+v", progress)
	}
}
