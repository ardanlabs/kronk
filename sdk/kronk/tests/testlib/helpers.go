package testlib

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

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
	if len(resp.Choice) == 0 {
		return
	}

	choice := resp.Choice[0]

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

// HasReasoningField reports whether the model populates the Reasoning field.
// MoE and Hybrid models emit reasoning tokens inline in Content.
func HasReasoningField(krn *kronk.Kronk) bool {
	mt := krn.ModelInfo().Type
	return mt != model.ModelTypeMoE && mt != model.ModelTypeHybrid
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

	if len(resp.Choice) == 0 {
		return fmt.Errorf("basics: expected choice, got %d", len(resp.Choice))
	}

	msg := GetMsg(resp.Choice[0], streaming)

	if resp.Choice[0].FinishReason() == "" && msg.Content == "" && msg.Reasoning == "" {
		return fmt.Errorf("basics: expected delta content and reasoning to be non-empty")
	}

	if resp.Choice[0].FinishReason() == "" && msg.Role != "assistant" {
		return fmt.Errorf("basics: expected delta role to be assistant, got %s", msg.Role)
	}

	if resp.Choice[0].FinishReason() == "stop" && msg.Content == "" {
		return fmt.Errorf("basics: expected final content to be non-empty")
	}

	if resp.Choice[0].FinishReason() == "tool_calls" && len(msg.ToolCalls) == 0 {
		return fmt.Errorf("basics: expected tool calls to be non-empty")
	}

	if resp.Choice[0].FinishReason() == "tool_calls" && streaming {
		if resp.Choice[0].Delta == nil || len(resp.Choice[0].Delta.ToolCalls) == 0 {
			return fmt.Errorf("basics: expected tool calls in Delta for streaming compatibility")
		}
	}

	if resp.Choice[0].FinishReason() == "tool_calls" && !streaming {
		if resp.Choice[0].Message == nil || len(resp.Choice[0].Message.ToolCalls) == 0 {
			return fmt.Errorf("basics: expected tool calls in Message for non-streaming")
		}
	}

	if reasoning {
		if resp.Choice[0].FinishReason() == "stop" && msg.Reasoning == "" {
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

	msg := GetMsg(resp.Choice[0], streaming)

	find = strings.ToLower(find)
	funct = strings.ToLower(funct)
	msg.Reasoning = strings.ToLower(msg.Reasoning)
	msg.Content = strings.ToLower(msg.Content)

	if len(msg.ToolCalls) > 0 {
		msg.ToolCalls[0].Function.Name = strings.ToLower(msg.ToolCalls[0].Function.Name)
	}

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

	if resp.Choice[0].FinishReason() == "stop" {
		if len(msg.Content) == 0 {
			result.Err = fmt.Errorf("content: expected some content")
		}

		if !strings.Contains(msg.Content, find) {
			result.Warnings = append(result.Warnings, fmt.Sprintf("content: expected %q, got %q", find, msg.Content))
			return result
		}
	}

	if resp.Choice[0].FinishReason() == "tool" {
		if !strings.Contains(msg.ToolCalls[0].Function.Name, funct) {
			result.Warnings = append(result.Warnings, fmt.Sprintf("tooling: expected %q, got %q", funct, msg.ToolCalls[0].Function.Name))
			return result
		}

		if len(msg.ToolCalls[0].Function.Arguments) == 0 {
			result.Err = fmt.Errorf("tooling: expected arguments to be non-empty, got %v", msg.ToolCalls[0].Function.Arguments)
			return result
		}

		location, exists := msg.ToolCalls[0].Function.Arguments[arg]
		if !exists {
			result.Err = fmt.Errorf("tooling: expected an argument named %s", arg)
			return result
		}

		if !strings.Contains(strings.ToLower(location.(string)), find) {
			result.Err = fmt.Errorf("tooling: expected %q, got %q", find, location.(string))
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
	var result TestResult

	find = strings.ToLower(find)
	funct = strings.ToLower(funct)

	var toolCalls []model.ResponseToolCall
	if len(acc.ToolCalls) > 0 {
		toolCalls = acc.ToolCalls
	} else if len(lastResp.Choice) > 0 && lastResp.Choice[0].Message != nil {
		toolCalls = lastResp.Choice[0].Message.ToolCalls
	}

	if len(toolCalls) == 0 {
		result.Err = fmt.Errorf("streaming: expected tool calls, got none")
		return result
	}

	funcName := strings.ToLower(toolCalls[0].Function.Name)
	if !strings.Contains(funcName, funct) {
		result.Warnings = append(result.Warnings, fmt.Sprintf("streaming: expected function %q, got %q", funct, funcName))
		return result
	}

	if len(toolCalls[0].Function.Arguments) == 0 {
		result.Err = fmt.Errorf("streaming: expected arguments to be non-empty")
		return result
	}

	location, exists := toolCalls[0].Function.Arguments[arg]
	if !exists {
		result.Err = fmt.Errorf("streaming: expected an argument named %s", arg)
		return result
	}

	if !strings.Contains(strings.ToLower(location.(string)), find) {
		result.Err = fmt.Errorf("streaming: expected %q in %s, got %q", find, arg, location.(string))
		return result
	}

	return result
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

	if len(resp.Choice) == 0 {
		result.Err = fmt.Errorf("expected choice, got %d", len(resp.Choice))
		return result
	}

	msg := resp.Choice[0].Message
	if msg == nil {
		result.Err = fmt.Errorf("expected message to be non-nil")
		return result
	}

	content := strings.TrimSpace(msg.Content)
	if content == "" {
		result.Err = fmt.Errorf("expected content to be non-empty")
		return result
	}

	var js any
	if err := json.Unmarshal([]byte(content), &js); err != nil {
		result.Err = fmt.Errorf("grammar: expected valid JSON, got parse error: %w\ncontent: %s", err, content)
		return result
	}

	return result
}

// TestGrammarStreamingContent validates grammar-constrained streaming content.
func TestGrammarStreamingContent(content string, modelName string) TestResult {
	var result TestResult

	content = strings.TrimSpace(content)
	if content == "" {
		result.Err = fmt.Errorf("grammar streaming: expected content, got empty")
		return result
	}

	var js any
	if err := json.Unmarshal([]byte(content), &js); err != nil {
		result.Err = fmt.Errorf("grammar streaming: expected valid JSON, got parse error: %w\ncontent: %s", err, content)
		return result
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
		if len(resp.Choice) > 0 && resp.Choice[0].Delta != nil {
			reasoning.WriteString(resp.Choice[0].Delta.Reasoning)
			content.WriteString(resp.Choice[0].Delta.Content)
		}
	}

	if len(lastResp.Choice) > 0 && lastResp.Choice[0].FinishReason() == model.FinishReasonError {
		errMsg := ""
		if lastResp.Choice[0].Delta != nil {
			errMsg = lastResp.Choice[0].Delta.Content
		}
		return lastResp, content.String(), fmt.Errorf("model error: %s", errMsg)
	}

	if content.Len() == 0 && reasoning.Len() > 0 {
		return lastResp, reasoning.String(), nil
	}

	return lastResp, content.String(), nil
}
