package prepare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type liveKronkCompleter struct {
	client   *http.Client
	endpoint string
}

func newLiveKronkCompleter() *liveKronkCompleter {
	return &liveKronkCompleter{
		client:   http.DefaultClient,
		endpoint: "http://localhost:11435",
	}
}

func (c *liveKronkCompleter) Complete(ctx context.Context, completion CompletionRequest) (Completion, error) {
	body := struct {
		Model               string         `json:"model"`
		Messages            []Message      `json:"messages"`
		MaxCompletionTokens int            `json:"max_completion_tokens"`
		Temperature         float64        `json:"temperature"`
		TopP                float64        `json:"top_p"`
		Seed                uint32         `json:"seed"`
		EnableThinking      bool           `json:"enable_thinking"`
		ResponseFormat      map[string]any `json:"response_format"`
	}{
		Model:               completion.Model,
		Messages:            completion.Messages,
		MaxCompletionTokens: completion.MaxTokens,
		Temperature:         0.2,
		TopP:                0.9,
		Seed:                completion.Seed,
		ResponseFormat: map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "moxie_training_records",
				"strict": true,
				"schema": completion.ResponseSchema,
			},
		},
	}

	data, err := json.Marshal(body)
	if err != nil {
		return Completion{}, fmt.Errorf("encode completion request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return Completion{}, fmt.Errorf("create completion request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return Completion{}, fmt.Errorf("call Kronk: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		return Completion{}, fmt.Errorf("call Kronk: server returned %s: %s", resp.Status, strings.TrimSpace(string(message)))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Completion{}, fmt.Errorf("decode completion response: %w", err)
	}
	if len(result.Choices) == 0 {
		return Completion{}, fmt.Errorf("complete: Kronk returned no choices")
	}

	content := strings.TrimSpace(result.Choices[0].Message.Content)
	if content == "" {
		return Completion{}, fmt.Errorf("complete: Kronk returned empty content")
	}

	return Completion{
		Content:          content,
		PromptTokens:     result.Usage.PromptTokens,
		CompletionTokens: result.Usage.CompletionTokens,
	}, nil
}
