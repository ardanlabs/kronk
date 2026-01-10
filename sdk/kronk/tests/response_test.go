package kronk_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

func Test_ThinkResponse(t *testing.T) {
	testResponse(t, mpThinkToolChat, false)
}

func Test_ThinkStreamingResponse(t *testing.T) {
	testResponseStreaming(t, mpThinkToolChat, false)
}

func Test_ToolResponse(t *testing.T) {
	testResponse(t, mpThinkToolChat, true)
}

func Test_ToolStreamingResponse(t *testing.T) {
	testResponseStreaming(t, mpThinkToolChat, true)
}

// =============================================================================

func testResponse(t *testing.T, mp models.Path, tooling bool) {
	if runInParallel {
		t.Parallel()
	}

	krn, d := initResponseTest(t, mp, tooling)
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

		resp, err := krn.Response(ctx, d)
		if err != nil {
			return fmt.Errorf("response: %w", err)
		}

		if tooling {
			if err := testResponseResponse(resp, krn.ModelInfo().ID, "London", "get_weather", "location"); err != nil {
				t.Logf("%#v", resp)
				return err
			}
			return nil
		}

		if err := testResponseResponse(resp, krn.ModelInfo().ID, "Gorilla", "", ""); err != nil {
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

func testResponseStreaming(t *testing.T, mp models.Path, tooling bool) {
	if runInParallel {
		t.Parallel()
	}

	krn, d := initResponseTest(t, mp, tooling)
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

		ch, err := krn.ResponseStreaming(ctx, d)
		if err != nil {
			return fmt.Errorf("response streaming: %w", err)
		}

		var finalResp *kronk.ResponseResponse
		var hasTextDelta bool
		var hasReasoningDelta bool
		var hasFunctionCallDone bool

		for event := range ch {
			switch event.Type {
			case "response.created":
				if event.Response == nil {
					return fmt.Errorf("response.created: expected response")
				}
				if event.Response.Status != "in_progress" {
					return fmt.Errorf("response.created: expected status in_progress, got %s", event.Response.Status)
				}

			case "response.reasoning_summary_text.delta":
				if event.Delta == "" {
					return fmt.Errorf("response.reasoning_summary_text.delta: expected delta")
				}
				hasReasoningDelta = true

			case "response.output_text.delta":
				if event.Delta == "" {
					return fmt.Errorf("response.output_text.delta: expected delta")
				}
				hasTextDelta = true

			case "response.function_call_arguments.done":
				if event.Name == "" {
					return fmt.Errorf("response.function_call_arguments.done: expected name")
				}
				if event.Arguments == "" {
					return fmt.Errorf("response.function_call_arguments.done: expected arguments")
				}
				hasFunctionCallDone = true

			case "response.completed":
				if event.Response == nil {
					return fmt.Errorf("response.completed: expected response")
				}
				if event.Response.Status != "completed" {
					return fmt.Errorf("response.completed: expected status completed, got %s", event.Response.Status)
				}
				finalResp = event.Response
			}
		}

		if finalResp == nil {
			return fmt.Errorf("expected response.completed event")
		}

		if tooling {
			if !hasFunctionCallDone {
				return fmt.Errorf("expected function_call_arguments.done event for tooling")
			}
			if err := testResponseResponse(*finalResp, krn.ModelInfo().ID, "London", "get_weather", "location"); err != nil {
				t.Logf("%#v", finalResp)
				return err
			}
			return nil
		}

		if !hasTextDelta {
			return fmt.Errorf("expected output_text.delta events")
		}

		if !hasReasoningDelta {
			return fmt.Errorf("expected reasoning_summary_text.delta events")
		}

		if err := testResponseResponse(*finalResp, krn.ModelInfo().ID, "Gorilla", "", ""); err != nil {
			t.Logf("%#v", finalResp)
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

func initResponseTest(t *testing.T, mp models.Path, tooling bool) (*kronk.Kronk, model.D) {
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

	return krn, d
}

func testResponseBasics(resp kronk.ResponseResponse, modelName string) error {
	if resp.ID == "" {
		return fmt.Errorf("expected id")
	}

	if resp.Object != "response" {
		return fmt.Errorf("expected object type to be response, got %s", resp.Object)
	}

	if resp.CreatedAt == 0 {
		return fmt.Errorf("expected created time")
	}

	if resp.Model != modelName {
		return fmt.Errorf("basics: expected model to be %s, got %s", modelName, resp.Model)
	}

	if resp.Status != "completed" {
		return fmt.Errorf("basics: expected status to be completed, got %s", resp.Status)
	}

	if len(resp.Output) == 0 {
		return fmt.Errorf("basics: expected output, got %d", len(resp.Output))
	}

	return nil
}

func testResponseResponse(resp kronk.ResponseResponse, modelName string, find string, funct string, arg string) error {
	if err := testResponseBasics(resp, modelName); err != nil {
		return err
	}

	find = strings.ToLower(find)
	funct = strings.ToLower(funct)

	if funct != "" {
		for _, output := range resp.Output {
			if output.Type == "function_call" {
				name := strings.ToLower(output.Name)
				if !strings.Contains(name, funct) {
					return fmt.Errorf("tooling: expected function name %q, got %q", funct, name)
				}

				args := strings.ToLower(output.Arguments)
				if !strings.Contains(args, find) {
					return fmt.Errorf("tooling: expected arguments to contain %q, got %q", find, args)
				}

				return nil
			}
		}
		return fmt.Errorf("tooling: expected function_call output item")
	}

	for _, output := range resp.Output {
		if output.Type == "message" {
			for _, content := range output.Content {
				if content.Type == "output_text" {
					text := strings.ToLower(content.Text)
					if strings.Contains(text, find) {
						return nil
					}
				}
			}
		}
	}

	return fmt.Errorf("content: expected to find %q in output", find)
}
