package tuning

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type httpCompleter struct {
	endpoint string
	client   *http.Client
}

// NewHTTPCompleter talks to an OpenAI-compatible chat-completions endpoint
// supporting structured output. endpoint is the complete URL, including its
// path. A custom client/transport can attach credentials; they are not persisted
// in Temporal inputs. No browser, fineapp or model pool is required.
func NewHTTPCompleter(endpoint string, client *http.Client) (ModelCompleter, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return nil, fmt.Errorf("%w: a valid HTTP model endpoint without embedded credentials is required", ErrInvalidArgument)
	}
	if client == nil {
		client = http.DefaultClient
	}
	return httpCompleter{endpoint: endpoint, client: client}, nil
}

func (c httpCompleter) Complete(ctx context.Context, input CompletionRequest) (Completion, error) {
	body := struct {
		Model          string         `json:"model"`
		Messages       []Message      `json:"messages"`
		MaxTokens      int            `json:"max_tokens"`
		Seed           uint32         `json:"seed"`
		ResponseFormat map[string]any `json:"response_format"`
	}{input.Model, input.Messages, input.MaxTokens, input.Seed, map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "tuning_preparation", "strict": true, "schema": input.ResponseSchema}}}
	data, err := json.Marshal(body)
	if err != nil {
		return Completion{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(data))
	if err != nil {
		return Completion{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return Completion{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		// A provider response may echo source text or credentials. Do not expose it.
		return Completion{}, fmt.Errorf("model completion returned HTTP %d", response.StatusCode)
	}
	data, err = io.ReadAll(io.LimitReader(response.Body, (16<<20)+1))
	if err != nil {
		return Completion{}, err
	}
	if len(data) > 16<<20 {
		return Completion{}, fmt.Errorf("model completion exceeds response limit")
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return Completion{}, fmt.Errorf("decode model completion: %w", err)
	}
	if len(result.Choices) != 1 {
		return Completion{}, fmt.Errorf("model completion must contain exactly one choice")
	}
	completion := Completion{Content: result.Choices[0].Message.Content, PromptTokens: result.Usage.PromptTokens, CompletionTokens: result.Usage.CompletionTokens}
	if result.Choices[0].FinishReason == "length" {
		return completion, ErrCompletionTruncated
	}
	if result.Choices[0].FinishReason != "stop" {
		return Completion{}, fmt.Errorf("model completion did not finish successfully")
	}
	if completion.Content == "" {
		return Completion{}, fmt.Errorf("model returned an empty completion")
	}
	return completion, nil
}
