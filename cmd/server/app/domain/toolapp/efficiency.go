package toolapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// =============================================================================
// Requests
//
// The Efficiency app measures how fast a model runs a single prompt. The flow
// in the BUI:
//
//  1. Pick up to N models (GET /v1/kronk/models).
//  2. Give a prompt and a max_tokens cap.
//  3. Run each model (POST /v1/efficiency/run) — one model per call.
//
// Each call loads the model and performs an untimed warm-up inference before
// timing the real prompt, so the reported wallclock and tokens/sec reflect
// steady-state throughput and exclude model load and first-inference cost.

// EfficiencyRequest is the body for running a single model against a prompt and
// measuring its throughput.
type EfficiencyRequest struct {
	Model     string `json:"model"`
	Prompt    string `json:"prompt"`
	MaxTokens int    `json:"max_tokens"`
}

// defaultEfficiencyMaxTokens caps output length when the request omits it. A
// fixed cap normalizes the amount of work across models so wallclock and
// tokens/sec are comparable.
const defaultEfficiencyMaxTokens = 512

// =============================================================================
// Responses

// EfficiencyUsage reports throughput accounting for a single timed run. InTPS
// is derived from the prompt tokens and the time-to-first-token (prefill
// speed); OutTPS is the generation speed reported by the model. WallclockMS is
// generation time only — prefill (TTFTMS) is excluded.
type EfficiencyUsage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	InTPS            float64 `json:"in_tps"`
	OutTPS           float64 `json:"out_tps"`
	TTFTMS           float64 `json:"ttft_ms"`
	WallclockMS      float64 `json:"wallclock_ms"`
}

// EfficiencyResponse is the result of running a model against a prompt.
type EfficiencyResponse struct {
	Model  string          `json:"model"`
	Prompt string          `json:"prompt"`
	Output string          `json:"output"`
	Usage  EfficiencyUsage `json:"usage"`
	RanAt  int64           `json:"ran_at"`
}

// Encode implements the web.Encoder interface.
func (r EfficiencyResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(r)
	return data, "application/json", err
}

// =============================================================================
// Handlers

// runEfficiency loads the chosen model, performs an untimed warm-up, then times
// a single prompt and returns throughput metrics plus the model's output.
func (a *app) runEfficiency(ctx context.Context, r *http.Request) web.Encoder {
	var req EfficiencyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	switch {
	case strings.TrimSpace(req.Model) == "":
		return errs.Errorf(errs.InvalidArgument, "missing model field")
	case strings.TrimSpace(req.Prompt) == "":
		return errs.Errorf(errs.InvalidArgument, "missing prompt field")
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultEfficiencyMaxTokens
	}

	// Resolve the model's normal config and disable incremental caching so each
	// prompt is measured from a clean state (no KV reuse skewing the numbers).
	cfg, err := a.models.KronkResolvedConfig(req.Model, a.pool.Kronk.ModelConfig())
	if err != nil {
		return errs.New(errs.InvalidArgument, fmt.Errorf("resolving model config: %w", err))
	}

	imc := false
	cfg.PtrIncrementalCache = &imc

	// Acquire a dedicated instance with a stable per-model key so switching the
	// prompt and re-running reuses the loaded model rather than reloading it.
	krn, err := a.pool.Kronk.AquireCustom(ctx, req.Model+"/efficiency", cfg)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Warm-up: a tiny throwaway inference that warms kernels after the model is
	// loaded. This is NOT timed, so the measured run reflects steady-state
	// throughput rather than first-inference cost.
	warmup := model.D{
		"messages": []model.D{
			model.TextMessage(model.RoleUser, "hi"),
		},
		"max_tokens":      1,
		"enable_thinking": false,
	}
	if _, err := krn.Chat(ctx, warmup); err != nil {
		return errs.New(errs.Internal, fmt.Errorf("warm-up failed: %w", err))
	}

	messages := []model.D{
		model.TextMessage(model.RoleUser, req.Prompt),
	}

	d := model.D{
		"messages":        messages,
		"max_tokens":      maxTokens,
		"enable_thinking": false,
	}

	// Time only the real prompt.
	start := time.Now()
	resp, err := krn.Chat(ctx, d)
	wallclock := time.Since(start)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message == nil {
		return errs.Errorf(errs.Internal, "model returned no choices")
	}

	choice := resp.Choices[0]
	if choice.FinishReason() == model.FinishReasonError {
		return errs.Errorf(errs.Internal, "error from model: %s", choice.Message.Content)
	}

	out := EfficiencyResponse{
		Model:  req.Model,
		Prompt: req.Prompt,
		Output: strings.TrimSpace(choice.Message.Content),
		RanAt:  time.Now().UnixMilli(),
		Usage: EfficiencyUsage{
			WallclockMS: float64(wallclock.Microseconds()) / 1000,
		},
	}

	if resp.Usage != nil {
		out.Usage.PromptTokens = resp.Usage.PromptTokens
		out.Usage.CompletionTokens = resp.Usage.CompletionTokens
		out.Usage.OutTPS = resp.Usage.TokensPerSecond
		out.Usage.TTFTMS = resp.Usage.TimeToFirstTokenMS

		// Input tps = prompt tokens processed during prefill (time-to-first-token).
		if resp.Usage.TimeToFirstTokenMS > 0 {
			out.Usage.InTPS = float64(resp.Usage.PromptTokens) / (resp.Usage.TimeToFirstTokenMS / 1000)
		}

		// Exclude prefill from wall clock so it reports generation time only.
		// Prefill (time-to-first-token) scales with prompt length, which would
		// otherwise make wall clock depend on the prompt rather than decode speed.
		out.Usage.WallclockMS -= resp.Usage.TimeToFirstTokenMS
		if out.Usage.WallclockMS < 0 {
			out.Usage.WallclockMS = 0
		}
	}

	return out
}
