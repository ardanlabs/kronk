package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
	ModelID          string           `json:"model_id"`
	PrefillBatchSize int              `json:"prefill_batch_size"`
	NBatch           int              `json:"nbatch"`
	NUBatch          int              `json:"nubatch"`
	MTP              bool             `json:"mtp"`
	NDraft           int              `json:"ndraft"`
	Slots            []slotDiagnostic `json:"slots"`
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
	return result, rc.checkUsage(result.Usage)
}

// Completed requests carry final usage; cancelled streams do not. Short exact
// marker replies may finish before a draft proposal, so require drafting only
// above eight completion tokens, but reject disablement at any length.
func (rc *runContext) checkUsage(u usage) error {
	if u.PromptTokens <= 0 || u.CompletionTokens <= 0 {
		return fmt.Errorf("completed request lacks positive prompt/completion usage")
	}
	if rc.cfg.IMC == "off" && u.PromptTokensDetails.CachedTokens != 0 {
		return fmt.Errorf("IMC off: reported %d cached tokens", u.PromptTokensDetails.CachedTokens)
	}
	if rc.cfg.MTP == "off" {
		if u.DraftTokens != 0 || u.DraftAcceptedTokens != 0 || u.DraftCoverage != 0 {
			return fmt.Errorf("MTP off: reported drafting: tokens=%d accepted=%d coverage=%g", u.DraftTokens, u.DraftAcceptedTokens, u.DraftCoverage)
		}
		return nil
	}
	if u.DraftDisableReason != "" {
		return fmt.Errorf("MTP on: request disabled drafting: %s", u.DraftDisableReason)
	}
	if u.CompletionTokens > 8 && (u.DraftTokens <= 0 || u.DraftCoverage <= 0) {
		return fmt.Errorf("MTP on: %d completion tokens without positive drafting and coverage", u.CompletionTokens)
	}
	return nil
}

func (rc *runContext) stream(ctx context.Context, body any, visit func(chatChunk, time.Time) error) (streamResult, error) {
	result, err := rc.client.stream(ctx, rc.scenario, body, visit)
	if err == nil {
		err = rc.checkUsage(result.Usage)
	}
	return result, err
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
	if previous := rc.client.managedModel; previous != "" && previous != model {
		if err := rc.unloadModel(previous, "model-switch"); err != nil {
			return loadedModel{}, err
		}
	}
	rc.client.managedModel = model
	loaded, err := rc.loadedModel(model)
	if err != nil {
		return loadedModel{}, err
	}
	if loaded != nil {
		return *loaded, rc.checkModelModes(model)
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
	return *loaded, rc.checkModelModes(model)
}

func (rc *runContext) checkModelModes(model string) error {
	var raw json.RawMessage
	var detail struct {
		ModelConfig struct {
			IMC *bool `json:"incremental-cache"`
		} `json:"model_config"`
	}
	if _, err := rc.client.json(rc, rc.scenario, http.MethodGet, "/v1/kronk/models/"+url.PathEscape(model), nil, &raw); err != nil {
		return fmt.Errorf("verify IMC configuration: %w", err)
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		return fmt.Errorf("decode model configuration: %w", err)
	}
	if rc.modelConfigurations == nil {
		rc.modelConfigurations = make(map[string]any)
	}
	rc.modelConfigurations[model] = raw
	engine, err := rc.engine(model)
	if err != nil {
		return err
	}
	if engine == nil || detail.ModelConfig.IMC == nil {
		return fmt.Errorf("cannot verify MTP/IMC modes for %s", model)
	}
	rc.modelConfigurations[model] = map[string]any{"detail": raw, "scheduler": engine}
	rc.observedModes[model] = map[string]bool{"mtp": engine.MTP, "imc": *detail.ModelConfig.IMC}
	if engine.MTP != (rc.cfg.MTP == "on") || *detail.ModelConfig.IMC != (rc.cfg.IMC == "on") {
		return fmt.Errorf("%s: observed MTP=%t IMC=%t, expected MTP=%s IMC=%s", model, engine.MTP, *detail.ModelConfig.IMC, rc.cfg.MTP, rc.cfg.IMC)
	}
	return nil
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
	return left == right
}

func appendFailure(failures *[]string, format string, args ...any) {
	*failures = append(*failures, fmt.Sprintf(format, args...))
}

type modelUnloadResult struct {
	Repetition int    `json:"repetition"`
	Cell       string `json:"cell"`
	Scenario   string `json:"scenario"`
	Phase      string `json:"phase"`
	Model      string `json:"model"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
}

// Only explicitly selected suite IDs reach this function. Never unload all
// models or cancel another client's work to make an unload succeed.
func (rc *runContext) unloadModel(id, phase string) (err error) {
	loaded, err := rc.loadedModel(id)
	if err == nil && loaded == nil {
		if rc.client.managedModel == id {
			rc.client.managedModel = ""
		}
		return nil
	}
	result := modelUnloadResult{Cell: (modeCell{MTP: rc.cfg.MTP, IMC: rc.cfg.IMC}).id(), Scenario: rc.scenario, Phase: phase, Model: id, Status: "PASS"}
	result.Repetition = rc.repetition
	if rc.scenario == "setup" {
		result.Cell = "setup"
	}
	defer func() {
		if err != nil {
			result.Status, result.Error = "FAIL", err.Error()
			rc.client.cleanupFailed = true
		}
		rc.client.unloads = append(rc.client.unloads, result)
		rc.artifacts.logf("model unload: cell=%s phase=%s model=%s status=%s error=%s", result.Cell, phase, id, result.Status, result.Error)
	}()
	if err != nil {
		return fmt.Errorf("inspect model before unload %s: %w", id, err)
	}
	waitCtx, cancel := context.WithTimeout(rc, min(rc.cfg.Timeout, 15*time.Second))
	defer cancel()
	waitRC := *rc
	waitRC.Context = waitCtx
	for {
		engine, pollErr := waitRC.engine(id)
		if pollErr != nil {
			return fmt.Errorf("wait for idle model %s: %w", id, pollErr)
		}
		if engine == nil || len(engine.Slots) == 0 {
			return fmt.Errorf("cannot verify idle model %s before unload", id)
		}
		idle := true
		for _, slot := range engine.Slots {
			idle = idle && slot.Phase == "idle"
		}
		if idle {
			break
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("model %s still active before unload: %w", id, waitCtx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
	var response struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if _, err = rc.client.json(rc, rc.scenario, http.MethodPost, "/v1/kronk/models/unload", map[string]string{"id": id}, &response); err != nil {
		return fmt.Errorf("unload %s: %w", id, err)
	}
	if response.ID != id || response.Status != "unloaded" {
		return fmt.Errorf("unexpected unload response for %s: %+v", id, response)
	}
	loaded, err = rc.loadedModel(id)
	if err != nil {
		return fmt.Errorf("verify unload %s: %w", id, err)
	}
	if loaded != nil {
		return fmt.Errorf("model %s remains loaded after unload", id)
	}
	if rc.client.managedModel == id {
		rc.client.managedModel = ""
	}
	return nil
}
