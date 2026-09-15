// Package tokenizer accesses the tokenizer embedded in a model loaded by
// llama-server.
package tokenizer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const maximumResponseBytes = 32 << 20

// Client returns exact token IDs for model-formatted training text.
type Client struct {
	endpoint   string
	httpClient *http.Client
}

// New constructs a tokenizer client for a llama-server endpoint.
func New(endpoint string, httpClient *http.Client) (*Client, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse tokenizer endpoint: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("parse tokenizer endpoint: endpoint must include a scheme and host")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{endpoint: strings.TrimRight(endpoint, "/"), httpClient: httpClient}, nil
}

// VerifyModel confirms that llama-server and GoMLX are using the same local
// GGUF file. Training must not proceed with token IDs from another model.
func (client *Client) VerifyModel(ctx context.Context, modelPath string) error {
	if client == nil || client.httpClient == nil {
		return fmt.Errorf("tokenizer client is not initialized")
	}
	if modelPath == "" {
		return fmt.Errorf("tokenizer model path is required")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.endpoint+"/props", nil)
	if err != nil {
		return fmt.Errorf("create tokenizer model request: %w", err)
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("inspect tokenizer model: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		return fmt.Errorf("inspect tokenizer model status %s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read tokenizer model response: %w", err)
	}
	if len(data) > maximumResponseBytes {
		return fmt.Errorf("decode tokenizer model response: response exceeds %d bytes", maximumResponseBytes)
	}
	var props struct {
		ModelPath string `json:"model_path"`
	}
	if err := json.Unmarshal(data, &props); err != nil {
		return fmt.Errorf("decode tokenizer model response: %w", err)
	}
	if props.ModelPath == "" {
		return fmt.Errorf("tokenizer model response contains no model path")
	}
	wanted, err := os.Stat(modelPath)
	if err != nil {
		return fmt.Errorf("inspect GoMLX model %q: %w", modelPath, err)
	}
	servedPath := props.ModelPath
	if !filepath.IsAbs(servedPath) {
		servedPath, err = filepath.Abs(servedPath)
		if err != nil {
			return fmt.Errorf("resolve tokenizer model %q: %w", props.ModelPath, err)
		}
	}
	served, err := os.Stat(servedPath)
	if err != nil {
		return fmt.Errorf("inspect tokenizer model %q: %w", props.ModelPath, err)
	}
	if !os.SameFile(wanted, served) {
		return fmt.Errorf("tokenizer model %q does not match GoMLX model %q", props.ModelPath, modelPath)
	}
	return nil
}

// Tokenize converts content containing model control tokens into IDs without
// adding another beginning- or end-of-sequence token.
func (client *Client) Tokenize(ctx context.Context, content string) ([]int32, error) {
	if client == nil || client.httpClient == nil {
		return nil, fmt.Errorf("tokenizer client is not initialized")
	}
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("tokenizer content is required")
	}
	payload, err := json.Marshal(struct {
		Content      string `json:"content"`
		AddSpecial   bool   `json:"add_special"`
		ParseSpecial bool   `json:"parse_special"`
	}{Content: content, AddSpecial: false, ParseSpecial: true})
	if err != nil {
		return nil, fmt.Errorf("encode tokenize request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint+"/tokenize", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create tokenize request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("tokenize request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		return nil, fmt.Errorf("tokenize response status %s: %s", response.Status, strings.TrimSpace(string(message)))
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read tokenize response: %w", err)
	}
	if len(data) > maximumResponseBytes {
		return nil, fmt.Errorf("decode tokenize response: response exceeds %d bytes", maximumResponseBytes)
	}
	var result struct {
		Tokens []int32 `json:"tokens"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode tokenize response: %w", err)
	}
	if len(result.Tokens) == 0 {
		return nil, fmt.Errorf("tokenize response contains no token IDs")
	}
	for index, token := range result.Tokens {
		if token < 0 {
			return nil, fmt.Errorf("tokenize response token[%d]=%d must not be negative", index, token)
		}
	}
	return result.Tokens, nil
}
