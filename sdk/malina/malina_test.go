package malina

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/malina/model"
)

type fakeBackend struct {
	entered chan struct{}
	release chan struct{}
	err     error
	mu      sync.Mutex
	running int
	max     int
	unloads int
}

func (fb *fakeBackend) Generate(context.Context, model.GenerateParams) (model.GeneratedImage, error) {
	fb.mu.Lock()
	fb.running++
	fb.max = max(fb.max, fb.running)
	fb.mu.Unlock()

	fb.entered <- struct{}{}
	<-fb.release

	fb.mu.Lock()
	fb.running--
	fb.mu.Unlock()

	return model.GeneratedImage{PNG: []byte("png")}, fb.err
}

func (fb *fakeBackend) Stop() {}

func (fb *fakeBackend) Unload() error {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	fb.unloads++

	return nil
}

func (fb *fakeBackend) Config() model.Config {
	return model.Config{QueueDepth: 2, AdmissionTimeout: time.Minute}
}

func (fb *fakeBackend) Info() model.ModelInfo {
	return model.ModelInfo{}
}

func newTestMalina(t *testing.T, fb *fakeBackend, depth int) *Malina {
	t.Helper()

	initState.Lock()
	oldDone := initState.done
	oldPath := initState.path
	initState.done = true
	initState.path = "test"
	initState.Unlock()

	oldBackend := newBackend
	newBackend = func(context.Context, model.Config) (backend, error) {
		return fb, nil
	}

	t.Cleanup(func() {
		newBackend = oldBackend
		initState.Lock()
		initState.done = oldDone
		initState.path = oldPath
		initState.Unlock()
	})

	m, err := New(
		model.WithModelPath("fake"),
		model.WithQueueDepth(depth),
		model.WithAdmissionTimeout(time.Minute),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	m.config.QueueDepth = depth
	m.config.AdmissionTimeout = time.Minute

	return m
}

func testParams() model.GenerateParams {
	params := model.NewGenerateParams()
	params.Prompt = "test"
	params.Width = 64
	params.Height = 64
	params.Steps = 1

	return params
}

func TestGenerateSerializesAndUnload(t *testing.T) {
	fb := fakeBackend{
		entered: make(chan struct{}, 2),
		release: make(chan struct{}, 2),
	}
	m := newTestMalina(t, &fb, 2)

	results := make(chan error, 2)
	go func() {
		_, err := m.Generate(t.Context(), testParams())
		results <- err
	}()
	<-fb.entered

	go func() {
		_, err := m.Generate(t.Context(), testParams())
		results <- err
	}()
	select {
	case <-fb.entered:
		t.Fatal("second generation entered concurrently")
	default:
	}

	fb.release <- struct{}{}
	<-fb.entered
	fb.release <- struct{}{}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
	}

	if err := m.Unload(t.Context()); err != nil {
		t.Fatalf("Unload() error = %v", err)
	}
	if err := m.Unload(t.Context()); err != nil {
		t.Fatalf("second Unload() error = %v", err)
	}

	fb.mu.Lock()
	defer fb.mu.Unlock()
	if fb.max != 1 || fb.unloads != 1 {
		t.Errorf("max/unloads: got %d/%d, want 1/1", fb.max, fb.unloads)
	}
}

func TestQueuedCancellationDoesNotGenerate(t *testing.T) {
	fb := fakeBackend{
		entered: make(chan struct{}, 2),
		release: make(chan struct{}, 1),
	}
	m := newTestMalina(t, &fb, 2)

	first := make(chan error, 1)
	go func() {
		_, err := m.Generate(t.Context(), testParams())
		first <- err
	}()
	<-fb.entered

	ctx, cancel := context.WithCancel(t.Context())
	second := make(chan error, 1)
	go func() {
		_, err := m.Generate(ctx, testParams())
		second <- err
	}()
	for m.ActiveGenerations() != 2 {
		runtime.Gosched()
	}
	cancel()
	if err := <-second; !errors.Is(err, context.Canceled) {
		t.Fatalf("Generate() error = %v, want context.Canceled", err)
	}

	fb.release <- struct{}{}
	if err := <-first; err != nil {
		t.Fatalf("first Generate() error = %v", err)
	}
	select {
	case <-fb.entered:
		t.Fatal("canceled generation entered backend")
	default:
	}

	if err := m.Unload(t.Context()); err != nil {
		t.Fatalf("Unload() error = %v", err)
	}
}

func TestCancellationWaitsForNativeCompletion(t *testing.T) {
	fb := fakeBackend{
		entered: make(chan struct{}, 1),
		release: make(chan struct{}, 1),
	}
	m := newTestMalina(t, &fb, 1)

	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, err := m.Generate(ctx, testParams())
		result <- err
	}()
	<-fb.entered
	cancel()

	select {
	case err := <-result:
		t.Fatalf("Generate returned before native completion: %v", err)
	default:
	}

	fb.release <- struct{}{}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("Generate() error = %v, want context.Canceled", err)
	}
	if err := m.Unload(t.Context()); err != nil {
		t.Fatalf("Unload() error = %v", err)
	}
}

func TestNativeFailurePoisonsHandle(t *testing.T) {
	nativeErr := errors.New("native failure")
	fb := fakeBackend{
		entered: make(chan struct{}, 1),
		release: make(chan struct{}, 1),
		err:     errors.Join(model.ErrNativeGeneration, nativeErr),
	}
	m := newTestMalina(t, &fb, 1)

	result := make(chan error, 1)
	go func() {
		_, err := m.Generate(t.Context(), testParams())
		result <- err
	}()
	<-fb.entered
	fb.release <- struct{}{}

	if err := <-result; !errors.Is(err, ErrPoisoned) || !errors.Is(err, nativeErr) {
		t.Fatalf("Generate() error = %v, want poisoned native error", err)
	}
	if m.Ready() {
		t.Fatal("Ready() = true, want false")
	}
	if err := m.Unload(t.Context()); err != nil {
		t.Fatalf("Unload() error = %v", err)
	}
}

func TestInitRejectsConflictingPath(t *testing.T) {
	initState.Lock()
	oldDone := initState.done
	oldPath := initState.path
	initState.done = true
	initState.path = "/first"
	initState.Unlock()

	t.Cleanup(func() {
		initState.Lock()
		initState.done = oldDone
		initState.path = oldPath
		initState.Unlock()
	})

	if err := Init(WithLibPath("/first")); err != nil {
		t.Fatalf("Init() same path error = %v", err)
	}
	if err := Init(WithLibPath("/second")); err == nil {
		t.Fatal("Init() conflicting path error = nil, want error")
	}
}
