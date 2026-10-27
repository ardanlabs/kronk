//go:build malina_integration

package malina_test

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"strconv"
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
	mu          sync.Mutex
	enabled     bool
	started     map[int]bool
	finished    map[int]bool
	overlaps    bool
	cancelSteps int
	cancel      context.CancelFunc
}

func (po *progressOverlap) observe(step int, steps int, _ float32) {
	po.mu.Lock()
	defer po.mu.Unlock()

	if !po.enabled || step <= 0 {
		return
	}
	if steps == po.cancelSteps && po.cancel != nil {
		po.cancel()
		po.cancel = nil
	}
	if steps != firstSteps && steps != secondSteps {
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

func (po *progressOverlap) cancelAt(steps int, cancel context.CancelFunc) {
	po.mu.Lock()
	defer po.mu.Unlock()

	po.cancelSteps = steps
	po.cancel = cancel
}

func TestMalinaModelInference(t *testing.T) {
	mdls, err := malinamodels.New()
	if err != nil {
		t.Fatalf("models.New() error = %v", err)
	}
	mp, err := mdls.FullPath(malinamodels.BundleSD15.String())
	if err != nil {
		t.Skipf("%s bundle is not installed: %v", malinamodels.BundleSD15, err)
	}
	// No runner pulls the malina bundles, so an indexed-but-empty bundle
	// means the model is not on this machine, not that anything is broken.
	if len(mp.ModelFiles) == 0 {
		t.Skipf("%s bundle contains no model files", malinamodels.BundleSD15)
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

	const cancelSteps = 100
	cancelCtx, cancel := context.WithCancel(t.Context())
	progress.cancelAt(cancelSteps, cancel)
	canceled := params
	canceled.Steps = cancelSteps
	if _, err := handle.Generate(cancelCtx, canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Generate() error = %v, want context.Canceled", err)
	}

	reuse := params
	reuse.Steps = 1
	reuse.Seed = -1
	generated, err := handle.Generate(t.Context(), reuse)
	if err != nil {
		t.Fatalf("Generate() after cancellation error = %v", err)
	}
	if err := handle.Unload(t.Context()); err != nil {
		t.Fatalf("Unload() error = %v", err)
	}

	var info struct {
		Schema string `json:"schema"`
		Seed   string `json:"seed"`
	}
	if err := json.Unmarshal([]byte(generated.GenerationInfo), &info); err != nil {
		t.Fatalf("generation metadata after unload: %v", err)
	}
	if info.Schema != "sdcpp.generation/v1" || generated.Seed != -1 {
		t.Errorf("metadata: got schema %q, requested seed %d, want sdcpp.generation/v1 and -1", info.Schema, generated.Seed)
	}
	if _, err := strconv.ParseUint(info.Seed, 10, 64); err != nil {
		t.Errorf("resolved seed: got %q, want a nonnegative native seed", info.Seed)
	}
}

func TestMalinaUpscaleMetadata(t *testing.T) {
	mdls, err := malinamodels.New()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := mdls.LoadManifest(malinamodels.BundleRealESRGANX4Anime)
	if err != nil {
		t.Skipf("upscaler bundle is not installed: %v", err)
	}
	if err := malina.Init(malina.WithLibPath(malinalibs.Path(""))); err != nil {
		t.Fatal(err)
	}
	u, err := malina.NewUpscaler(t.Context(), malina.UpscalerConfig{
		ModelPath: manifest.Files[malinamodels.RoleUpscaler.String()],
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := u.Unload(); err != nil {
			t.Error(err)
		}
	})

	input := image.NewRGBA(image.Rect(0, 0, 8, 16))
	for y := range 16 {
		for x := range 8 {
			input.SetRGBA(x, y, color.RGBA{R: uint8(x * 30), G: uint8(y * 15), B: 90, A: 255})
		}
	}
	results, err := u.UpscaleWithInfo(t.Context(), input)
	if err != nil {
		t.Fatalf("UpscaleWithInfo(): %v", err)
	}
	legacy, err := u.Upscale(t.Context(), input)
	if err != nil {
		t.Fatalf("Upscale(): %v", err)
	}
	if err := u.Unload(); err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || len(legacy) != 1 {
		t.Fatalf("result counts: got %d/%d, want 1/1", len(results), len(legacy))
	}
	wantBounds := image.Rect(0, 0, 32, 64)
	if results[0].Image.Bounds() != wantBounds || legacy[0].Bounds() != wantBounds {
		t.Fatalf("bounds: got %v/%v, want %v", results[0].Image.Bounds(), legacy[0].Bounds(), wantBounds)
	}
	for y := range 64 {
		for x := range 32 {
			if results[0].Image.At(x, y) != legacy[0].At(x, y) {
				t.Fatalf("pixel (%d,%d): got %v, want legacy %v", x, y, results[0].Image.At(x, y), legacy[0].At(x, y))
			}
		}
	}
	var info struct {
		Schema  string `json:"schema"`
		Upscale struct {
			Factor int `json:"upscale_factor"`
		} `json:"upscale"`
	}
	if err := json.Unmarshal([]byte(results[0].GenerationInfo), &info); err != nil {
		t.Fatalf("metadata after unload: %v", err)
	}
	if info.Schema != "sdcpp.generation/v1" || info.Upscale.Factor != 4 {
		t.Errorf("metadata: got %+v, want sdcpp.generation/v1 and factor 4", info)
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
