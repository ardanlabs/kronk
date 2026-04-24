package audio_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

func TestSuite(t *testing.T) {
	testlib.WithModel(t, testlib.CfgAudio(), func(t *testing.T, krn *kronk.Kronk) {
		t.Run("AudioChat", func(t *testing.T) { testAudio(t, krn) })
		t.Run("AudioStreamingChat", func(t *testing.T) { testAudioStreaming(t, krn) })
	})
}

func testAudio(t *testing.T, krn *kronk.Kronk) {
	if testlib.RunInParallel {
		t.Parallel()
	}

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
		defer cancel()

		id := uuid.New().String()
		now := time.Now()
		resp, err := krn.Chat(ctx, testlib.DAudio)
		done := time.Now()

		t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, krn.ModelInfo().ID, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))

		if err != nil {
			return fmt.Errorf("chat: %w", err)
		}

		result := testlib.TestChatResponse(resp, krn.ModelInfo().ID, model.ObjectChatMedia, "speech", "", "", false, false)

		for _, w := range result.Warnings {
			t.Logf("WARNING: %s", w)
		}

		if result.Err != nil {
			return result.Err
		}

		return nil
	}

	var g errgroup.Group
	for range testlib.Goroutines {
		g.Go(testlib.WithRetry(t, f))
	}

	if err := g.Wait(); err != nil {
		t.Errorf("error: %v", err)
	}
}

func testAudioStreaming(t *testing.T, krn *kronk.Kronk) {
	if testlib.RunInParallel {
		t.Parallel()
	}

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
		defer cancel()

		id := uuid.New().String()
		now := time.Now()
		ch, err := krn.ChatStreaming(ctx, testlib.DAudio)
		if err != nil {
			return fmt.Errorf("chat streaming: %w", err)
		}

		var acc testlib.StreamAccumulator
		var lastResp model.ChatResponse
		for resp := range ch {
			acc.Accumulate(resp)
			lastResp = resp

			if err := testlib.TestChatBasics(resp, krn.ModelInfo().ID, model.ObjectChatMedia, false, true); err != nil {
				return fmt.Errorf("basics: %w", err)
			}
		}

		done := time.Now()

		t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, krn.ModelInfo().ID, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))

		result := testlib.TestStreamingContent(&acc, lastResp, "speech")

		for _, w := range result.Warnings {
			t.Logf("WARNING: %s", w)
		}

		if result.Err != nil {
			return result.Err
		}

		return nil
	}

	var g errgroup.Group
	for range testlib.Goroutines {
		g.Go(testlib.WithRetry(t, f))
	}

	if err := g.Wait(); err != nil {
		t.Errorf("error: %v", err)
	}
}
