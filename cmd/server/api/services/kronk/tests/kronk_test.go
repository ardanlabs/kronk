package chatapi_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/uuid"
)

var grammarJSONObject = `root ::= object
value ::= object | array | string | number | "true" | "false" | "null"
object ::= "{" ws ( string ":" ws value ("," ws string ":" ws value)* )? ws "}"
array ::= "[" ws ( value ("," ws value)* )? ws "]"
string ::= "\"" ([^"\\] | "\\" ["\\bfnrt/] | "\\u" [0-9a-fA-F]{4})* "\""
number ::= "-"? ("0" | [1-9][0-9]*) ("." [0-9]+)? ([eE] [+-]? [0-9]+)?
ws ::= [ \t\n\r]*`

var (
	gw        = os.Getenv("GITHUB_WORKSPACE")
	imageFile = filepath.Join(gw, "examples/samples/giraffe.jpg")
	audioFile = filepath.Join(gw, "examples/samples/jfk.wav")
)

func Test_API(t *testing.T) {
	test := apitest.New(t, "Test_API")

	tokens := createTokens(t, test.Sec)

	// =========================================================================
	// Tests are organized by model to minimize model loading/unloading.
	// Each model group runs all its tests before moving to the next model.

	// -------------------------------------------------------------------------
	// Model: Qwen3-8B-Q8_0 (text chat and responses)

	test.Run(t, chatNonStreamQwen3(t, tokens), "chat-nonstream-qwen3")
	test.RunStreaming(t, chatStreamQwen3(t, tokens), "chat-stream-qwen3")
	test.Run(t, chatArrayFormatQwen3(t, tokens), "chat-array-format-qwen3")
	test.RunStreaming(t, chatArrayFormatStreamQwen3(t, tokens), "chat-array-format-stream-qwen3")
	test.RunStreaming(t, chatStreamIMCQwen3(t, tokens), "chat-stream-imc-qwen3")
	test.Run(t, chatGrammarQwen3(t, tokens), "chat-grammar-qwen3")
	test.RunStreaming(t, chatGrammarStreamQwen3(t, tokens), "chat-grammar-stream-qwen3")
	test.Run(t, chatToolCallQwen3(t, tokens), "chat-toolcall-qwen3")
	test.RunStreaming(t, chatToolCallStreamQwen3(t, tokens), "chat-toolcall-stream-qwen3")
	test.Run(t, respNonStreamQwen3(t, tokens), "resp-nonstream-qwen3")
	test.RunStreaming(t, respStreamQwen3(t, tokens), "resp-stream-qwen3")
	test.Run(t, msgsNonStreamQwen3(t, tokens), "msgs-nonstream-qwen3")
	test.RunStreaming(t, msgsStreamQwen3(t, tokens), "msgs-stream-qwen3")
	test.Run(t, tokenize200(tokens), "tokenize-200")

	// -------------------------------------------------------------------------
	// Model: Qwen3.5-0.8B-Q8_0 (vision)

	test.Run(t, chatImageQwen35VL(t, tokens), "chat-image-qwen35vl")
	test.Run(t, respImageQwen35VL(t, tokens), "resp-image-qwen35vl")
	test.Run(t, msgsImageQwen35VL(t, tokens), "msgs-image-qwen35vl")

	// -------------------------------------------------------------------------
	// Model: Qwen2.5-Omni-3B-Q8_0 (audio)

	const audioSkipReason = "disabled until Qwen2.5-Omni audio generation is stable"
	test.Run(t, chatAudioQwen25Omni(t, tokens), "chat-audio-qwen25omni", apitest.WithSkip(true, audioSkipReason))
	test.Run(t, respAudioQwen25Omni(t, tokens), "resp-audio-qwen25omni", apitest.WithSkip(true, audioSkipReason))

	// -------------------------------------------------------------------------
	// Model: Qwen3-Embedding-0.6B-Q8_0

	test.Run(t, chatEmbed200(tokens), "embedding-200")

	// -------------------------------------------------------------------------
	// Model: bge-reranker-v2-m3-Q8_0

	test.Run(t, rerank200(tokens), "rerank-200")

	// -------------------------------------------------------------------------
	// Model: ggml-tiny.bin (whisper / bucky)

	test.Run(t, audioTranscriptions200(t, tokens), "audio-transcriptions-200")

	// -------------------------------------------------------------------------
	// Auth tests (don't require model loading, use tokens without the required grant)

	test.Run(t, chatEndpoint403(tokens), "chatEndpoint-403")
	test.Run(t, respEndpoint403(tokens), "respEndpoint-403")
	test.Run(t, msgsEndpoint403(tokens), "msgsEndpoint-403")
	test.Run(t, embed403(tokens), "embedding-403")
	test.Run(t, rerank403(tokens), "rerank-403")
	test.Run(t, tokenize403(tokens), "tokenize-403")
	test.Run(t, audioTranscriptions403(t, tokens), "audio-transcriptions-403")
}

// =============================================================================

func createTokens(t *testing.T, sec *security.Security) map[string]string {
	tokens := make(map[string]string)

	token, err := sec.GenerateToken(true, nil, 60*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	tokens["admin"] = token

	// -------------------------------------------------------------------------

	token, err = sec.GenerateToken(true, nil, 60*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	tokens["non-admin-no-endpoints"] = token

	// -------------------------------------------------------------------------

	endpoints := map[string]auth.RateLimit{
		"chat-completions": {
			Limit:  0,
			Window: auth.RateUnlimited,
		},
	}

	token, err = sec.GenerateToken(false, endpoints, 60*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	tokens["chat-completions"] = token

	// -------------------------------------------------------------------------

	endpoints = map[string]auth.RateLimit{
		"embeddings": {
			Limit:  0,
			Window: auth.RateUnlimited,
		},
	}

	token, err = sec.GenerateToken(false, endpoints, 60*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	tokens["embeddings"] = token

	// -------------------------------------------------------------------------

	endpoints = map[string]auth.RateLimit{
		"responses": {
			Limit:  0,
			Window: auth.RateUnlimited,
		},
	}

	token, err = sec.GenerateToken(false, endpoints, 60*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	tokens["responses"] = token

	// -------------------------------------------------------------------------

	endpoints = map[string]auth.RateLimit{
		"rerank": {
			Limit:  0,
			Window: auth.RateUnlimited,
		},
	}

	token, err = sec.GenerateToken(false, endpoints, 60*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	tokens["rerank"] = token

	// -------------------------------------------------------------------------

	endpoints = map[string]auth.RateLimit{
		"messages": {
			Limit:  0,
			Window: auth.RateUnlimited,
		},
	}

	token, err = sec.GenerateToken(false, endpoints, 60*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	tokens["messages"] = token

	// -------------------------------------------------------------------------

	endpoints = map[string]auth.RateLimit{
		"tokenize": {
			Limit:  0,
			Window: auth.RateUnlimited,
		},
	}

	token, err = sec.GenerateToken(false, endpoints, 60*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	tokens["tokenize"] = token

	// -------------------------------------------------------------------------

	endpoints = map[string]auth.RateLimit{
		"transcriptions": {
			Limit:  0,
			Window: auth.RateUnlimited,
		},
	}

	token, err = sec.GenerateToken(false, endpoints, 60*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	tokens["transcriptions"] = token

	return tokens
}

func readFile(file string) ([]byte, error) {
	if _, err := os.Stat(file); err != nil {
		return nil, fmt.Errorf("error accessing file %q: %w", file, err)
	}

	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("error reading file %q: %w", file, err)
	}

	return data, nil
}

// =============================================================================

type responseValidator struct {
	resp      *model.ChatResponse
	streaming bool
	errors    []string
	warnings  []string
}

func validateResponse(got any, streaming bool) responseValidator {
	return responseValidator{resp: got.(*model.ChatResponse), streaming: streaming}
}

func (v responseValidator) getMsg() model.ResponseMessage {
	if v.streaming && v.resp.Choices[0].FinishReason() == "" && v.resp.Choices[0].Delta != nil {
		return *v.resp.Choices[0].Delta
	}
	if v.resp.Choices[0].Message != nil {
		return *v.resp.Choices[0].Message
	}
	return model.ResponseMessage{}
}

func (v responseValidator) hasValidUUID() responseValidator {
	id := v.resp.ID

	// Try parsing as-is first.
	if _, err := uuid.Parse(id); err == nil {
		return v
	}

	// Try extracting UUID from the last 36 characters (after prefix).
	if len(id) >= 36 {
		if _, err := uuid.Parse(id[len(id)-36:]); err == nil {
			return v
		}
	}

	v.errors = append(v.errors, "expected id to contain a valid UUID")

	return v
}

func (v responseValidator) hasCreated() responseValidator {
	if v.resp.Created <= 0 {
		v.errors = append(v.errors, "expected created to be greater than 0")
	}

	return v
}

func (v responseValidator) hasUsage(reasoning bool) responseValidator {
	u := v.resp.Usage

	if u.PromptTokens <= 0 {
		v.errors = append(v.errors, "expected prompt_tokens to be greater than 0")
	}

	if reasoning && u.ReasoningTokens <= 0 {
		v.errors = append(v.errors, "expected reasoning_tokens to be greater than 0")
	}

	if u.CompletionTokens <= 0 {
		v.errors = append(v.errors, "expected completion_tokens to be greater than 0")
	}

	if u.OutputTokens <= 0 {
		v.errors = append(v.errors, "expected output_tokens to be greater than 0")
	}

	if u.TotalTokens <= 0 {
		v.errors = append(v.errors, "expected total_tokens to be greater than 0")
	}

	if u.TokensPerSecond <= 0 {
		v.errors = append(v.errors, "expected tokens_per_second to be greater than 0")
	}

	return v
}

func (v responseValidator) hasValidChoice() responseValidator {
	switch {
	case len(v.resp.Choices) == 0:
		v.errors = append(v.errors, "expected at least one choice")

	case v.resp.Choices[0].Index != 0:
		v.errors = append(v.errors, "expected index to be 0")
	}

	return v
}

func (v responseValidator) hasContent() responseValidator {
	if len(v.resp.Choices) == 0 {
		v.errors = append(v.errors, "expected at least one choice")
		return v
	}

	if v.getMsg().Content == "" {
		v.errors = append(v.errors, "expected content to be non-empty")
	}

	return v
}

func (v responseValidator) hasReasoning() responseValidator {
	if len(v.resp.Choices) == 0 {
		v.errors = append(v.errors, "expected at least one choice")
		return v
	}

	if v.getMsg().Reasoning == "" {
		v.errors = append(v.errors, "expected reasoning to be non-empty")
	}

	return v
}

func (v responseValidator) warnContainsInContent(find string) responseValidator {
	if len(v.resp.Choices) == 0 {
		return v
	}

	if !strings.Contains(strings.ToLower(v.getMsg().Content), find) {
		v.warnings = append(v.warnings, fmt.Sprintf("WARNING: expected to find %q in content, got: %s", find, v.getMsg().Content))
	}

	return v
}

func (v responseValidator) warnContainsInReasoning(find string) responseValidator {
	if len(v.resp.Choices) == 0 {
		return v
	}

	if !strings.Contains(strings.ToLower(v.getMsg().Reasoning), find) {
		v.warnings = append(v.warnings, fmt.Sprintf("WARNING: expected to find %q in reasoning, got: %s", find, v.getMsg().Reasoning))
	}

	return v
}

func (v responseValidator) hasNoLogprobs() responseValidator {
	if len(v.resp.Choices) == 0 {
		return v
	}

	if v.resp.Choices[0].Logprobs != nil {
		v.errors = append(v.errors, "expected logprobs to be nil in final streaming chunk")
	}

	return v
}

func (v responseValidator) hasLogprobs(topLogprobs int) responseValidator {
	if len(v.resp.Choices) == 0 {
		v.errors = append(v.errors, "expected at least one choice for logprobs check")
		return v
	}

	logprobs := v.resp.Choices[0].Logprobs
	if logprobs == nil {
		v.errors = append(v.errors, "expected logprobs to be non-nil")
		return v
	}

	if len(logprobs.Content) == 0 {
		v.errors = append(v.errors, "expected logprobs.content to have at least one entry")
		return v
	}

	for i, lp := range logprobs.Content {
		if lp.Token == "" {
			v.errors = append(v.errors, fmt.Sprintf("expected logprobs.content[%d].token to be non-empty", i))
		}

		if lp.Logprob > 0 {
			v.errors = append(v.errors, fmt.Sprintf("expected logprobs.content[%d].logprob to be <= 0, got %f", i, lp.Logprob))
		}

		if len(lp.Bytes) == 0 {
			v.errors = append(v.errors, fmt.Sprintf("expected logprobs.content[%d].bytes to be non-empty", i))
		}

		if topLogprobs > 0 {
			if len(lp.TopLogprobs) == 0 {
				v.errors = append(v.errors, fmt.Sprintf("expected logprobs.content[%d].top_logprobs to have entries", i))
			} else if len(lp.TopLogprobs) > topLogprobs {
				v.errors = append(v.errors, fmt.Sprintf("expected logprobs.content[%d].top_logprobs to have at most %d entries, got %d", i, topLogprobs, len(lp.TopLogprobs)))
			}
		}
	}

	return v
}

func (v responseValidator) hasNoPrompt() responseValidator {
	if v.resp.Prompt != "" {
		v.errors = append(v.errors, "expected prompt to be empty when return_prompt is not set")
	}

	return v
}

func (v responseValidator) hasValidJSON() responseValidator {
	if len(v.resp.Choices) == 0 {
		v.errors = append(v.errors, "expected at least one choice")
		return v
	}

	content := strings.TrimSpace(v.getMsg().Content)
	if content == "" {
		v.errors = append(v.errors, "expected content to be non-empty for JSON validation")
		return v
	}

	var js any
	if err := json.Unmarshal([]byte(content), &js); err != nil {
		v.errors = append(v.errors, fmt.Sprintf("expected valid JSON, got parse error: %v, content: %s", err, content))
	}

	return v
}

func (v responseValidator) hasToolCalls(funcName string) responseValidator {
	if len(v.resp.Choices) == 0 {
		v.errors = append(v.errors, "expected at least one choice")
		return v
	}

	choice := v.resp.Choices[0]

	// Check finish reason is "tool_calls".
	if choice.FinishReason() != "tool_calls" {
		v.errors = append(v.errors, fmt.Sprintf("expected finish_reason to be 'tool_calls', got '%s'", choice.FinishReason()))
		return v
	}

	// For non-streaming: check tool calls in Message.
	// For streaming: check tool calls in Delta.
	switch v.streaming {
	case true:
		if choice.Delta == nil || len(choice.Delta.ToolCalls) == 0 {
			v.errors = append(v.errors, "expected tool calls in Delta for streaming")
		} else if !strings.Contains(strings.ToLower(choice.Delta.ToolCalls[0].Function.Name), strings.ToLower(funcName)) {
			v.errors = append(v.errors, fmt.Sprintf("expected Delta tool call function name to contain '%s', got '%s'", funcName, choice.Delta.ToolCalls[0].Function.Name))
		}

	default:
		if choice.Message == nil || len(choice.Message.ToolCalls) == 0 {
			v.errors = append(v.errors, "expected tool calls in Message")
		} else if !strings.Contains(strings.ToLower(choice.Message.ToolCalls[0].Function.Name), strings.ToLower(funcName)) {
			v.errors = append(v.errors, fmt.Sprintf("expected tool call function name to contain '%s', got '%s'", funcName, choice.Message.ToolCalls[0].Function.Name))
		}
	}

	return v
}

func validateToolCallStream(events []json.RawMessage, funcName string, argName string) string {
	type functionDelta struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	type toolCallDelta struct {
		ID       string        `json:"id"`
		Index    int           `json:"index"`
		Type     string        `json:"type"`
		Function functionDelta `json:"function"`
	}
	type streamEvent struct {
		ID      string `json:"id"`
		Choices []struct {
			Delta struct {
				ToolCalls []toolCallDelta `json:"tool_calls"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
	}

	if len(events) == 0 {
		return "expected at least one streaming event"
	}

	var completionID string
	var toolCallID string
	var arguments strings.Builder
	var finalFinishReason string
	seenToolCall := false
	seenFunctionName := false

	for i, data := range events {
		var event streamEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Sprintf("stream event %d: unmarshal: %v", i, err)
		}
		if event.ID == "" {
			return fmt.Sprintf("stream event %d: expected completion id", i)
		}
		if completionID == "" {
			completionID = event.ID
		} else if event.ID != completionID {
			return fmt.Sprintf("stream event %d: completion id changed from %q to %q", i, completionID, event.ID)
		}
		if len(event.Choices) == 0 {
			return fmt.Sprintf("stream event %d: expected at least one choice", i)
		}

		choice := event.Choices[0]
		if choice.FinishReason != nil {
			finalFinishReason = *choice.FinishReason
		}
		for _, toolCall := range choice.Delta.ToolCalls {
			if toolCall.Index != 0 {
				return fmt.Sprintf("stream event %d: tool call index: got %d, want 0", i, toolCall.Index)
			}
			seenToolCall = true
			if toolCall.ID != "" {
				if toolCallID == "" {
					toolCallID = toolCall.ID
				} else if toolCall.ID != toolCallID {
					return fmt.Sprintf("stream event %d: tool call id changed from %q to %q", i, toolCallID, toolCall.ID)
				}
			}
			if toolCall.Type != "" && toolCall.Type != "function" {
				return fmt.Sprintf("stream event %d: tool call type: got %q, want %q", i, toolCall.Type, "function")
			}
			if toolCall.Function.Name != "" {
				if !strings.EqualFold(toolCall.Function.Name, funcName) {
					return fmt.Sprintf("stream event %d: function name: got %q, want %q", i, toolCall.Function.Name, funcName)
				}
				seenFunctionName = true
			}
			arguments.WriteString(toolCall.Function.Arguments)
		}
	}

	if !seenToolCall {
		return "expected streamed tool call deltas"
	}
	if toolCallID == "" {
		return "expected streamed tool call id"
	}
	if !seenFunctionName {
		return fmt.Sprintf("expected streamed function name %q", funcName)
	}
	if finalFinishReason != "tool_calls" {
		return fmt.Sprintf("final finish reason: got %q, want %q", finalFinishReason, "tool_calls")
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(arguments.String()), &args); err != nil {
		return fmt.Sprintf("streamed tool call arguments: expected valid JSON, got %q: %v", arguments.String(), err)
	}
	if _, exists := args[argName]; !exists {
		return fmt.Sprintf("streamed tool call arguments: expected argument %q, got %v", argName, args)
	}

	return ""
}

func (v responseValidator) result(t *testing.T) string {
	for _, w := range v.warnings {
		t.Log(w)
	}

	if len(v.errors) == 0 {
		return ""
	}

	return strings.Join(v.errors, "; ")
}

// =============================================================================

type respResponseValidator struct {
	resp     *kronk.ResponseResponse
	errors   []string
	warnings []string
}

func validateRespResponse(got any) respResponseValidator {
	return respResponseValidator{resp: got.(*kronk.ResponseResponse)}
}

func (v respResponseValidator) hasValidID() respResponseValidator {
	if v.resp.ID == "" || len(v.resp.ID) < 5 {
		v.errors = append(v.errors, "expected id to be a valid response ID")
	}

	return v
}

func (v respResponseValidator) hasCreatedAt() respResponseValidator {
	if v.resp.CreatedAt <= 0 {
		v.errors = append(v.errors, "expected created_at to be greater than 0")
	}

	return v
}

func (v respResponseValidator) hasStatus(expected string) respResponseValidator {
	if v.resp.Status != expected {
		v.errors = append(v.errors, "expected status to be "+expected)
	}

	return v
}

func (v respResponseValidator) hasOutput() respResponseValidator {
	if len(v.resp.Output) == 0 {
		v.errors = append(v.errors, "expected at least one output item")
	}

	return v
}

func (v respResponseValidator) hasOutputText() respResponseValidator {
	if len(v.resp.Output) == 0 {
		return v
	}

	for _, item := range v.resp.Output {
		if item.Type == "message" && len(item.Content) > 0 {
			for _, content := range item.Content {
				if content.Type == "output_text" && content.Text != "" {
					return v
				}
			}
		}
	}

	v.errors = append(v.errors, "expected output to contain text content")
	return v
}

func (v respResponseValidator) warnContainsInContent(find string) respResponseValidator {
	if len(v.resp.Output) == 0 {
		return v
	}

	for _, item := range v.resp.Output {
		if item.Type == "message" && len(item.Content) > 0 {
			for _, content := range item.Content {
				if content.Type == "output_text" {
					if containsIgnoreCase(content.Text, find) {
						return v
					}
				}
			}
		}
	}

	v.warnings = append(v.warnings, "WARNING: expected to find \""+find+"\" in content, got: "+v.extractContent())
	return v
}

func (v respResponseValidator) result(t *testing.T) string {
	for _, w := range v.warnings {
		t.Log(w)
	}

	if len(v.errors) == 0 {
		return ""
	}

	return strings.Join(v.errors, "; ")
}

func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func (v respResponseValidator) extractContent() string {
	var texts []string
	for _, item := range v.resp.Output {
		if item.Type == "message" {
			for _, content := range item.Content {
				if content.Type == "output_text" && content.Text != "" {
					texts = append(texts, content.Text)
				}
			}
		}
	}
	return strings.Join(texts, " | ")
}
