package engine

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/pool/engine/loader"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
)

type preparedHandle struct {
	unloadCalls atomic.Int32
	unloadErr   error
}

func (*preparedHandle) ActiveStreams() int {
	return 0
}

func (h *preparedHandle) Unload(context.Context) error {
	h.unloadCalls.Add(1)
	return h.unloadErr
}

type preparedLoader struct {
	prepareCalls int
	planPrepared any
	loadPrepared any
	handles      map[string]*preparedHandle
	validateErr  error
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

	h := &preparedHandle{}
	if pl.handles != nil {
		pl.handles[req.Key] = h
	}

	return h, nil
}

func (*preparedLoader) Display(*preparedHandle, string) loader.Display {
	return loader.Display{}
}

func (pl *preparedLoader) Validate(context.Context, loader.LoadRequest, *preparedHandle) error {
	return pl.validateErr
}

func newTestResourceManager(t *testing.T) *resman.Manager {
	t.Helper()

	rm, err := resman.New(resman.Config{
		Snapshot:      resman.Snapshot{RAMBytes: 1024},
		BudgetPercent: 100,
		HeadroomBytes: -1,
	})
	if err != nil {
		t.Fatalf("resman.New: %v", err)
	}

	return rm
}

func TestAcquirePreparesRequestOnceForPlanAndLoad(t *testing.T) {
	pl := &preparedLoader{}
	p, err := New(Config{
		Log:      func(context.Context, string, ...any) {},
		Resman:   newTestResourceManager(t),
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

func TestAcquireValidationFailureUnloadsAndReleases(t *testing.T) {
	wantErr := errors.New("backend memory exhausted")
	pl := &preparedLoader{
		handles:     make(map[string]*preparedHandle),
		validateErr: wantErr,
	}
	rm := newTestResourceManager(t)
	p, err := New(Config{
		Log:      func(context.Context, string, ...any) {},
		Resman:   rm,
		MaxItems: 1,
	}, pl)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = p.Acquire(context.Background(), loader.LoadRequest{ModelID: "model", Key: "model"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Acquire error: got %v, want %v", err, wantErr)
	}
	if got := pl.handles["model"].unloadCalls.Load(); got != 1 {
		t.Errorf("Unload calls: got %d, want 1", got)
	}
	if got := len(rm.Usage().Reservations); got != 0 {
		t.Errorf("Reservations: got %d, want 0", got)
	}
	if _, exists := p.cache.GetIfPresent("model"); exists {
		t.Error("cache contains handle after validation failure")
	}
}

func TestNewTTL(t *testing.T) {
	tests := []struct {
		name    string
		ttl     time.Duration
		wantErr bool
	}{
		{name: "negative", ttl: -time.Second, wantErr: true},
		{name: "disabled"},
		{name: "enabled", ttl: time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(Config{
				Log:      func(context.Context, string, ...any) {},
				Resman:   newTestResourceManager(t),
				MaxItems: 1,
				TTL:      tt.ttl,
			}, &preparedLoader{})
			if (err != nil) != tt.wantErr {
				t.Fatalf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTTLDisabled(t *testing.T) {
	pl := &preparedLoader{}
	p, err := New(Config{
		Log:      func(context.Context, string, ...any) {},
		Resman:   newTestResourceManager(t),
		MaxItems: 1,
		TTL:      0,
	}, pl)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := p.Acquire(context.Background(), loader.LoadRequest{ModelID: "model", Key: "model"}); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	entry, exists := p.cache.GetEntryQuietly("model")
	if !exists {
		t.Fatal("cache entry: got missing, want present")
	}

	const noExpiration = time.Duration(1<<63 - 1)
	if got := entry.ExpiresAfter(); got != noExpiration {
		t.Errorf("ExpiresAfter: got %s, want %s", got, noExpiration)
	}
	if got := p.EntryExpiresAt(entry); !got.IsZero() {
		t.Errorf("EntryExpiresAt: got %s, want zero time", got)
	}
}

func TestTTLDisabledStillAllowsCapacityEviction(t *testing.T) {
	pl := &preparedLoader{handles: make(map[string]*preparedHandle)}
	p, err := New(Config{
		Log:      func(context.Context, string, ...any) {},
		Resman:   newTestResourceManager(t),
		MaxItems: 1,
		TTL:      0,
	}, pl)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if _, err := p.Acquire(ctx, loader.LoadRequest{ModelID: "model-a", Key: "model-a"}); err != nil {
		t.Fatalf("Acquire model-a: %v", err)
	}
	if _, err := p.Acquire(ctx, loader.LoadRequest{ModelID: "model-b", Key: "model-b"}); err != nil {
		t.Fatalf("Acquire model-b: %v", err)
	}

	if _, exists := p.GetExisting("model-a"); exists {
		t.Error("model-a: got resident, want capacity-evicted")
	}
	if _, exists := p.GetExisting("model-b"); !exists {
		t.Error("model-b: got missing, want resident")
	}
	if got := pl.handles["model-a"].unloadCalls.Load(); got != 1 {
		t.Errorf("model-a unload calls: got %d, want 1", got)
	}
}
