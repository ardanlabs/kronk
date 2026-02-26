package qwen3_test

import (
	"context"
	"fmt"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
	"github.com/google/uuid"
)

func Test_ConTest1(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	id := uuid.New().String()
	now := time.Now()
	defer func() {
		name := strings.TrimSuffix(testlib.MPThinkToolChat.ModelFiles[0], path.Ext(testlib.MPThinkToolChat.ModelFiles[0]))
		done := time.Now()
		t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, name, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
	}()

	krn, d := testlib.InitChatTest(t, testlib.MPThinkToolChat, false)
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
		if resp.Choice[0].FinishReason() == model.FinishReasonError {
			lastResp = resp
		}
	}

	t.Log("check conditions")

	if len(lastResp.Choice) == 0 {
		t.Log("WARNING: Didn't get any response from the api call, but channel is closed")
		return
	}

	if v := lastResp.Choice[0].FinishReason(); v != model.FinishReasonError {
		t.Errorf("expected error finish reason, got %s", v)
	}

	if lastResp.Choice[0].Delta == nil || lastResp.Choice[0].Delta.Content != "context canceled" {
		errContent := ""
		if lastResp.Choice[0].Delta != nil {
			errContent = lastResp.Choice[0].Delta.Content
		}
		t.Errorf("expected error context canceled, got %s", errContent)
	}
}

func Test_ConTest2(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	id := uuid.New().String()
	now := time.Now()
	defer func() {
		name := strings.TrimSuffix(testlib.MPThinkToolChat.ModelFiles[0], path.Ext(testlib.MPThinkToolChat.ModelFiles[0]))
		done := time.Now()
		t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, name, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
	}()

	krn, d := testlib.InitChatTest(t, testlib.MPThinkToolChat, false)
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
		if resp.Choice[0].FinishReason() == model.FinishReasonError {
			lastResp = resp
		}

		index++
		if index == 2 {
			t.Log("cancel context inside channel loop")
			cancel()
		}
	}

	t.Log("check conditions")

	if len(lastResp.Choice) == 0 {
		t.Log("WARNING: Didn't get any response from the api call, but channel is closed")
		return
	}

	if v := lastResp.Choice[0].FinishReason(); v != model.FinishReasonError {
		t.Errorf("expected error finish reason, got %s", v)
	}

	if lastResp.Choice[0].Delta == nil || lastResp.Choice[0].Delta.Content != "context canceled" {
		errContent := ""
		if lastResp.Choice[0].Delta != nil {
			errContent = lastResp.Choice[0].Delta.Content
		}
		t.Errorf("expected error context canceled, got %s", errContent)
	}

	if t.Failed() {
		fmt.Printf("%#v\n", lastResp)
	}
}

func Test_ConTest3(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	id := uuid.New().String()
	now := time.Now()
	defer func() {
		name := strings.TrimSuffix(testlib.MPThinkToolChat.ModelFiles[0], path.Ext(testlib.MPThinkToolChat.ModelFiles[0]))
		done := time.Now()
		t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, name, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
	}()

	krn, d := testlib.InitChatTest(t, testlib.MPThinkToolChat, false)
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
		if index == 2 {
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

	t.Log("flush channel until it closes")
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for flushing := true; flushing; {
		select {
		case _, open := <-ch:
			if !open {
				flushing = false
			}
		case <-timer.C:
			t.Fatal("timed out waiting for channel to close")
		}
	}
}
