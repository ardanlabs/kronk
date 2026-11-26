package model

import (
	"maps"
	"path/filepath"
	"strings"
	"testing"

	internalspec "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestDSparkSelectionCompatibility(t *testing.T) {
	targetMetadata := map[string]any{
		"general.architecture": "inkling2_loop", "inkling2_loop.embedding_length": uint32(2304),
		"inkling2_loop.block_count":          uint32(24),
		"inkling2_loop.attention.head_count": uint32(8), "inkling2_loop.attention.head_count_kv": uint32(2),
		"inkling2_loop.attention.key_length": uint32(128), "inkling2_loop.attention.value_length": uint32(128),
		"inkling2_loop.attention.sliding_window":         uint32(128),
		"inkling2_loop.attention.sliding_window_pattern": "[" + strings.Repeat("false ", 24) + "]",
		"inkling2_loop.shortconv.l_cache":                uint32(3), "inkling2_loop.attn_res.block_size": uint32(6),
		"inkling2_loop.loop.start": uint32(6), "inkling2_loop.loop.end": uint32(18),
		"inkling2_loop.loop.count": uint32(3), "inkling2_loop.loop.gate_rank": uint32(64),
		"tokenizer.ggml.model": "bpe", "tokenizer.ggml.tokens": "[alpha beta gamma]",
	}
	draftMetadata := map[string]any{
		"general.architecture": "dflash", "dflash.embedding_length": uint32(2304),
		"dflash.block_size": uint32(8), "dflash.target_layers": "[5 23 41 48]",
		"dflash.sample_from_anchor": "false", "dflash.has_markov_context": "true",
		"dflash.has_confidence_head": "false",
		"tokenizer.ggml.model":       "bpe", "tokenizer.ggml.tokens": "[alpha beta gamma]",
		"tokenizer.ggml.mask_token_id": uint32(0),
	}
	for _, tt := range []struct {
		name, key          string
		value              any
		nDraft, wantNDraft int
		wantErr            string
	}{
		{"terminal layer is valid", "", nil, 0, 7, ""},
		{"override below trained limit", "", nil, 2, 2, ""},
		{"override capped at trained limit", "", nil, 12, 7, ""},
		{"trained limit is metadata driven", "dflash.block_size", uint32(5), 12, 4, ""},
		{"not an MTP head", "general.architecture", "qwen35", 0, 0, "unsupported target/DSpark architectures"},
		{"width mismatch", "dflash.embedding_length", uint32(2048), 0, 0, "does not match target width"},
		{"token order mismatch", "tokenizer.ggml.tokens", "[beta alpha gamma]", 0, 0, "tokenizer metadata"},
		{"missing tokenizer", "tokenizer.ggml.tokens", nil, 0, 0, "tokenizer metadata"},
		{"missing mask", "tokenizer.ggml.mask_token_id", nil, 0, 0, "mask token ID"},
		{"out of range layer", "dflash.target_layers", "[5 49]", 0, 0, "invalid or duplicate"},
		{"duplicate layer", "dflash.target_layers", "[5 23 5]", 0, 0, "invalid or duplicate"},
		{"negative layer", "dflash.target_layers", "[-1 5]", 0, 0, "invalid or duplicate"},
		{"empty layers", "dflash.target_layers", "[]", 0, 0, "DSpark target layers"},
		{"causal graph unsupported", "dflash.attention.causal", "true", 0, 0, "noncausal"},
		{"anchor sampling unsupported", "dflash.sample_from_anchor", "true", 0, 0, "sample_from_anchor"},
		{"confidence graph unsupported", "dflash.has_confidence_head", "true", 0, 0, "has_confidence_head"},
		{"missing Markov context", "dflash.has_markov_context", nil, 0, 0, "has_markov_context"},
		{"anchor only block", "dflash.block_size", uint32(1), 0, 0, "trained block size"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			target, draft := filepath.Join(dir, "target.gguf"), filepath.Join(dir, "companion.gguf")
			metadata := maps.Clone(draftMetadata)
			if tt.key != "" {
				metadata[tt.key] = tt.value
				if tt.value == nil {
					delete(metadata, tt.key)
				}
			}
			writeTestGGUF(t, target, targetMetadata)
			writeTestGGUF(t, draft, metadata)
			cfg := NewConfig(WithModelFiles([]string{target}), WithDrafterFile(draft))
			if tt.nDraft > 0 {
				cfg.PtrDraftModel = &DraftModelConfig{NDraft: tt.nDraft}
			}
			plan, err := resolveSpeculationPlan(t.Context(), nil, cfg)
			wantSource := internalspec.SourceDSpark
			if tt.wantErr != "" {
				wantSource = internalspec.SourceNone
			}
			if err != nil || plan.Source != wantSource || plan.Active() || plan.LoadMTP || plan.RowsPerSequence() != 1 {
				t.Fatalf("auto plan = %+v, %v, want inactive %s", plan, err, wantSource)
			}
			if tt.wantErr == "" && plan.NDraft != tt.wantNDraft {
				t.Errorf("NDraft = %d, want %d", plan.NDraft, tt.wantNDraft)
			}
			if plan.Source == internalspec.SourceDSpark {
				draft, err := selectAndLoadDraft(t.Context(), nil, cfg, 0, 0, llama.ContextParams{}, plan)
				if err != nil || draft != nil {
					t.Fatalf("inactive DSpark loaded a drafter: %v, %v", draft, err)
				}
			}
			cfg.Speculation = SpeculationDSpark
			_, err = NewModel(t.Context(), cfg)
			wantErr := tt.wantErr
			if wantErr == "" {
				wantErr = "DSpark execution is not yet available"
			}
			if err == nil || !strings.Contains(err.Error(), wantErr) {
				t.Errorf("explicit DSpark error = %v, want containing %q", err, wantErr)
			}
		})
	}
}

func TestDSparkMissingAndUnusedCompanions(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target.gguf")
	writeTestGGUF(t, target, map[string]any{"general.architecture": "llama"})
	cfg := NewConfig(WithModelFiles([]string{target}), WithDrafterFile(filepath.Join(t.TempDir(), "missing.gguf")))
	for _, mode := range []SpeculationMode{SpeculationAuto, SpeculationDisabled, SpeculationMTP, SpeculationDSpark, SpeculationClassic} {
		cfg.Speculation = mode
		if mode == SpeculationClassic {
			cfg.PtrDraftModel = &DraftModelConfig{ModelFiles: []string{"classic.gguf"}}
		}
		plan, err := resolveSpeculationPlan(t.Context(), nil, cfg)
		switch mode {
		case SpeculationDSpark:
			if err == nil || !strings.Contains(err.Error(), "missing.gguf") {
				t.Fatalf("missing DSpark error = %v", err)
			}
		case SpeculationMTP:
			if err == nil || !strings.Contains(err.Error(), "no companion or embedded MTP") {
				t.Fatalf("MTP error = %v", err)
			}
		default:
			if err != nil || plan.Source == internalspec.SourceDSpark {
				t.Fatalf("%s plan = %+v, %v", mode, plan, err)
			}
		}
	}
	cfg.Speculation = SpeculationDSpark
	if _, err := resolveSpeculationPlan(t.Context(), nil, cfg); err == nil || !strings.Contains(err.Error(), "cannot use a separate draft model") {
		t.Fatalf("DSpark/classic conflict error = %v", err)
	}
}
