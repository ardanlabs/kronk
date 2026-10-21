package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type usage struct {
	PromptTokens        int     `json:"prompt_tokens"`
	CompletionTokens    int     `json:"completion_tokens"`
	DraftTokens         int     `json:"draft_tokens"`
	DraftAcceptedTokens int     `json:"draft_accepted_tokens"`
	DraftCoverage       float64 `json:"draft_coverage"`
	DraftDisableReason  string  `json:"draft_disable_reason"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	ServerTPS  float64 `json:"tokens_per_second"`
	ServerTTFT float64 `json:"time_to_first_token_ms"`
}

type result struct {
	Phase           string             `json:"phase"`
	TraceID         string             `json:"trace_id"`
	Content         string             `json:"content"`
	FinishReason    string             `json:"finish_reason"`
	Usage           usage              `json:"usage"`
	Started         time.Time          `json:"started_at"`
	Elapsed         float64            `json:"elapsed_seconds"`
	FirstContent    float64            `json:"first_content_seconds"`
	Generation      float64            `json:"generation_seconds"`
	OutputTPS       float64            `json:"client_output_tokens_per_second"`
	Cancelled       bool               `json:"cancelled"`
	Error           string             `json:"error,omitempty"`
	CacheMatch      string             `json:"cache_match,omitempty"`
	CacheReason     string             `json:"cache_reason,omitempty"`
	ReusableTokens  int                `json:"reusable_tokens,omitempty"`
	StableTokens    int                `json:"stable_tokens,omitempty"`
	ExtensionTokens int                `json:"extension_tokens,omitempty"`
	TailTokens      int                `json:"tail_tokens,omitempty"`
	SystemBoundary  int                `json:"system_boundary_tokens,omitempty"`
	Execution       execution          `json:"execution"`
	ServerTimings   map[string]float64 `json:"server_timings"`
	StartupTimings  map[string]float64 `json:"startup_timings,omitempty"`
}

// Execution evidence comes from this request's trace, not a server-wide mode.
type execution struct {
	Slot              *int      `json:"slot"`
	SlotConflict      bool      `json:"slot_conflict"`
	Started           time.Time `json:"started_at"`
	Finished          time.Time `json:"finished_at"`
	TerminalStatus    string    `json:"terminal_status"`
	SlotFinished      bool      `json:"slot_finished"`
	SequenceCleared   bool      `json:"sequence_cleared"`
	DraftCleaned      bool      `json:"draft_cleaned"`
	TargetRestored    bool      `json:"target_restored"`
	DraftRestored     bool      `json:"draft_restored"`
	TargetSnapshotted bool      `json:"target_snapshotted"`
	DraftSnapshotted  bool      `json:"draft_snapshotted"`
	SystemPublished   int       `json:"system_published_tokens"`
	ReusedTokens      int       `json:"actual_reused_tokens"`
	OutputTokens      int       `json:"output_tokens"`
	DraftTokens       int       `json:"draft_tokens"`
	DraftAccepted     int       `json:"draft_accepted_tokens"`
	DraftResume       string    `json:"draft_resume_source"`
	ExpectedErrors    int       `json:"expected_cancel_errors"`
}

type client struct {
	host, token string
	http        http.Client
	timeout     time.Duration
	traces      map[string]string
	events      *json.Encoder
	repetition  int
}

func (c *client) request(ctx context.Context, method, path string, body any) (*http.Response, string, error) {
	var input io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, "", err
		}
		input = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.host+path, input)
	if err != nil {
		return nil, "", err
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, "", err
	}
	trace := hex.EncodeToString(id[:])
	req.Header.Set("Traceparent", "00-"+trace+"-"+trace[:16]+"-01")
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, trace, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		text, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return nil, trace, fmt.Errorf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, text)
	}
	return resp, trace, nil
}

func (c *client) json(method, path string, body, dst any) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	resp, _, err := c.request(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(dst)
}

func (c *client) chat(phase string, messages []message, limit int, cancelAfterContent bool) (r result, err error) {
	r.Phase, r.Started = phase, time.Now().UTC()
	defer func() {
		r.Elapsed = time.Since(r.Started).Seconds()
		if r.FirstContent > 0 {
			r.Generation = r.Elapsed - r.FirstContent
			if r.Generation > 0 && !r.Cancelled {
				r.OutputTPS = float64(r.Usage.CompletionTokens) / r.Generation
			}
		}
		if err != nil {
			r.Error = err.Error()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	body := map[string]any{
		"model": modelID, "messages": messages, "stream": true,
		"stream_options": map[string]bool{"include_usage": true},
		"temperature":    0, "seed": 42, "enable_thinking": false, "max_tokens": limit,
	}
	if c.events != nil {
		if err := c.events.Encode(map[string]any{"source": "request", "repetition": c.repetition, "phase": phase, "body": body}); err != nil {
			return r, err
		}
	}
	resp, trace, err := c.request(ctx, "POST", "/v1/chat/completions", body)
	r.TraceID = trace
	c.traces[trace] = phase
	if err != nil {
		return r, err
	}
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 65536), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			if r.FinishReason == "" || r.Usage.CompletionTokens == 0 {
				return r, fmt.Errorf("stream ended without finish reason or final usage")
			}
			return r, nil
		}
		var chunk struct {
			Choices []struct {
				Delta        message `json:"delta"`
				FinishReason string  `json:"finish_reason"`
			} `json:"choices"`
			Usage usage           `json:"usage"`
			Error json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return r, fmt.Errorf("invalid SSE JSON: %w", err)
		}
		if len(chunk.Error) > 0 && string(chunk.Error) != "null" {
			return r, fmt.Errorf("stream error: %s", chunk.Error)
		}
		if c.events != nil {
			if err := c.events.Encode(map[string]any{"source": "client", "repetition": c.repetition, "phase": phase, "trace_id": trace, "at": time.Now().UTC(), "chunk": json.RawMessage(data)}); err != nil {
				return r, err
			}
		}
		if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
			r.Usage = chunk.Usage
		}
		for _, choice := range chunk.Choices {
			if choice.FinishReason != "" {
				r.FinishReason = choice.FinishReason
			}
			if choice.Delta.Content != "" {
				if r.FirstContent == 0 {
					r.FirstContent = time.Since(r.Started).Seconds()
				}
				r.Content += choice.Delta.Content
				if cancelAfterContent {
					r.Cancelled = true
					cancel()
					return r, nil
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return r, err
	}
	return r, fmt.Errorf("truncated stream: missing [DONE]")
}
