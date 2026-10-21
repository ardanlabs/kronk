package testlib

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// WithRetry wraps a test function with retry logic to handle LLM
// non-determinism under concurrent load. On failure, it logs the attempt
// and retries up to MaxRetries times before returning the final error.
func WithRetry(t testing.TB, f func() error) func() error {
	return func() error {
		var err error
		for attempt := 1; attempt <= MaxRetries; attempt++ {
			err = f()
			if err == nil {
				return nil
			}
			if attempt < MaxRetries {
				t.Logf("RETRY: attempt %d/%d failed: %v", attempt, MaxRetries, err)
			}
		}
		return err
	}
}

// TestResult holds an error and non-fatal warnings from test validation.
type TestResult struct {
	Err      error
	Warnings []string
}

// StreamAccumulator collects content from streaming delta chunks.
type StreamAccumulator struct {
	Content   strings.Builder
	Reasoning strings.Builder
	ToolCalls []model.ResponseToolCall
}

// Accumulate adds delta content from a streaming response chunk.
func (sa *StreamAccumulator) Accumulate(resp model.ChatResponse) {
	if len(resp.Choices) == 0 {
		return
	}

	choice := resp.Choices[0]

	if choice.FinishReason() == "" && choice.Delta != nil {
		sa.Content.WriteString(choice.Delta.Content)
		sa.Reasoning.WriteString(choice.Delta.Reasoning)
		if len(choice.Delta.ToolCalls) > 0 {
			sa.ToolCalls = append(sa.ToolCalls, choice.Delta.ToolCalls...)
		}
		return
	}

	if choice.Message != nil {
		sa.Content.WriteString(choice.Message.Content)
		sa.Reasoning.WriteString(choice.Message.Reasoning)
		if len(choice.Message.ToolCalls) > 0 {
			sa.ToolCalls = append(sa.ToolCalls, choice.Message.ToolCalls...)
		}
	}
}

// GetMsg returns the message from a choice, preferring Delta for streaming.
func GetMsg(choice model.Choice, streaming bool) model.ResponseMessage {
	if streaming && choice.FinishReason() == "" && choice.Delta != nil {
		return *choice.Delta
	}
	if choice.Message != nil {
		return *choice.Message
	}
	return model.ResponseMessage{}
}

// =========================================================================

// TestChatBasics validates the common fields of a chat response.
func TestChatBasics(resp model.ChatResponse, modelName string, object string, reasoning bool, streaming bool) error {
	if resp.ID == "" {
		return fmt.Errorf("expected id")
	}

	if resp.Object != object {
		return fmt.Errorf("expected object type to be %s, got %s", object, resp.Object)
	}

	if resp.Created == 0 {
		return fmt.Errorf("expected created time")
	}

	if resp.Model != modelName {
		return fmt.Errorf("basics: expected model to be %s, got %s", modelName, resp.Model)
	}

	if len(resp.Choices) == 0 {
		return fmt.Errorf("basics: expected choice, got %d", len(resp.Choices))
	}

	msg := GetMsg(resp.Choices[0], streaming)

	if resp.Choices[0].FinishReason() == "" &&
		msg.Content == "" &&
		msg.Reasoning == "" &&
		len(msg.ToolCalls) == 0 &&
		len(msg.ToolCallDeltas) == 0 {
		return fmt.Errorf("basics: expected delta content, reasoning, or tool calls to be non-empty")
	}

	if resp.Choices[0].FinishReason() == "" && msg.Role != "assistant" {
		return fmt.Errorf("basics: expected delta role to be assistant, got %s", msg.Role)
	}

	if resp.Choices[0].FinishReason() == "stop" && msg.Content == "" {
		return fmt.Errorf("basics: expected final content to be non-empty")
	}

	if resp.Choices[0].FinishReason() == "tool_calls" && len(msg.ToolCalls) == 0 {
		return fmt.Errorf("basics: expected tool calls to be non-empty")
	}

	if resp.Choices[0].FinishReason() == "tool_calls" {
		if resp.Choices[0].Message == nil || len(resp.Choices[0].Message.ToolCalls) == 0 {
			return fmt.Errorf("basics: expected tool calls in Message")
		}
	}

	if reasoning {
		if resp.Choices[0].FinishReason() == "stop" && msg.Reasoning == "" {
			return fmt.Errorf("basics: expected final reasoning")
		}
	}

	return nil
}

// TestChatResponse validates a full chat response including content matching.
func TestChatResponse(resp model.ChatResponse, modelName string, object string, find string, funct string, arg string, streaming bool, reasoning bool) TestResult {
	if err := TestChatBasics(resp, modelName, object, reasoning, streaming); err != nil {
		return TestResult{Err: err}
	}

	var result TestResult

	msg := GetMsg(resp.Choices[0], streaming)

	if funct != "" {
		if resp.Choices[0].FinishReason() != model.FinishReasonTool {
			return TestResult{Err: fmt.Errorf("tooling: expected finish reason %q, got %q", model.FinishReasonTool, resp.Choices[0].FinishReason())}
		}
		if err := validateToolCalls(msg.ToolCalls, find, funct, arg); err != nil {
			return TestResult{Err: err}
		}
	}

	find = strings.ToLower(find)
	msg.Reasoning = strings.ToLower(msg.Reasoning)
	msg.Content = strings.ToLower(msg.Content)

	if reasoning {
		if len(msg.Reasoning) == 0 {
			result.Err = fmt.Errorf("content: expected some reasoning")
		}

		switch {
		case funct == "":
			if !strings.Contains(msg.Reasoning, find) {
				result.Warnings = append(result.Warnings, fmt.Sprintf("reasoning: expected %q, got %q", find, msg.Reasoning))
			}

		case funct != "":
			if !strings.Contains(msg.Reasoning, funct) {
				result.Warnings = append(result.Warnings, fmt.Sprintf("reasoning: expected %q, got %q", funct, msg.Reasoning))
			}
		}
	}

	if resp.Choices[0].FinishReason() == "stop" {
		if len(msg.Content) == 0 {
			result.Err = fmt.Errorf("content: expected some content")
		}

		if !strings.Contains(msg.Content, find) {
			result.Warnings = append(result.Warnings, fmt.Sprintf("content: expected %q, got %q", find, msg.Content))
			return result
		}
	}

	return result
}

// TestStreamingContent validates accumulated streaming content.
func TestStreamingContent(acc *StreamAccumulator, lastResp model.ChatResponse, find string) TestResult {
	var result TestResult

	content := strings.ToLower(acc.Content.String())
	reasoning := strings.ToLower(acc.Reasoning.String())
	find = strings.ToLower(find)

	if len(content) == 0 && len(reasoning) == 0 {
		result.Err = fmt.Errorf("streaming: expected some content or reasoning, got neither")
		return result
	}

	if !strings.Contains(content, find) && !strings.Contains(reasoning, find) {
		result.Warnings = append(result.Warnings, fmt.Sprintf("streaming: expected %q in content or reasoning, content=%q, reasoning=%q", find, content, reasoning))
	}

	return result
}

// TestStreamingToolCall validates accumulated streaming tool call content.
func TestStreamingToolCall(acc *StreamAccumulator, lastResp model.ChatResponse, find string, funct string, arg string) TestResult {
	if len(lastResp.Choices) == 0 || lastResp.Choices[0].FinishReason() != model.FinishReasonTool {
		return TestResult{Err: fmt.Errorf("streaming: expected terminal tool_calls choice")}
	}

	var toolCalls []model.ResponseToolCall
	if len(acc.ToolCalls) > 0 {
		toolCalls = acc.ToolCalls
	} else if len(lastResp.Choices) > 0 && lastResp.Choices[0].Message != nil {
		toolCalls = lastResp.Choices[0].Message.ToolCalls
	}

	return TestResult{Err: validateToolCalls(toolCalls, find, funct, arg)}
}

func validateToolCalls(toolCalls []model.ResponseToolCall, find string, funct string, arg string) error {
	if len(toolCalls) != 1 {
		return fmt.Errorf("tooling: expected one tool call, got %d", len(toolCalls))
	}

	function := toolCalls[0].Function
	if function.Name != funct {
		return fmt.Errorf("tooling: expected function %q, got %q", funct, function.Name)
	}

	value, ok := function.Arguments[arg].(string)
	if !ok {
		return fmt.Errorf("tooling: expected string argument %q, got %v", arg, function.Arguments)
	}

	if !strings.Contains(strings.ToLower(value), strings.ToLower(find)) {
		return fmt.Errorf("tooling: expected %q in %s, got %q", find, arg, value)
	}

	return nil
}

// =========================================================================
// Response API helpers

// TestResponseBasics validates the common fields of a Response API response.
func TestResponseBasics(resp kronk.ResponseResponse, modelName string) error {
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

// TestResponseResponse validates a Response API response with content matching.
func TestResponseResponse(resp kronk.ResponseResponse, modelName string, find string, funct string, arg string) error {
	if err := TestResponseBasics(resp, modelName); err != nil {
		return err
	}

	find = strings.ToLower(find)

	if funct != "" {
		var toolCalls []model.ResponseToolCall
		for _, output := range resp.Output {
			if output.Type == "function_call" {
				if output.Arguments == nil {
					return fmt.Errorf("tooling: expected arguments")
				}

				var args map[string]any
				if err := json.Unmarshal([]byte(*output.Arguments), &args); err != nil {
					return fmt.Errorf("tooling: decoding arguments: %w", err)
				}
				toolCalls = append(toolCalls, model.ResponseToolCall{
					Function: model.ResponseToolCallFunction{Name: output.Name, Arguments: model.ToolCallArguments(args)},
				})
			}
		}
		return validateToolCalls(toolCalls, find, funct, arg)
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

// =========================================================================
// Media helpers

// TestMediaResponseResponse validates a media Response API response.
func TestMediaResponseResponse(resp kronk.ResponseResponse, modelName string, find string) error {
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
		return fmt.Errorf("expected model to be %s, got %s", modelName, resp.Model)
	}

	if resp.Status != "completed" {
		return fmt.Errorf("expected status to be completed, got %s", resp.Status)
	}

	if len(resp.Output) == 0 {
		return fmt.Errorf("expected output, got %d", len(resp.Output))
	}

	find = strings.ToLower(find)

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

	return fmt.Errorf("expected to find %q in output", find)
}

// =========================================================================
// Grammar helpers

// TestGrammarJSONResponse validates a grammar-constrained chat response.
func TestGrammarJSONResponse(resp model.ChatResponse, modelName string) TestResult {
	var result TestResult

	if resp.ID == "" {
		result.Err = fmt.Errorf("expected id")
		return result
	}

	if resp.Object != model.ObjectChatTextFinal {
		result.Err = fmt.Errorf("expected object type to be %s, got %s", model.ObjectChatTextFinal, resp.Object)
		return result
	}

	if resp.Model != modelName {
		result.Err = fmt.Errorf("expected model to be %s, got %s", modelName, resp.Model)
		return result
	}

	if len(resp.Choices) == 0 {
		result.Err = fmt.Errorf("expected choice, got %d", len(resp.Choices))
		return result
	}

	msg := resp.Choices[0].Message
	if msg == nil {
		result.Err = fmt.Errorf("expected message to be non-nil")
		return result
	}

	content := strings.TrimSpace(msg.Content)
	if content == "" {
		result.Err = fmt.Errorf("expected content to be non-empty")
		return result
	}

	return TestGrammarStreamingContent(content, modelName)
}

// TestGrammarStreamingContent validates grammar-constrained streaming content.
func TestGrammarStreamingContent(content string, modelName string) TestResult {
	var result TestResult

	content = strings.TrimSpace(content)
	if content == "" {
		result.Err = fmt.Errorf("grammar streaming: expected content, got empty")
		return result
	}

	var js struct {
		Languages []struct {
			Name string `json:"name"`
			Year int    `json:"year"`
		} `json:"languages"`
	}
	if err := json.Unmarshal([]byte(content), &js); err != nil {
		result.Err = fmt.Errorf("grammar streaming: expected valid JSON, got parse error: %w\ncontent: %s", err, content)
		return result
	}

	if len(js.Languages) != 3 {
		return TestResult{Err: fmt.Errorf("grammar: expected three languages, got %d", len(js.Languages))}
	}
	for i, language := range js.Languages {
		if strings.TrimSpace(language.Name) == "" || language.Year <= 0 {
			return TestResult{Err: fmt.Errorf("grammar: language %d: expected non-empty name and positive integer year, got %+v", i, language)}
		}
	}

	return result
}

// =========================================================================
// Drain helper

// DrainChat reads all responses from a streaming chat channel, returning
// the last response and accumulated content.
func DrainChat(ctx context.Context, ch <-chan model.ChatResponse) (model.ChatResponse, string, error) {
	var lastResp model.ChatResponse
	var reasoning strings.Builder
	var content strings.Builder

	for resp := range ch {
		lastResp = resp
		if len(resp.Choices) > 0 && resp.Choices[0].Delta != nil {
			reasoning.WriteString(resp.Choices[0].Delta.Reasoning)
			content.WriteString(resp.Choices[0].Delta.Content)
		}
	}

	if len(lastResp.Choices) > 0 && lastResp.Choices[0].FinishReason() == model.FinishReasonError {
		errMsg := ""
		if lastResp.Choices[0].Delta != nil {
			errMsg = lastResp.Choices[0].Delta.Content
		}
		return lastResp, content.String(), fmt.Errorf("model error: %s", errMsg)
	}

	if content.Len() == 0 && reasoning.Len() > 0 {
		return lastResp, reasoning.String(), nil
	}

	return lastResp, content.String(), nil
}
