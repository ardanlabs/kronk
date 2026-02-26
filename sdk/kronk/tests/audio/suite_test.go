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
		id := uuid.New().String()

		const maxRetries = 3
		for attempt := 1; attempt <= maxRetries; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)

			now := time.Now()
			resp, err := krn.Chat(ctx, testlib.DAudio)
			done := time.Now()
			cancel()

			t.Logf("%s: %s, st: %v, en: %v, Duration: %s (attempt %d/%d)", id, krn.ModelInfo().ID, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now), attempt, maxRetries)

			if err != nil {
				if attempt < maxRetries {
					t.Logf("%s: retrying after error: %v", id, err)
					time.Sleep(250 * time.Millisecond)
					continue
				}
				return fmt.Errorf("chat: %w", err)
			}

			result := testlib.TestChatResponse(resp, krn.ModelInfo().ID, model.ObjectChatMedia, "speech", "", "", false, false)

			for _, w := range result.Warnings {
				t.Logf("WARNING: %s", w)
			}

			if result.Err != nil {
				if attempt < maxRetries {
					t.Logf("%s: retrying after empty content", id)
					continue
				}
				t.Logf("%#v", resp)
				return result.Err
			}

			return nil
		}

		return nil
	}

	var g errgroup.Group
	for range testlib.Goroutines {
		g.Go(f)
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
		id := uuid.New().String()

		const maxRetries = 2
		for attempt := 1; attempt <= maxRetries; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)

			now := time.Now()
			ch, err := krn.ChatStreaming(ctx, testlib.DAudio)
			if err != nil {
				cancel()
				if attempt < maxRetries {
					t.Logf("%s: retrying after error: %v", id, err)
					continue
				}
				return fmt.Errorf("chat streaming: %w", err)
			}

			var acc testlib.StreamAccumulator
			var lastResp model.ChatResponse
			var basicErr error
			for resp := range ch {
				acc.Accumulate(resp)
				lastResp = resp

				if err := testlib.TestChatBasics(resp, krn.ModelInfo().ID, model.ObjectChatMedia, false, true); err != nil {
					t.Logf("%#v", resp)
					basicErr = err
					break
				}
			}

			done := time.Now()
			cancel()

			t.Logf("%s: %s, st: %v, en: %v, Duration: %s (attempt %d/%d)", id, krn.ModelInfo().ID, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now), attempt, maxRetries)

			if basicErr != nil {
				if attempt < maxRetries {
					t.Logf("%s: retrying after basics error: %v", id, basicErr)
					continue
				}
				return fmt.Errorf("basics: %w", basicErr)
			}

			result := testlib.TestStreamingContent(&acc, lastResp, "speech")

			for _, w := range result.Warnings {
				t.Logf("WARNING: %s", w)
			}

			if result.Err != nil {
				if attempt < maxRetries {
					t.Logf("%s: retrying after empty content", id)
					continue
				}
				t.Logf("accumulated content: %q", acc.Content.String())
				t.Logf("%#v", lastResp)
				return result.Err
			}

			return nil
		}

		return nil
	}

	var g errgroup.Group
	for range testlib.Goroutines {
		g.Go(f)
	}

	if err := g.Wait(); err != nil {
		t.Errorf("error: %v", err)
	}
}
