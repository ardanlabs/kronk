package qwen06_test

import (
	"context"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestLengthTerminatedToolCallBecomesContent(t *testing.T) {
	tests := []struct {
		name          string
		contextWindow int
		maxTokens     int
		promptPadding int
	}{
		{name: "maximum output tokens", contextWindow: 2048, maxTokens: 12},
		{name: "maximum context window", contextWindow: 256, maxTokens: 100, promptPadding: 90},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rawToolOutput string
			cfg := model.Config{
				ModelFiles:          testlib.MPDraft.ModelFiles,
				PtrContextWindow:    &tt.contextWindow,
				PtrPrefillBatchSize: new(256),
				CacheTypeK:          model.GGMLTypeQ8_0,
				CacheTypeV:          model.GGMLTypeQ8_0,
				PtrInsecureLogging:  new(true),
				Log: func(_ context.Context, msg string, args ...any) {
					if msg != "tool-call" {
						return
					}

					var raw bool
					for i := 0; i+1 < len(args); i += 2 {
						if args[i] == "status" && args[i+1] == "raw-model-output" {
							raw = true
						}
					}
					if !raw {
						return
					}

					for i := 0; i+1 < len(args); i += 2 {
						if args[i] == "content" {
							rawToolOutput, _ = args[i+1].(string)
							return
						}
					}
				},
				PtrNSeqMax: new(1),
			}

			testlib.WithModel(t, cfg, func(t *testing.T, krn *kronk.Kronk) {
				ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
				defer cancel()

				prompt := "What is the weather in London, England?" + strings.Repeat(" test", tt.promptPadding)
				d := model.D{
					"messages": []model.D{{
						"role":    "user",
						"content": prompt,
					}},
					"tools": []model.D{{
						"type": "function",
						"function": model.D{
							"name":        "get_weather",
							"description": "Get the current weather for a location",
							"arguments": model.D{
								"location": model.D{
									"type":        "string",
									"description": "The location",
								},
							},
						},
					}},
					"tool_choice":     "required",
					"enable_thinking": false,
					"temperature":     0,
					"seed":            42,
					"max_tokens":      tt.maxTokens,
				}

				resp, err := krn.Chat(ctx, d)
				if err != nil {
					t.Fatalf("Chat: %v", err)
				}
				if len(resp.Choices) != 1 {
					t.Fatalf("Choices: got %d, want 1", len(resp.Choices))
				}

				choice := resp.Choices[0]
				content := choice.Message.Content
				t.Logf("raw model output:    %q", rawToolOutput)
				t.Logf("returned content:    %q", content)
				t.Logf("returned tool calls: %+v", choice.Message.ToolCalls)
				if resp.Usage != nil {
					t.Logf("usage: prompt=%d completion=%d total=%d", resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
				}
				if rawToolOutput == "" {
					t.Fatal("RawToolOutput: got empty content, want buffered tool output")
				}

				markers := []string{"<tool_call>", "</tool_call>", "<|tool_call>", "<tool_call|>"}
				var rawHasMarker bool
				for _, marker := range markers {
					if strings.Contains(rawToolOutput, marker) {
						rawHasMarker = true
					}
					if strings.Contains(content, marker) {
						t.Errorf("Content: got tool marker %q in %q", marker, content)
					}
				}
				if !rawHasMarker {
					t.Errorf("RawToolOutput: got %q, want a tool marker before sanitization", rawToolOutput)
				}

				if got := choice.FinishReason(); got != model.FinishReasonLength {
					t.Fatalf("FinishReason: got %q, want %q", got, model.FinishReasonLength)
				}
				if got := len(choice.Message.ToolCalls); got != 0 {
					t.Fatalf("ToolCalls: got %d, want 0", got)
				}
				const truncatedMessage = "Response truncated before completion."
				if content != truncatedMessage {
					t.Fatalf("Content: got %q, want %q", content, truncatedMessage)
				}

				if resp.Usage == nil {
					t.Fatal("Usage: got nil, want usage")
				}
				generationBudget := min(tt.maxTokens, tt.contextWindow-resp.Usage.PromptTokens)
				if generationBudget <= 0 {
					t.Fatalf("GenerationBudget: got %d, want a positive output budget", generationBudget)
				}
				if got := resp.Usage.CompletionTokens; got != generationBudget {
					t.Errorf("CompletionTokens: got %d, want generated token budget %d", got, generationBudget)
				}
				if got, want := resp.Usage.TotalTokens, resp.Usage.PromptTokens+resp.Usage.CompletionTokens; got != want {
					t.Errorf("TotalTokens: got %d, want %d", got, want)
				}
			})
		})
	}
}
