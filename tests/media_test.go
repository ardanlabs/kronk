package kronk_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/model"
	"github.com/ardanlabs/kronk/tools"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

func Test_SimpleMedia(t *testing.T) {
	testMedia(t, mpSimpleVision, imageFile)
}

func Test_SimpleMediaStreaming(t *testing.T) {
	testMediaStreaming(t, mpSimpleVision, imageFile)
}

// =============================================================================

func testMedia(t *testing.T, mp tools.ModelPath, imageFile string) {
	if runInParallel {
		t.Parallel()
	}

	krn, d := initMediaTest(t, mp, imageFile)
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
			t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, krn.ModelInfo().ID, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
		}()

		resp, err := krn.Chat(ctx, d)
		if err != nil {
			return fmt.Errorf("chat streaming: %w", err)
		}

		if err := testChatResponse(resp, krn.ModelInfo().ID, model.ObjectChatMedia, "giraffes", "", ""); err != nil {
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

func testMediaStreaming(t *testing.T, mp tools.ModelPath, imageFile string) {
	if runInParallel {
		t.Parallel()
	}

	krn, d := initMediaTest(t, mp, imageFile)
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
			t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, krn.ModelInfo().ID, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
		}()

		ch, err := krn.ChatStreaming(ctx, d)
		if err != nil {
			return fmt.Errorf("chat streaming: %w", err)
		}

		var lastResp model.ChatResponse
		for resp := range ch {
			lastResp = resp

			if err := testChatBasics(resp, krn.ModelInfo().ID, model.ObjectChatMedia, false); err != nil {
				t.Logf("%#v", resp)
				return err
			}
		}

		if err := testChatResponse(lastResp, krn.ModelInfo().ID, model.ObjectChatMedia, "giraffes", "", ""); err != nil {
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

func initMediaTest(t *testing.T, mp tools.ModelPath, mediaFile string) (*kronk.Kronk, model.D) {
	if _, err := os.Stat(mediaFile); err != nil {
		t.Fatalf("error accessing file %q: %s", mediaFile, err)
	}

	media, err := os.ReadFile(mediaFile)
	if err != nil {
		t.Fatalf("error reading file %q: %s", mediaFile, err)
	}

	// -------------------------------------------------------------------------

	krn, err := kronk.New(modelInstances, model.Config{
		ModelFile:      mp.ModelFile,
		ProjectionFile: mp.ProjFile,
	})

	if err != nil {
		t.Fatalf("unable to load model: %s: %v", mp.ModelFile, err)
	}

	// -------------------------------------------------------------------------

	d := model.D{
		"messages":   model.MediaMessage("What is in this picture?", media),
		"max_tokens": 2048,
	}

	return krn, d
}
