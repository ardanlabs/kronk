package model

import (
	"context"
	"strings"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// =============================================================================
// Flash Attention configuration path: Kronk config -> llama.ContextParams ->
// llama_context_params.flash_attn_type.
//
// GROUND TRUTH (.extras/llama.cpp @ b10211)
//
//	include/llama.h:190-194
//	    enum llama_flash_attn_type {
//	        LLAMA_FLASH_ATTN_TYPE_AUTO     = -1,
//	        LLAMA_FLASH_ATTN_TYPE_DISABLED =  0,
//	        LLAMA_FLASH_ATTN_TYPE_ENABLED  =  1,
//	    };
//
//	include/llama.h:364                   llama_context_params.flash_attn_type
//	src/llama-context.cpp:3493            default = LLAMA_FLASH_ATTN_TYPE_AUTO
//	common/common.h:496                   CLI default = LLAMA_FLASH_ATTN_TYPE_AUTO
//	common/arg.cpp:1666-1680              -fa on|off|auto
//
// yzma mirrors the C values exactly (.extras/yzma/pkg/llama/llama.go:163-165)
// and its ContextParams field order matches llama_context_params field for
// field, so nothing is lost in the FFI hand-off. Kronk, however, declares its
// OWN int32 enum with DIFFERENT numbers (config.go:1183-1185):
//
//	FlashAttentionEnabled  = 0   // == LLAMA_FLASH_ATTN_TYPE_DISABLED
//	FlashAttentionDisabled = 1   // == LLAMA_FLASH_ATTN_TYPE_ENABLED
//	FlashAttentionAuto     = 2   // not a valid llama_flash_attn_type at all
//
// Every value therefore MUST pass through the translation switch at
// config.go:873-882. Today exactly one call site writes
// ctxParams.FlashAttentionType and it does go through that switch, so there is
// no live mistranslation to pin here — the two enums are audited as equivalent
// only through that switch, which is what the mirror below reproduces.
// =============================================================================

// resolveFlashAttentionType is a VERBATIM MIRROR of the translation switch that
// turns Kronk's FlashAttentionType into the llama.cpp enum value handed to
// llama_init_from_model:
//
//	sdk/kronk/model/config.go:873-882
//		switch cfg.FlashAttention() {
//		case FlashAttentionDisabled:
//			ctxParams.FlashAttentionType = llama.FlashAttentionTypeDisabled
//
//		case FlashAttentionAuto:
//			ctxParams.FlashAttentionType = llama.FlashAttentionTypeAuto
//
//		default:
//			ctxParams.FlashAttentionType = llama.FlashAttentionTypeEnabled
//		}
//
// The switch lives inside modelCtxParams (sdk/kronk/model/config.go:842), which
// opens with llama.ContextDefaultParams() — a raw FFI call with no nil-handle
// guard (.extras/yzma/pkg/llama/context.go) that panics without a prior dlopen
// of the llama shared library. That is why the switch is mirrored instead of
// called; see the same reasoning in mtp_ctxparams_source_test.go.
//
// MAINTAINER: when config.go:873-882 changes, update this mirror in the same
// commit so it keeps tracking the production mapping.
func resolveFlashAttentionType(cfg Config) llama.FlashAttentionType {
	switch cfg.FlashAttention() {
	case FlashAttentionDisabled:
		return llama.FlashAttentionTypeDisabled

	case FlashAttentionAuto:
		return llama.FlashAttentionTypeAuto

	default:
		return llama.FlashAttentionTypeEnabled
	}
}

// TestFlashAttentionUnsetResolvesToLlamaAuto pins the Flash Attention default
// divergence between Kronk and llama.cpp.
//
// FINDING
// An unset PtrFlashAttention resolves to FlashAttentionEnabled, so Kronk hands
// llama.cpp a FORCED LLAMA_FLASH_ATTN_TYPE_ENABLED (1) on every load that does
// not name a mode explicitly. llama.cpp's own default — for the library, for
// common/, and for llama-server — is LLAMA_FLASH_ATTN_TYPE_AUTO (-1).
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/config.go:1255-1259  DerefFlashAttention returns
//     FlashAttentionEnabled for a nil pointer.
//   - sdk/kronk/model/config.go:441-443    Config.FlashAttention() delegates to it.
//   - sdk/kronk/model/config.go:880-881    the switch's default arm turns that
//     into llama.FlashAttentionTypeEnabled.
//   - sdk/kronk/model/config.go:183-187    the doc comment that documents the
//     divergence as intentional ("When nil, FlashAttentionEnabled is used").
//
// LLAMA.CPP REFERENCE
//   - .extras/llama.cpp/src/llama-context.cpp:3493 — llama_context_default_params()
//     sets flash_attn_type = LLAMA_FLASH_ATTN_TYPE_AUTO.
//   - .extras/llama.cpp/common/common.h:496 — the CLI/server default is AUTO too.
//   - .extras/llama.cpp/src/llama-context.cpp:246-247
//     cparams.flash_attn = params.flash_attn_type != DISABLED;
//     cparams.auto_fa    = params.flash_attn_type == AUTO;
//   - .extras/llama.cpp/src/llama-context.cpp:549-552 — the Flash Attention
//     capability probe runs ONLY under `if (cparams.auto_fa)`.
//
// FAILURE SCENARIO
// AUTO makes llama.cpp reserve a probe graph and compare the device the fused
// FLASH_ATTN node landed on against the device that owns the layer
// (llama-context.cpp:520-547). On a mismatch — "usually due to missing support"
// — it logs "Flash Attention not supported, set to disabled" and rebuilds
// without FA. A forced ENABLED sets auto_fa = false, so that probe never runs:
// cparams.flash_attn stays true unconditionally and the graph keeps its
// ggml_flash_attn_ext node (llama-graph.cpp:2418-2441). On a backend/head-dim
// combination the device cannot honour there is no fallback — the node is
// scheduled onto whichever backend accepts it (typically CPU) while the layer's
// K/V live on the accelerator, silently adding a per-layer, per-token
// device round trip, with no error and no log line from either side. The
// forced value also suppresses the AUTO-only promotions and rejections that
// llama.cpp performs at llama-context.cpp:3546-3569.
//
// The fix is for DerefFlashAttention(nil) to return FlashAttentionAuto, so an
// unconfigured Kronk behaves like an unconfigured llama.cpp. Note that
// config_test.go:104 ("unset" -> FlashAttentionEnabled) pins the current wrong
// default and must be updated in the same commit.
func TestFlashAttentionUnsetResolvesToLlamaAuto(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want llama.FlashAttentionType
	}{
		{"unset must match the llama.cpp default", NewConfig(), llama.FlashAttentionTypeAuto},
		{"explicit auto", NewConfig(WithFlashAttention(FlashAttentionAuto)), llama.FlashAttentionTypeAuto},
		{"explicit enabled", NewConfig(WithFlashAttention(FlashAttentionEnabled)), llama.FlashAttentionTypeEnabled},
		{"explicit disabled", NewConfig(WithFlashAttention(FlashAttentionDisabled)), llama.FlashAttentionTypeDisabled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveFlashAttentionType(tt.cfg); got != tt.want {
				t.Errorf("flash_attn_type handed to llama_init_from_model: got %d, want %d", got, tt.want)
			}
		})
	}

	// The numeric values are load bearing: Kronk's own enum reuses 0 and 1 with
	// the opposite meaning, so any future assignment that skips the switch at
	// config.go:873-882 would be a silent inversion (both types are int32).
	if llama.FlashAttentionTypeAuto != -1 || llama.FlashAttentionTypeDisabled != 0 || llama.FlashAttentionTypeEnabled != 1 {
		t.Fatalf("yzma no longer mirrors llama.h:190-194: auto=%d disabled=%d enabled=%d",
			llama.FlashAttentionTypeAuto, llama.FlashAttentionTypeDisabled, llama.FlashAttentionTypeEnabled)
	}
}

// TestValidateConfigRejectsQuantizedVCacheWithFlashAttentionDisabled pins the
// missing pre-flight guard for the one KV-cache/Flash-Attention combination
// llama.cpp refuses outright.
//
// FINDING
// llama.cpp hard-rejects a quantized V cache when Flash Attention is explicitly
// DISABLED. Kronk lets that Config through validateConfig untouched, and the
// AutoTune/analyze path can even PRODUCE it: on a host without a usable GPU,
// buildProfile hardcodes flash_attention = "disabled"
// (sdk/tools/models/analyze.go:374-381) while the cache selector independently
// falls back from f16 to q8_0 for both K and V when f16 does not fit
// (sdk/tools/models/analyze.go:439, 462-464 via cacheRecommendations at :622-650).
// applyRecommendation then writes both into the model.Config
// (sdk/tools/models/kronkresolve.go:229-237, :306-315). Nothing in between
// notices the conflict.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/config.go:525-674 — validateConfig has no
//     CacheTypeK/CacheTypeV vs FlashAttention rule at all.
//   - sdk/kronk/model/config.go:865-871 — CacheTypeV is copied into
//     ctxParams.TypeV whenever it is not GGMLTypeAuto.
//   - sdk/kronk/model/config.go:874-876 — FlashAttentionDisabled is copied into
//     ctxParams.FlashAttentionType.
//
// LLAMA.CPP REFERENCE
//   - .extras/llama.cpp/src/llama-context.cpp:3561-3569
//     if (ggml_is_quantized(params.type_v) && flash_attn_type != ENABLED) {
//     AUTO     -> promoted to ENABLED ("required for quantized V cache");
//     DISABLED -> LLAMA_LOG_ERROR "quantized V cache requires flash_attn to be
//     enabled" and llama_init_from_model returns nullptr.
//   - .extras/llama.cpp/src/llama-context.cpp:458-461 — the same invariant is
//     re-checked inside the constructor and throws
//     "quantized V cache was requested, but this requires Flash Attention".
//
// FAILURE SCENARIO
// A default-config load is safe (CacheTypeV stays GGMLTypeAuto, so TypeV keeps
// llama.cpp's F16), but any config that pairs cache-type-v: q8_0 with
// flash-attention: disabled — including one AutoTune wrote itself on a
// CPU-only, RAM-tight host — reaches llama_init_from_model and comes back as a
// nil context. Kronk surfaces that as a generic "unable to init context" with
// llama.cpp's explanatory line only in the C log, instead of failing in
// validateConfig with the actual conflict. The three control rows below are the
// combinations llama.cpp accepts (AUTO is promoted, ENABLED is honoured, F16
// needs nothing) and must keep passing after the guard is added.
func TestValidateConfigRejectsQuantizedVCacheWithFlashAttentionDisabled(t *testing.T) {
	discard := func(ctx context.Context, msg string, args ...any) {}

	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			"quantized V cache with flash attention disabled is rejected by llama.cpp",
			NewConfig(
				WithModelFiles([]string{"dummy.gguf"}),
				WithCacheTypeV(GGMLTypeQ8_0),
				WithFlashAttention(FlashAttentionDisabled),
			),
			true,
		},
		{
			"quantized V cache with flash attention auto is promoted by llama.cpp",
			NewConfig(
				WithModelFiles([]string{"dummy.gguf"}),
				WithCacheTypeV(GGMLTypeQ8_0),
				WithFlashAttention(FlashAttentionAuto),
			),
			false,
		},
		{
			"quantized V cache with flash attention enabled is honoured",
			NewConfig(
				WithModelFiles([]string{"dummy.gguf"}),
				WithCacheTypeV(GGMLTypeQ8_0),
				WithFlashAttention(FlashAttentionEnabled),
			),
			false,
		},
		{
			"f16 V cache with flash attention disabled is fine",
			NewConfig(
				WithModelFiles([]string{"dummy.gguf"}),
				WithCacheTypeV(GGMLTypeF16),
				WithFlashAttention(FlashAttentionDisabled),
			),
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(context.Background(), tt.cfg, discard)

			switch {
			case tt.wantErr && err == nil:
				t.Error("validateConfig: got nil, want an error naming the quantized V cache / flash attention conflict")

			case tt.wantErr && !strings.Contains(strings.ToLower(err.Error()), "flash"):
				t.Errorf("validateConfig: got %v, want an error naming the flash attention conflict", err)

			case !tt.wantErr && err != nil:
				t.Errorf("validateConfig: got %v, want nil", err)
			}
		})
	}
}
