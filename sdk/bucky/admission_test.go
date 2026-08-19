package bucky

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/bucky/model"
)

func TestAcquireModelCallerCancellation(t *testing.T) {
	b := Bucky{
		cfg:         model.NewConfig(model.WithAdmissionTimeout(time.Hour)),
		admissionCh: make(chan struct{}, 1),
	}
	b.admissionCh <- struct{}{}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := b.acquireModel(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("acquireModel: got error %v, want %v", err, context.Canceled)
	}
	if active := b.ActiveStreams(); active != 0 {
		t.Errorf("ActiveStreams: got %d, want 0", active)
	}
}

func TestAcquireModelAdmissionTimeout(t *testing.T) {
	b := Bucky{
		cfg:         model.NewConfig(model.WithAdmissionTimeout(time.Millisecond)),
		admissionCh: make(chan struct{}, 1),
	}
	b.admissionCh <- struct{}{}

	_, err := b.acquireModel(t.Context())
	if !errors.Is(err, ErrAdmissionTimeout) {
		t.Fatalf("acquireModel: got error %v, want %v", err, ErrAdmissionTimeout)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("acquireModel: got context deadline for admission timeout: %v", err)
	}
	if active := b.ActiveStreams(); active != 0 {
		t.Errorf("ActiveStreams: got %d, want 0", active)
	}
}

func TestAcquireModelCallerDeadline(t *testing.T) {
	b := Bucky{
		cfg:         model.NewConfig(model.WithAdmissionTimeout(time.Hour)),
		admissionCh: make(chan struct{}, 1),
	}
	b.admissionCh <- struct{}{}

	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	defer cancel()

	_, err := b.acquireModel(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("acquireModel: got error %v, want %v", err, context.DeadlineExceeded)
	}
	if errors.Is(err, ErrAdmissionTimeout) {
		t.Fatalf("acquireModel: got admission timeout for caller deadline: %v", err)
	}
	if active := b.ActiveStreams(); active != 0 {
		t.Errorf("ActiveStreams: got %d, want 0", active)
	}
}

func TestAcquireModelAdmissionTimeoutIsLocal(t *testing.T) {
	b := Bucky{
		cfg:         model.NewConfig(model.WithAdmissionTimeout(time.Hour)),
		admissionCh: make(chan struct{}, 1),
	}

	ctx := t.Context()
	if _, err := b.acquireModel(ctx); err != nil {
		t.Fatalf("acquireModel: %v", err)
	}
	defer b.releaseModel()

	if err := ctx.Err(); err != nil {
		t.Errorf("caller context after admission: got %v, want nil", err)
	}
}

func TestAcquireModelQueueDepth(t *testing.T) {
	b := Bucky{
		cfg:         model.NewConfig(model.WithAdmissionTimeout(time.Millisecond)),
		admissionCh: make(chan struct{}, 2),
	}
	b.admissionCh <- struct{}{}

	if _, err := b.acquireModel(t.Context()); err != nil {
		t.Fatalf("acquireModel queued call: %v", err)
	}
	if admitted := len(b.admissionCh); admitted != 2 {
		t.Fatalf("admitted calls: got %d, want 2", admitted)
	}

	_, err := b.acquireModel(t.Context())
	if !errors.Is(err, ErrAdmissionTimeout) {
		t.Fatalf("acquireModel beyond queue: got error %v, want %v", err, ErrAdmissionTimeout)
	}

	b.releaseModel()
	<-b.admissionCh
	if active := b.ActiveStreams(); active != 0 {
		t.Errorf("ActiveStreams: got %d, want 0", active)
	}
}
