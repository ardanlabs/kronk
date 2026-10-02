package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

type loadedModel struct {
	ID            string `json:"id"`
	Slots         int    `json:"slots"`
	HasProjection *bool  `json:"has_projection,omitempty"`
	ModelFamily   string `json:"model_family,omitempty"`
}

type slotDiagnostic struct {
	ID        int    `json:"id"`
	Phase     string `json:"phase"`
	RequestID string `json:"request_id"`
}

type engineDiagnostic struct {
	ModelID string           `json:"model_id"`
	MTP     bool             `json:"mtp"`
	NDraft  int              `json:"ndraft"`
	Slots   []slotDiagnostic `json:"slots"`
}

type completionResult struct {
	TraceID      string  `json:"trace_id"`
	ResponseID   string  `json:"response_id,omitempty"`
	Content      string  `json:"-"`
	Characters   int     `json:"characters"`
	FinishReason string  `json:"finish_reason,omitempty"`
	Elapsed      float64 `json:"elapsed_seconds"`
	Usage        usage   `json:"usage"`
}

func (rc *runContext) completion(model string, messages []map[string]any, maxTokens int) (completionResult, error) {
	body := map[string]any{
		"model":           model,
		"stream":          false,
		"temperature":     0,
		"seed":            rc.cfg.Seed,
		"max_tokens":      maxTokens,
		"enable_thinking": false,
		"messages":        messages,
	}
	started := time.Now()
	var response chatResponse
	traceID, err := rc.client.json(rc, rc.scenario, http.MethodPost, "/v1/chat/completions", body, &response)
	if err != nil {
		return completionResult{TraceID: traceID, Elapsed: seconds(time.Since(started))}, err
	}
	if len(response.Choices) == 0 {
		return completionResult{TraceID: traceID, ResponseID: response.ID, Elapsed: seconds(time.Since(started))}, fmt.Errorf("chat completion returned no choices")
	}
	result := completionResult{
		TraceID:    traceID,
		ResponseID: response.ID,
		Content:    response.Choices[0].Message.Content,
		Characters: len(response.Choices[0].Message.Content),
		Elapsed:    seconds(time.Since(started)),
		Usage:      response.Usage,
	}
	if response.Choices[0].FinishReason != nil {
		result.FinishReason = *response.Choices[0].FinishReason
	}
	return result, nil
}

func (rc *runContext) tokenCount(model, text string, applyTemplate bool) (int, error) {
	body := map[string]any{"model": model, "input": text, "apply_template": applyTemplate}
	var response struct {
		Tokens int `json:"tokens"`
	}
	_, err := rc.client.json(rc, rc.scenario, http.MethodPost, "/v1/tokenize", body, &response)
	if err != nil {
		return 0, err
	}
	if response.Tokens <= 0 {
		return 0, fmt.Errorf("tokenizer returned invalid token count %d", response.Tokens)
	}
	return response.Tokens, nil
}

func (rc *runContext) ensureModel(model string) (loadedModel, error) {
	loaded, err := rc.loadedModel(model)
	if err != nil {
		return loadedModel{}, err
	}
	if loaded != nil {
		return *loaded, nil
	}
	rc.artifacts.logf("model=%s is not loaded; loading it now", model)
	if _, err := rc.completion(model, []map[string]any{{"role": "user", "content": "Reply OK."}}, 8); err != nil {
		return loadedModel{}, fmt.Errorf("lazy-load model: %w", err)
	}
	loaded, err = rc.loadedModel(model)
	if err != nil {
		return loadedModel{}, err
	}
	if loaded == nil {
		return loadedModel{}, fmt.Errorf("model did not appear after loading: %s", model)
	}
	return *loaded, nil
}

func (rc *runContext) loadedModel(model string) (*loadedModel, error) {
	var models []loadedModel
	if _, err := rc.client.json(rc, rc.scenario, http.MethodGet, "/v1/kronk/models/ps", nil, &models); err != nil {
		return nil, err
	}
	for _, candidate := range models {
		if modelMatches(candidate.ID, model) {
			return &candidate, nil
		}
	}
	return nil, nil
}

func (rc *runContext) engine(model string) (*engineDiagnostic, error) {
	var engines []engineDiagnostic
	if _, err := rc.client.json(rc, rc.scenario, http.MethodGet, "/v1/kronk/models/slots", nil, &engines); err != nil {
		return nil, err
	}
	for _, candidate := range engines {
		if modelMatches(candidate.ModelID, model) {
			return &candidate, nil
		}
	}
	return nil, nil
}

func modelMatches(left, right string) bool {
	return left == right || strings.HasSuffix(left, right) || strings.HasSuffix(right, left)
}

func appendFailure(failures *[]string, format string, args ...any) {
	*failures = append(*failures, fmt.Sprintf(format, args...))
}
