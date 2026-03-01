package imcdiag_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

// =========================================================================
// Diagnostic tests — one per IMC combination.

func TestDiag_DenseVision(t *testing.T) {
	if len(testlib.MPSimpleVision.ModelFiles) == 0 {
		t.Skip("model Qwen2.5-VL-3B-Instruct-Q8_0 not downloaded")
	}

	cfg := model.Config{
		Log:              diagLog,
		ModelFiles:       testlib.MPSimpleVision.ModelFiles,
		ProjFile:         testlib.MPSimpleVision.ProjFile,
		ContextWindow:    131072,
		NBatch:           2048,
		NUBatch:          2048,
		CacheTypeK:       model.GGMLTypeQ8_0,
		CacheTypeV:       model.GGMLTypeQ8_0,
		NSeqMax:          1,
		IncrementalCache: true,
	}

	runDiag(t, cfg, true)
}

func TestDiag_MoEVision(t *testing.T) {
	if len(testlib.MPMoEVision.ModelFiles) == 0 {
		t.Skip("model Qwen3-VL-30B-A3B-Instruct-Q8_0 not downloaded")
	}

	cfg := model.Config{
		Log:              diagLog,
		ModelFiles:       testlib.MPMoEVision.ModelFiles,
		ProjFile:         testlib.MPMoEVision.ProjFile,
		ContextWindow:    131072,
		NBatch:           2048,
		NUBatch:          2048,
		CacheTypeK:       model.GGMLTypeF16,
		CacheTypeV:       model.GGMLTypeF16,
		NSeqMax:          1,
		IncrementalCache: true,
	}

	runDiag(t, cfg, true)
}

func TestDiag_MoENonDeterministic(t *testing.T) {
	if len(testlib.MPGPTChat.ModelFiles) == 0 {
		t.Skip("model gpt-oss-20b-Q8_0 not downloaded")
	}

	cfg := model.Config{
		Log:              diagLog,
		ModelFiles:       testlib.MPGPTChat.ModelFiles,
		ContextWindow:    131072,
		NBatch:           2048,
		NUBatch:          2048,
		CacheTypeK:       model.GGMLTypeQ8_0,
		CacheTypeV:       model.GGMLTypeQ8_0,
		NSeqMax:          1,
		IncrementalCache: true,
		CacheMinTokens:   100,
	}

	runDiag(t, cfg, false)
}

func TestDiag_HybridVision(t *testing.T) {
	if len(testlib.MPHybridVision.ModelFiles) == 0 {
		t.Skip("model Qwen_Qwen3.5-35B-A3B-Q8_0 not downloaded")
	}

	cfg := model.Config{
		Log:              diagLog,
		ModelFiles:       testlib.MPHybridVision.ModelFiles,
		ProjFile:         testlib.MPHybridVision.ProjFile,
		ContextWindow:    131072,
		NBatch:           2048,
		NUBatch:          2048,
		CacheTypeK:       model.GGMLTypeF16,
		CacheTypeV:       model.GGMLTypeF16,
		NSeqMax:          1,
		IncrementalCache: true,
	}

	runDiag(t, cfg, true)
}

// =========================================================================
// Conversation runner.

// parseTurns splits a turns file into individual user messages.
// Lines starting with [IMAGE] are image turns: the text after the marker
// becomes the image prompt.
func parseTurns(raw string) []string {
	var turns []string
	for line := range strings.SplitSeq(strings.TrimSpace(raw), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			turns = append(turns, line)
		}
	}
	return turns
}

const imageMarker = "[IMAGE] "
const toolMarker = "[TOOL] "

var weatherTools = model.DocumentArray(
	model.D{
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
)

const weatherToolResult = `{"temperature": "82°F", "condition": "Sunny", "humidity": "65%"}`

func runDiag(t *testing.T, cfg model.Config, vision bool) {
	t.Helper()

	krn, err := kronk.New(cfg)
	if err != nil {
		t.Fatalf("unable to load model: %v", err)
	}
	defer func() {
		if err := krn.Unload(context.Background()); err != nil {
			t.Errorf("failed to unload model: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	modelID := krn.ModelInfo().ID
	t.Logf("model: %s  type: %s  vision: %v", modelID, krn.ModelInfo().Type, vision)

	// Select the turn script for this model type.
	turnsRaw := turnsTextRaw
	if vision {
		turnsRaw = turnsVisionRaw
	}
	turns := parseTurns(turnsRaw)

	if len(turns) == 0 {
		t.Fatal("no turns found in turn script")
	}

	// The conversation history grows with each turn.
	messages := []model.D{
		{"role": "system", "content": SystemPrompt},
	}

	for i, turn := range turns {
		turnNum := i + 1

		switch {
		case strings.HasPrefix(turn, toolMarker):
			// Tool turn — two-step: get tool call, inject result, get response.
			question := strings.TrimPrefix(turn, toolMarker)
			messages = append(messages, model.D{
				"role":    "user",
				"content": question,
			})

			toolCalls := chatToolCallTurn(t, ctx, krn, messages, turnNum)
			tc := toolCalls[0]

			argsJSON, err := json.Marshal(map[string]any(tc.Function.Arguments))
			if err != nil {
				t.Fatalf("turn %d: marshal tool call args: %v", turnNum, err)
			}

			messages = append(messages, model.D{
				"role":    "assistant",
				"content": "",
				"tool_calls": []model.D{
					{
						"id":   tc.ID,
						"type": "function",
						"function": model.D{
							"name":      tc.Function.Name,
							"arguments": string(argsJSON),
						},
					},
				},
			})

			messages = append(messages, model.D{
				"role":         "tool",
				"tool_call_id": tc.ID,
				"content":      weatherToolResult,
			})

			resp := chatTurn(t, ctx, krn, messages, turnNum)

			messages = append(messages, model.D{
				"role":    "assistant",
				"content": resp,
			})

		case strings.HasPrefix(turn, imageMarker):
			// Image turn — append image messages then the text prompt.
			imageMsgs, ok := buildImageMessages(t)
			if !ok {
				t.Fatal("image file not available for vision diagnostic")
			}
			messages = append(messages, imageMsgs...)

			resp := chatTurn(t, ctx, krn, messages, turnNum)

			messages = append(messages, model.D{
				"role":    "assistant",
				"content": resp,
			})

		default:
			// Text-only turn.
			messages = append(messages, model.D{
				"role":    "user",
				"content": turn,
			})

			resp := chatTurn(t, ctx, krn, messages, turnNum)

			messages = append(messages, model.D{
				"role":    "assistant",
				"content": resp,
			})
		}
	}
}

// chatTurn sends the full conversation history as a streaming request,
// drains the response, and logs the result. Returns the assistant content
// for appending to the conversation.
func chatTurn(t *testing.T, ctx context.Context, krn *kronk.Kronk, messages []model.D, turn int) string {
	t.Helper()

	d := model.D{
		"messages":    messages,
		"tools":       weatherTools,
		"max_tokens":  512,
		"temperature": 0.0,
	}

	lastMsg := messages[len(messages)-1]
	if content, ok := lastMsg["content"].(string); ok {
		t.Logf("--- turn %d: sending %d messages [user: %s] ---", turn, len(messages), truncate(content, 100))
	} else {
		t.Logf("--- turn %d: sending %d messages [media] ---", turn, len(messages))
	}

	start := time.Now()

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		t.Fatalf("turn %d: chat streaming: %v", turn, err)
	}

	lastResp, content, err := testlib.DrainChat(ctx, ch)
	if err != nil {
		t.Fatalf("turn %d: %v", turn, err)
	}

	elapsed := time.Since(start)

	promptTokens := 0
	outputTokens := 0
	tps := 0.0
	if lastResp.Usage != nil {
		promptTokens = lastResp.Usage.PromptTokens
		outputTokens = lastResp.Usage.OutputTokens
		tps = lastResp.Usage.TokensPerSecond
	}

	t.Logf("turn %d: %dms  prompt=%d  output=%d  tps=%.1f",
		turn, elapsed.Milliseconds(), promptTokens, outputTokens, tps)
	t.Logf("turn %d: %s", turn, truncate(content, 200))

	if content == "" {
		t.Fatalf("turn %d: empty response", turn)
	}

	return content
}

// chatToolCallTurn sends the conversation expecting a tool call response.
// It drains the stream and returns the tool calls from the final chunk.
func chatToolCallTurn(t *testing.T, ctx context.Context, krn *kronk.Kronk, messages []model.D, turn int) []model.ResponseToolCall {
	t.Helper()

	d := model.D{
		"messages":    messages,
		"tools":       weatherTools,
		"max_tokens":  512,
		"temperature": 0.0,
	}

	lastMsg := messages[len(messages)-1]
	if content, ok := lastMsg["content"].(string); ok {
		t.Logf("--- turn %d (tool call): sending %d messages [user: %s] ---", turn, len(messages), truncate(content, 100))
	} else {
		t.Logf("--- turn %d (tool call): sending %d messages ---", turn, len(messages))
	}

	start := time.Now()

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		t.Fatalf("turn %d: tool call chat streaming: %v", turn, err)
	}

	var toolCalls []model.ResponseToolCall
	var lastResp model.ChatResponse
	for resp := range ch {
		lastResp = resp
		if len(resp.Choice) > 0 && resp.Choice[0].FinishReason() == model.FinishReasonTool {
			if resp.Choice[0].Delta != nil && len(resp.Choice[0].Delta.ToolCalls) > 0 {
				toolCalls = resp.Choice[0].Delta.ToolCalls
			}
		}
	}

	elapsed := time.Since(start)

	promptTokens := 0
	outputTokens := 0
	tps := 0.0
	if lastResp.Usage != nil {
		promptTokens = lastResp.Usage.PromptTokens
		outputTokens = lastResp.Usage.OutputTokens
		tps = lastResp.Usage.TokensPerSecond
	}

	t.Logf("turn %d (tool call): %dms  prompt=%d  output=%d  tps=%.1f",
		turn, elapsed.Milliseconds(), promptTokens, outputTokens, tps)

	if len(toolCalls) == 0 {
		t.Fatalf("turn %d: expected tool calls, got none", turn)
	}

	t.Logf("turn %d (tool call): %s(%v)", turn, toolCalls[0].Function.Name, toolCalls[0].Function.Arguments)

	return toolCalls
}

// =========================================================================
// Helpers.

func buildImageMessages(t *testing.T) ([]model.D, bool) {
	t.Helper()

	if _, err := os.Stat(testlib.ImageFile); err != nil {
		return nil, false
	}

	mediaBytes, err := os.ReadFile(testlib.ImageFile)
	if err != nil {
		t.Fatalf("reading image file: %v", err)
	}

	// RawMediaMessage returns two user messages: [image bytes, text prompt].
	return model.RawMediaMessage("What do you see in this picture?", mediaBytes), true
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return fmt.Sprintf("%s...", s[:n])
}
