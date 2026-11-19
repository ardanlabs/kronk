package providers_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/providers"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/pool"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// TestKMSNativeSmoke is explicitly opt-in; run in a fresh test process:
//
//	GITHUB_WORKSPACE="$PWD" KRONK_TMPRL_NATIVE_SMOKE=1 \
//	KRONK_TMPRL_SMOKE_BASE=/path/to/existing/kronk-base \
//	KRONK_TMPRL_SMOKE_LIBS=/path/to/compatible/llama-libraries \
//	KRONK_TMPRL_SMOKE_MODEL=owner/small-chat-model \
//	go test -count=1 -run '^TestKMSNativeSmoke$' -timeout 3m ./cmd/server/foundation/tmprl/providers
//
// Prerequisites: compatible installed llama.cpp libraries, an existing indexed
// Kronk model catalog (models/.index.yaml), and a small single-file GGUF chat
// model supporting chat templates and JSON-schema constrained output. The model
// must fit available RAM on CPU with a 16K-token context. Nothing is downloaded,
// indexed, trained, or exported; supplied configuration errors fail, not skip.
// This exercises real KMS admission/loading, raw and templated tokenization,
// GoMLX preflight's private CPU pool entry, and KMSCompleter structured completion.
func TestKMSNativeSmoke(t *testing.T) {
	switch value := os.Getenv("KRONK_TMPRL_NATIVE_SMOKE"); value {
	case "":
		t.Skip("set KRONK_TMPRL_NATIVE_SMOKE=1 and the documented local prerequisites")
	case "1":
	default:
		t.Fatalf("KRONK_TMPRL_NATIVE_SMOKE: got %q, want 1 or unset", value)
	}
	base := os.Getenv("KRONK_TMPRL_SMOKE_BASE")
	libPath := os.Getenv("KRONK_TMPRL_SMOKE_LIBS")
	modelID := os.Getenv("KRONK_TMPRL_SMOKE_MODEL")
	if base == "" || libPath == "" || modelID == "" {
		t.Fatal("KRONK_TMPRL_SMOKE_BASE, KRONK_TMPRL_SMOKE_LIBS and KRONK_TMPRL_SMOKE_MODEL are required")
	}
	for _, path := range []string{base, libPath, filepath.Join(base, "models")} {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			t.Fatalf("local directory %q: got %v, %v, want existing directory", path, info, err)
		}
	}
	if _, err := os.Stat(filepath.Join(base, "models", ".index.yaml")); err != nil {
		t.Fatalf("existing model index: %v", err)
	}
	if _, err := models.ParseModelID(modelID); err != nil {
		t.Fatal(err)
	}
	if err := kronk.Init(kronk.WithLibPath(libPath)); err != nil {
		t.Fatalf("native init: %v", err)
	}
	mdls, err := models.NewWithPaths(base)
	if err != nil {
		t.Fatal(err)
	}
	hardware := devices.List()
	rm, err := resman.New(resman.Config{Snapshot: resman.FromDevices(hardware)})
	if err != nil {
		t.Fatal(err)
	}
	p, err := pool.New(pool.Config{
		Log: applog.DiscardLogger, Models: mdls, Resman: rm, StartupDevices: &hardware,
		ModelsInPool: 1,
		ModelConfig: map[string]models.ModelConfig{modelID: {
			PtrContextWindow: new(512), PtrNGpuLayers: new(-1), PtrNSeqMax: new(1),
			PtrIncrementalCache: new(true), PtrInsecureLogging: new(false),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := p.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown: got %v, want nil", err)
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	cfg, err := p.ResolvedKronkConfig(modelID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ModelFiles) != 1 {
		t.Fatalf("ModelFiles: got %v, want one GGUF", cfg.ModelFiles)
	}
	provider, err := providers.NewGoMLX(p)
	if err != nil {
		t.Fatal(err)
	}
	// Validate acquires and invalidates a private custom entry. Train/validation
	// use exact IDs, while protected-test preflight uses the chat template.
	err = provider.Validate(ctx, providers.TrainingJob{
		ModelID: modelID, Model: cfg.ModelFiles[0], PredictionTokens: 32,
		Dataset: providers.TrainingDataset{
			Train:      []providers.Record{{Prompt: "Say hello.", Target: "Hello."}},
			Validation: []providers.Record{{Prompt: "Say goodbye.", Target: "Goodbye."}},
			Test:       []providers.Record{{Prompt: "Say OK."}},
		},
	})
	if err != nil {
		t.Fatalf("pool-backed GoMLX tokenization preflight: %v", err)
	}
	krn, err := p.AquireModel(ctx, modelID)
	if err != nil {
		t.Fatal(err)
	}
	const prompt = "Say OK."
	ids, err := krn.TokenizeIDs(ctx, prompt)
	if err != nil || len(ids) == 0 {
		t.Fatalf("TokenizeIDs: got %v, %v, want nonempty IDs, nil", ids, err)
	}
	repeated, err := krn.TokenizeIDs(ctx, prompt)
	if err != nil || !slices.Equal(ids, repeated) {
		t.Fatalf("repeated TokenizeIDs: got %v, %v, want %v, nil", repeated, err, ids)
	}
	tokens, err := krn.Tokenize(ctx, model.D{"input": prompt, "apply_template": true})
	if err != nil || tokens.Tokens < 1 {
		t.Fatalf("templated Tokenize: got %+v, %v, want positive count, nil", tokens, err)
	}
	completer, err := providers.NewKMSCompleter(p)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := completer.Complete(ctx, providers.CompletionRequest{
		Model: modelID, Messages: []providers.Message{{Role: "user", Content: `Return {"status":"ok"}.`}},
		MaxTokens: 32, Seed: 53,
		ResponseSchema: map[string]any{
			"type": "object", "required": []string{"status"}, "additionalProperties": false,
			"properties": map[string]any{"status": map[string]any{"type": "string", "enum": []string{"ok"}}},
		},
	})
	if err != nil {
		t.Fatalf("pool-backed completion: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(completion.Content), &result); err != nil || len(result) != 1 || result["status"] != "ok" {
		t.Errorf("completion: got %q, %v, want object with only status=ok", completion.Content, err)
	}
	if completion.PromptTokens < 1 || completion.CompletionTokens < 1 || completion.CompletionTokens > 32 {
		t.Errorf("usage: got %+v, want positive prompt tokens and 1..32 completion tokens", completion)
	}
	prepared, ok := p.GetExisting(modelID + "/custom/preparation")
	if !ok {
		t.Fatal("preparation model: want a separate pooled entry")
	}
	if cfg := prepared.ModelConfig(); cfg.ContextWindow() != 16384 || cfg.IncrementalCache() {
		t.Errorf("preparation settings: got context=%d IMC=%t, want 16384 and false", cfg.ContextWindow(), cfg.IncrementalCache())
	}
	regular, err := p.AquireModel(ctx, modelID)
	if err != nil {
		t.Fatal(err)
	}
	if cfg := regular.ModelConfig(); cfg.ContextWindow() != 512 || !cfg.IncrementalCache() {
		t.Errorf("normal settings: got context=%d IMC=%t, want 512 and true", cfg.ContextWindow(), cfg.IncrementalCache())
	}
	t.Logf("native smoke: raw IDs=%d, templated tokens=%d, completion=%s, usage=%d/%d", len(ids), tokens.Tokens, completion.Content, completion.PromptTokens, completion.CompletionTokens)
}
