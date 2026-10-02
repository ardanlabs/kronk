package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var errStopStream = errors.New("stop event stream")

type apiClient struct {
	host    string
	token   string
	client  *http.Client
	traces  *traceRegistry
	timeout time.Duration
}

type httpError struct {
	Status int
	Body   string
}

func (err *httpError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", err.Status, err.Body)
}

type usage struct {
	PromptTokens        int     `json:"prompt_tokens"`
	CompletionTokens    int     `json:"completion_tokens"`
	DraftTokens         int     `json:"draft_tokens,omitempty"`
	DraftAcceptedTokens int     `json:"draft_accepted_tokens,omitempty"`
	DraftAcceptanceRate float64 `json:"draft_acceptance_rate,omitempty"`
	DraftCoverage       float64 `json:"draft_coverage,omitempty"`
	DraftDisableReason  string  `json:"draft_disable_reason,omitempty"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

type message struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type chatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message      message `json:"message"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage usage `json:"usage"`
}

type chatChunk struct {
	ID      string `json:"id"`
	Choices []struct {
		Delta        message `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage usage `json:"usage"`
}

type streamResult struct {
	TraceID           string    `json:"trace_id"`
	ResponseID        string    `json:"response_id,omitempty"`
	Content           string    `json:"-"`
	ContentCharacters int       `json:"content_characters"`
	ContentEvents     int       `json:"content_events"`
	StartedAt         time.Time `json:"-"`
	FirstContentAt    time.Time `json:"-"`
	FinishedAt        time.Time `json:"-"`
	FinishReason      string    `json:"finish_reason,omitempty"`
	Usage             usage     `json:"usage"`
}

func newAPIClient(host, token string, timeout time.Duration, traces *traceRegistry) *apiClient {
	return &apiClient{
		host:    host,
		token:   token,
		client:  &http.Client{},
		traces:  traces,
		timeout: timeout,
	}
}

func (client *apiClient) json(ctx context.Context, scenario, method, path string, body, target any) (string, error) {
	traceID, err := newTraceID()
	if err != nil {
		return "", err
	}
	client.traces.add(traceID, scenario)

	var source io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return traceID, fmt.Errorf("encode %s request: %w", path, err)
		}
		source = bytes.NewReader(data)
	}

	requestCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, method, client.host+path, source)
	if err != nil {
		return traceID, fmt.Errorf("create %s request: %w", path, err)
	}
	client.setHeaders(req, traceID, false)
	resp, err := client.client.Do(req)
	if err != nil {
		return traceID, fmt.Errorf("%s %s: %w", method, path, requestContextError(requestCtx, err))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return traceID, decodeHTTPError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return traceID, fmt.Errorf("decode %s response: %w", path, err)
	}
	return traceID, nil
}

func (client *apiClient) stream(ctx context.Context, scenario string, body any, visit func(chatChunk, time.Time) error) (streamResult, error) {
	traceID, err := newTraceID()
	if err != nil {
		return streamResult{}, err
	}
	client.traces.add(traceID, scenario)

	data, err := json.Marshal(body)
	if err != nil {
		return streamResult{}, fmt.Errorf("encode chat stream request: %w", err)
	}
	requestCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, client.host+"/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return streamResult{}, fmt.Errorf("create chat stream request: %w", err)
	}
	client.setHeaders(req, traceID, true)

	result := streamResult{TraceID: traceID, StartedAt: time.Now()}
	resp, err := client.client.Do(req)
	if err != nil {
		return result, fmt.Errorf("stream chat completion: %w", requestContextError(requestCtx, err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return result, decodeHTTPError(resp)
	}

	var content strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk chatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return result, fmt.Errorf("decode chat event: %w", err)
		}
		now := time.Now()
		if chunk.ID != "" {
			result.ResponseID = chunk.ID
		}
		if chunk.Usage.PromptTokens != 0 || chunk.Usage.CompletionTokens != 0 {
			result.Usage = chunk.Usage
		}
		if len(chunk.Choices) > 0 {
			if chunk.Choices[0].FinishReason != nil {
				result.FinishReason = *chunk.Choices[0].FinishReason
			}
			if piece := chunk.Choices[0].Delta.Content; piece != "" {
				if result.FirstContentAt.IsZero() {
					result.FirstContentAt = now
				}
				result.ContentEvents++
				content.WriteString(piece)
			}
		}
		if visit != nil {
			if err := visit(chunk, now); err != nil {
				result.Content = content.String()
				result.ContentCharacters = content.Len()
				result.FinishedAt = time.Now()
				if errors.Is(err, errStopStream) {
					cancel()
					return result, errStopStream
				}
				return result, err
			}
		}
	}
	result.Content = content.String()
	result.ContentCharacters = content.Len()
	result.FinishedAt = time.Now()
	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("read chat event stream: %w", requestContextError(requestCtx, err))
	}
	if ctxErr := requestCtx.Err(); ctxErr != nil {
		return result, ctxErr
	}
	return result, nil
}

func (client *apiClient) setHeaders(req *http.Request, traceID string, stream bool) {
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	spanID := traceID[:16]
	req.Header.Set("Traceparent", fmt.Sprintf("00-%s-%s-01", traceID, spanID))
	if client.token != "" {
		req.Header.Set("Authorization", "Bearer "+client.token)
	}
}

func decodeHTTPError(resp *http.Response) error {
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return fmt.Errorf("read HTTP %d error: %w", resp.StatusCode, err)
	}
	return &httpError{Status: resp.StatusCode, Body: strings.TrimSpace(string(data))}
}

func requestContextError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return err
}

func newTraceID() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf("generate trace ID: %w", err)
	}
	return hex.EncodeToString(data[:]), nil
}
