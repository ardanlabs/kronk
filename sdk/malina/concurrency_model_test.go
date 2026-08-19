//go:build malina_model_tests

package malina_test

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/malina"
	"github.com/ardanlabs/kronk/sdk/malina/model"
	malinalibs "github.com/ardanlabs/kronk/sdk/tools/malina/libs"
	malinamodels "github.com/ardanlabs/kronk/sdk/tools/malina/models"
)

const (
	firstSteps  = 10
	secondSteps = 11
)

type progressOverlap struct {
	mu       sync.Mutex
	enabled  bool
	started  map[int]bool
	finished map[int]bool
	overlaps bool
}

func (po *progressOverlap) observe(step int, steps int, _ float32) {
	po.mu.Lock()
	defer po.mu.Unlock()

	if !po.enabled || (steps != firstSteps && steps != secondSteps) || step <= 0 {
		return
	}

	po.started[steps] = true
	if po.started[firstSteps] && po.started[secondSteps] && !po.finished[firstSteps] && !po.finished[secondSteps] {
		po.overlaps = true
	}
	if step >= steps {
		po.finished[steps] = true
	}
}

func (po *progressOverlap) enable() {
	po.mu.Lock()
	defer po.mu.Unlock()

	po.enabled = true
	po.started = make(map[int]bool)
	po.finished = make(map[int]bool)
	po.overlaps = false
}

func (po *progressOverlap) overlapped() bool {
	po.mu.Lock()
	defer po.mu.Unlock()

	return po.overlaps
}

func TestHandleGeneratesConcurrently(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("real-model Malina tests do not run in GitHub Actions")
	}

	mdls, err := malinamodels.New()
	if err != nil {
		t.Fatalf("models.New() error = %v", err)
	}
	mp, err := mdls.FullPath(malinamodels.BundleSD15.String())
	if err != nil {
		t.Skipf("%s bundle is not installed: %v", malinamodels.BundleSD15, err)
	}
	if len(mp.ModelFiles) == 0 {
		t.Fatal("sd-1.5 bundle contains no model files")
	}

	var progress progressOverlap
	if err := malina.Init(
		malina.WithLibPath(malinalibs.Path("")),
		malina.WithProgress(progress.observe),
	); err != nil {
		t.Fatalf("malina.Init() error = %v", err)
	}

	handle := newTestHandle(t, mp.ModelFiles[0])
	warmPool(t, handle)
	progress.enable()

	params := model.NewGenerateParams()
	params.Prompt = "a red sailboat on a calm lake"
	params.Width = 512
	params.Height = 512
	params.Seed = 42

	start := make(chan struct{})
	results := make(chan error, 2)
	for i, test := range []struct {
		steps int
	}{
		{steps: firstSteps},
		{steps: secondSteps},
	} {
		go func() {
			request := params
			request.Steps = test.steps
			request.Seed += int64(i)
			<-start
			_, err := handle.Generate(t.Context(), request)
			results <- err
		}()
	}
	close(start)

	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
	}
	if !progress.overlapped() {
		t.Fatal("pooled native generation progress did not overlap")
	}
}

func newTestHandle(t *testing.T, modelPath string) *malina.Malina {
	t.Helper()

	handle, err := malina.New(
		model.WithModelPath(modelPath),
		model.WithConcurrency(2),
		model.WithQueueDepth(1),
	)
	if err != nil {
		t.Fatalf("malina.New() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := handle.Unload(ctx); err != nil {
			t.Errorf("Unload() error = %v", err)
		}
	})

	return handle
}

func warmPool(t *testing.T, handle *malina.Malina) {
	t.Helper()

	params := model.NewGenerateParams()
	params.Prompt = "warmup"
	params.Width = 64
	params.Height = 64
	params.Steps = 1

	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, err := handle.Generate(t.Context(), params)
			results <- err
		}()
	}
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("warm Generate() error = %v", err)
		}
	}
}
