package engine

import (
	"context"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/pool/engine/loader"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
)

type preparedHandle struct{}

func (*preparedHandle) ActiveStreams() int {
	return 0
}

func (*preparedHandle) Unload(context.Context) error {
	return nil
}

type preparedLoader struct {
	prepareCalls int
	planPrepared any
	loadPrepared any
}

func (pl *preparedLoader) Prepare(context.Context, loader.LoadRequest) (any, error) {
	pl.prepareCalls++
	return new(int), nil
}

func (pl *preparedLoader) Plan(_ context.Context, req loader.LoadRequest) (resman.PlanRequest, error) {
	pl.planPrepared = req.Prepared
	return resman.PlanRequest{Key: req.Key, RAMBytes: 1}, nil
}

func (pl *preparedLoader) Load(_ context.Context, req loader.LoadRequest) (*preparedHandle, error) {
	pl.loadPrepared = req.Prepared
	return &preparedHandle{}, nil
}

func (*preparedLoader) Display(*preparedHandle, string) loader.Display {
	return loader.Display{}
}

func TestAcquirePreparesRequestOnceForPlanAndLoad(t *testing.T) {
	rm, err := resman.New(resman.Config{
		Snapshot:      resman.Snapshot{RAMBytes: 1024},
		BudgetPercent: 100,
		HeadroomBytes: -1,
	})
	if err != nil {
		t.Fatalf("resman.New: %v", err)
	}

	pl := &preparedLoader{}
	p, err := New(Config{
		Log:      func(context.Context, string, ...any) {},
		Resman:   rm,
		MaxItems: 1,
		TTL:      time.Minute,
	}, pl)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := p.Acquire(context.Background(), loader.LoadRequest{ModelID: "model", Key: "model"}); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	if pl.prepareCalls != 1 {
		t.Errorf("prepare calls: got %d, want 1", pl.prepareCalls)
	}
	if pl.planPrepared == nil {
		t.Fatal("Plan prepared value: got nil, want non-nil")
	}
	if pl.planPrepared != pl.loadPrepared {
		t.Error("prepared value differs between Plan and Load")
	}
}
