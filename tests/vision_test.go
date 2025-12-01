package kronk_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/model"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

func Test_SimpleVision(t *testing.T) {
	testVision(t, modelSimpleVisionFile, projSimpleVisionFile, imageFile)
}

func Test_SimpleStreamingVision(t *testing.T) {
	testVisionStreaming(t, modelSimpleVisionFile, projSimpleVisionFile, imageFile)
}

// =============================================================================

func initVisionTest(t *testing.T, modelFile string, projFile string, imageFile string) (*kronk.Kronk, model.Params, model.D, []byte) {
	if _, err := os.Stat(imageFile); err != nil {
		t.Fatalf("error accessing file %q: %s", imageFile, err)
	}

	image, err := os.ReadFile(imageFile)
	if err != nil {
		t.Fatalf("error reading file %q: %s", imageFile, err)
	}

	// -------------------------------------------------------------------------

	krn, err := kronk.New(modelInstances, model.Config{
		ModelFile:      modelFile,
		ProjectionFile: projFile,
	})

	if err != nil {
		t.Fatalf("unable to load model: %s: %v", modelFile, err)
	}

	// -------------------------------------------------------------------------

	d := model.D{
		"messages": []model.D{
			{
				"role":    "user",
				"content": "What is in this picture?",
			},
		},
	}

	params := model.Params{
		MaxTokens: 4096,
	}

	return krn, params, d, image
}

func testVision(t *testing.T, modelFile string, projFile string, imageFile string) {
	if runInParallel {
		t.Parallel()
	}

	krn, params, d, image := initVisionTest(t, modelFile, projFile, imageFile)
	defer func() {
		t.Logf("active streams: %d", krn.ActiveStreams())
		t.Log("unload Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			t.Errorf("failed to unload model: %v", err)
		}
	}()

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		id := uuid.New().String()
		now := time.Now()
		defer func() {
			done := time.Now()
			t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, krn.ModelInfo().Name, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
		}()

		resp, err := krn.Vision(ctx, image, params, d)
		if err != nil {
			return fmt.Errorf("vision streaming: %w", err)
		}

		if err := testVisionResponse(resp, krn.ModelInfo().Name, "vision", "giraffes"); err != nil {
			t.Logf("%#v", resp)
			return err
		}

		return nil
	}

	var g errgroup.Group
	for range goroutines {
		g.Go(f)
	}

	if err := g.Wait(); err != nil {
		t.Errorf("error: %v", err)
	}
}

func testVisionStreaming(t *testing.T, modelFile string, projFile string, imageFile string) {
	if runInParallel {
		t.Parallel()
	}

	krn, params, d, image := initVisionTest(t, modelFile, projFile, imageFile)
	defer func() {
		t.Logf("active streams: %d", krn.ActiveStreams())
		t.Log("unload Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			t.Errorf("failed to unload model: %v", err)
		}
	}()

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		id := uuid.New().String()
		now := time.Now()
		defer func() {
			done := time.Now()
			t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, krn.ModelInfo().Name, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
		}()

		ch, err := krn.VisionStreaming(ctx, image, params, d)
		if err != nil {
			return fmt.Errorf("vision streaming: %w", err)
		}

		var lastResp model.ChatResponse
		for resp := range ch {
			lastResp = resp

			if err := testChatBasics(resp, krn.ModelInfo().Name, model.ObjectVision, false); err != nil {
				t.Logf("%#v", resp)
				return err
			}
		}

		if err := testVisionResponse(lastResp, krn.ModelInfo().Name, model.ObjectVision, "giraffes"); err != nil {
			t.Logf("%#v", lastResp)
			return err
		}

		return nil
	}

	var g errgroup.Group
	for range goroutines {
		g.Go(f)
	}

	if err := g.Wait(); err != nil {
		t.Errorf("error: %v", err)
	}
}
