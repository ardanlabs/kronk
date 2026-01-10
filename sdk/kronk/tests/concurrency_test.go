package kronk_test

import (
	"context"
	"fmt"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/google/uuid"
)

// initChatTest creates a new Kronk instance for tests that need their own
// model lifecycle (e.g., concurrency tests that test unload behavior).
func initChatTest(t *testing.T, mp models.Path, tooling bool) (*kronk.Kronk, model.D) {
	krn, err := kronk.New(modelInstances, model.Config{
		ModelFiles: mp.ModelFiles,
	})

	if err != nil {
		t.Fatalf("unable to load model: %v: %v", mp.ModelFiles, err)
	}

	question := "Echo back the word: Gorilla"
	if tooling {
		question = "What is the weather in London, England?"
	}

	d := model.D{
		"messages": []model.D{
			{
				"role":    "user",
				"content": question,
			},
		},
		"max_tokens": 2048,
	}

	if tooling {
		switch krn.ModelInfo().IsGPTModel {
		case true:
			d["tools"] = []model.D{
				{
					"type": "function",
					"function": model.D{
						"name":        "get_weather",
						"description": "Get the current weather for a location",
						"parameters": model.D{
							"type": "object",
							"properties": model.D{
								"location": model.D{
									"type":        "string",
									"description": "The location to get the weather for, e.g. San Francisco, CA",
								},
							},
							"required": []any{"location"},
						},
					},
				},
			}

		default:
			d["tools"] = []model.D{
				{
					"type": "function",
					"function": model.D{
						"name":        "get_weather",
						"description": "Get the current weather for a location",
						"arguments": model.D{
							"location": model.D{
								"type":        "string",
								"description": "The location to get the weather for, e.g. San Francisco, CA",
							},
						},
					},
				},
			}
		}
	}

	return krn, d
}

func Test_ConTest1(t *testing.T) {
	// This test cancels the context before the channel loop starts.

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	id := uuid.New().String()
	now := time.Now()
	defer func() {
		name := strings.TrimSuffix(mpThinkToolChat.ModelFiles[0], path.Ext(mpThinkToolChat.ModelFiles[0]))
		done := time.Now()
		t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, name, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
	}()

	krn, d := initChatTest(t, mpThinkToolChat, false)
	defer func() {
		t.Logf("active streams: %d", krn.ActiveStreams())
		t.Log("unload Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			t.Errorf("should not receive an error unloading Kronk: %s", err)
		}
	}()

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		t.Fatalf("should not receive an error starting chat streaming: %s", err)
	}

	t.Log("start processing stream")
	defer t.Log("end processing stream")

	t.Logf("active streams: %d", krn.ActiveStreams())

	t.Log("cancel context before channel loop")
	cancel()

	var lastResp model.ChatResponse
	for resp := range ch {
		lastResp = resp
	}

	t.Log("check conditions")

	if lastResp.Choice[0].FinishReason != model.FinishReasonError {
		t.Errorf("expected error finish reason, got %s", lastResp.Choice[0].FinishReason)
	}

	if lastResp.Choice[0].Delta.Content != "context canceled" {
		t.Errorf("expected error context canceled, got %s", lastResp.Choice[0].Delta.Content)
	}
}

func Test_ConTest2(t *testing.T) {
	// This test cancels the context inside the channel loop.

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	id := uuid.New().String()
	now := time.Now()
	defer func() {
		name := strings.TrimSuffix(mpThinkToolChat.ModelFiles[0], path.Ext(mpThinkToolChat.ModelFiles[0]))
		done := time.Now()
		t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, name, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
	}()

	krn, d := initChatTest(t, mpThinkToolChat, false)
	defer func() {
		t.Logf("active streams: %d", krn.ActiveStreams())
		t.Log("unload Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			t.Errorf("should not receive an error unloading Kronk: %s", err)
		}
	}()

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		t.Fatalf("should not receive an error starting chat streaming: %s", err)
	}

	t.Log("start processing stream")
	defer t.Log("end processing stream")

	t.Logf("active streams: %d", krn.ActiveStreams())

	var lastResp model.ChatResponse
	var index int
	for resp := range ch {
		lastResp = resp
		index++
		if index == 5 {
			t.Log("cancel context inside channel loop")
			cancel()
		}
	}

	t.Log("check conditions")

	if lastResp.Choice[0].FinishReason != model.FinishReasonError {
		t.Errorf("expected error finish reason, got %s", lastResp.Choice[0].FinishReason)
	}

	if lastResp.Choice[0].Delta.Content != "context canceled" {
		t.Errorf("expected error context canceled, got %s", lastResp.Choice[0].Delta.Content)
	}

	if t.Failed() {
		fmt.Printf("%#v\n", lastResp)
	}
}

func Test_ConTest3(t *testing.T) {
	// This test breaks out the channel loop before the context is canceled.
	// Then the context is cancelled and checks the system shuts down properly.

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	id := uuid.New().String()
	now := time.Now()
	defer func() {
		name := strings.TrimSuffix(mpThinkToolChat.ModelFiles[0], path.Ext(mpThinkToolChat.ModelFiles[0]))
		done := time.Now()
		t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, name, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
	}()

	krn, d := initChatTest(t, mpThinkToolChat, false)
	defer func() {
		t.Logf("active streams: %d", krn.ActiveStreams())
		t.Log("unload Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			t.Errorf("should not receive an error unloading Kronk: %s", err)
		}
	}()

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		t.Fatalf("should not receive an error starting chat streaming: %s", err)
	}

	t.Log("start processing stream")
	defer t.Log("end processing stream")

	t.Logf("active streams: %d", krn.ActiveStreams())

	var index int
	for range ch {
		index++
		if index == 5 {
			break
		}
	}

	t.Log("attempt to unload Knonk, should get error")

	shortCtx, shortCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer shortCancel()

	if err := krn.Unload(shortCtx); err == nil {
		t.Errorf("should receive an error unloading Kronk: %s", err)
	}

	t.Log("cancel context after breaking channel loop")
	cancel()

	t.Log("check if the channel is closed")
	var closed bool
	for range 3 {
		_, open := <-ch
		if !open {
			closed = true
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	t.Log("check conditions")

	if !closed {
		t.Errorf("expected channel to be closed")
	}
}
