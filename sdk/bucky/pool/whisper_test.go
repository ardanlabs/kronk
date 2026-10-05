package pool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/bucky/model"
	"github.com/ardanlabs/kronk/sdk/pool/engine/loader"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
	toolmodels "github.com/ardanlabs/kronk/sdk/tools/models"
)

func TestPrepareAppliesBuckyModelConfig(t *testing.T) {
	models, err := buckymodels.NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("buckymodels.NewWithPaths: %v", err)
	}
	modelPath := filepath.Join(models.Path(), "ggml-tiny.bin")
	if err := os.WriteFile(modelPath, []byte("model"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := models.BuildIndex(nil, false); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	nSeqMax := 3
	queueDepth := 5
	nThreads := int32(7)
	timeout := toolmodels.Duration(45 * time.Second)
	w := newWhisper(func(context.Context, string, ...any) {}, models, map[string]modelconfig.BuckyModelConfig{
		"tiny": {
			NSeqMax:          &nSeqMax,
			QueueDepth:       &queueDepth,
			AdmissionTimeout: &timeout,
			NThreads:         &nThreads,
		},
	}, nil)

	prepared, err := w.Prepare(t.Context(), loader.LoadRequest{ModelID: "tiny", Key: "tiny"})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	cfg, ok := prepared.(model.Config)
	if !ok {
		t.Fatalf("Prepare config type: got %T, want model.Config", prepared)
	}
	if cfg.ModelPath != modelPath {
		t.Errorf("ModelPath: got %q, want %q", cfg.ModelPath, modelPath)
	}
	if cfg.NSeqMax != nSeqMax {
		t.Errorf("NSeqMax: got %d, want %d", cfg.NSeqMax, nSeqMax)
	}
	if cfg.QueueDepth != queueDepth {
		t.Errorf("QueueDepth: got %d, want %d", cfg.QueueDepth, queueDepth)
	}
	if cfg.AdmissionTimeout != time.Duration(timeout) {
		t.Errorf("AdmissionTimeout: got %s, want %s", cfg.AdmissionTimeout, time.Duration(timeout))
	}
	if cfg.NThreads != nThreads {
		t.Errorf("NThreads: got %d, want %d", cfg.NThreads, nThreads)
	}
}

func TestPrepareDefaultsBuckyConcurrency(t *testing.T) {
	models, err := buckymodels.NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("buckymodels.NewWithPaths: %v", err)
	}
	if err := os.WriteFile(filepath.Join(models.Path(), "ggml-tiny.bin"), []byte("model"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := models.BuildIndex(nil, false); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	w := newWhisper(func(context.Context, string, ...any) {}, models, nil, nil)
	prepared, err := w.Prepare(t.Context(), loader.LoadRequest{ModelID: "tiny", Key: "tiny"})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	cfg := prepared.(model.Config)
	if cfg.NSeqMax != 1 {
		t.Errorf("NSeqMax: got %d, want 1", cfg.NSeqMax)
	}
}

func TestPlanScalesMemoryByBuckyConcurrency(t *testing.T) {
	models, err := buckymodels.NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("buckymodels.NewWithPaths: %v", err)
	}
	modelData := []byte("model")
	if err := os.WriteFile(filepath.Join(models.Path(), "ggml-tiny.bin"), modelData, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := models.BuildIndex(nil, false); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	rm, err := resman.New(resman.Config{
		Snapshot:      resman.Snapshot{RAMBytes: 8 << 30},
		BudgetPercent: 100,
	})
	if err != nil {
		t.Fatalf("resman.New: %v", err)
	}
	nSeqMax := 3
	queueDepth := 7
	w := newWhisper(func(context.Context, string, ...any) {}, models, map[string]modelconfig.BuckyModelConfig{
		"tiny": {
			NSeqMax:    &nSeqMax,
			QueueDepth: &queueDepth,
		},
	}, rm)
	req := loader.LoadRequest{ModelID: "tiny", Key: "tiny"}
	req.Prepared, err = w.Prepare(t.Context(), req)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	plan, err := w.Plan(t.Context(), req)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	want := int64(len(modelData)) + whisperStateOverhead*int64(nSeqMax)
	if plan.RAMBytes != want {
		t.Errorf("RAMBytes: got %d, want %d", plan.RAMBytes, want)
	}
	if plan.VRAMBytes != 0 {
		t.Errorf("VRAMBytes: got %d, want 0", plan.VRAMBytes)
	}
}
