package model_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// TestSpeechModelLifecycle uses an explicitly selected local speech bundle.
// Set KRONK_TEST_SPEECH_MODEL and KRONK_TEST_SPEECH_DECODER to the backbone and
// matching decoder paths; KRONK_TEST_LIB_PATH optionally selects native libraries.
func TestSpeechModelLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("native speech lifecycle skipped in short mode")
	}
	file := os.Getenv("KRONK_TEST_SPEECH_MODEL")
	decoder := os.Getenv("KRONK_TEST_SPEECH_DECODER")
	if file == "" || decoder == "" {
		t.Skip("set KRONK_TEST_SPEECH_MODEL and KRONK_TEST_SPEECH_DECODER to run native speech lifecycle checks")
	}
	if err := kronk.Init(kronk.WithLibPath(os.Getenv("KRONK_TEST_LIB_PATH"))); err != nil {
		t.Fatal(err)
	}
	opts := []model.Option{
		model.WithModelFiles([]string{file}), model.WithProjFile(decoder),
		// A speech load must never try to retrieve a chat template.
		model.WithJinjaFile("/does-not-exist/speech-must-not-load-chat.jinja"),
	}
	for _, nSlots := range []int{1, 2, 4} {
		t.Run(fmt.Sprintf("slots-%d", nSlots), func(t *testing.T) {
			krn, err := kronk.NewWithContext(t.Context(), append(opts, model.WithNSeqMax(nSlots))...)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := krn.Unload(context.Background()); err != nil {
					t.Errorf("unload: %v", err)
				}
			})
			info := krn.ModelInfo()
			if !info.IsSpeechModel || info.IsEmbedModel || info.IsRerankModel || info.IsDecisionModel || info.Template.FileName != "" {
				t.Errorf("model information: got %+v, want dedicated speech without a chat template", info)
			}
			cfg := krn.ModelConfig()
			if cfg.ContextWindow() != 2048 || cfg.NSeqMax() != nSlots || cfg.EffectiveNBatch() != 512 || cfg.EffectiveNUBatch() != 512 {
				t.Errorf("speech resource envelope: got context=%d slots=%d batch=%d ubatch=%d", cfg.ContextWindow(), cfg.NSeqMax(), cfg.EffectiveNBatch(), cfg.EffectiveNUBatch())
			}
			if cfg.IncrementalCache() || cfg.CacheTypeK != model.GGMLTypeF16 || cfg.CacheTypeV != model.GGMLTypeF16 || cfg.SpeculationMode() != model.SpeculationDisabled {
				t.Errorf("speech settings: got IMC=%t K=%s V=%s speculation=%s", cfg.IncrementalCache(), cfg.CacheTypeK, cfg.CacheTypeV, cfg.SpeculationMode())
			}
			if caps := krn.InputCapabilities(); !caps.Available || caps.Chat || caps.Vision || caps.Audio {
				t.Errorf("input capabilities: got %+v, want no chat or media inputs", caps)
			}
			if _, ok := krn.BatchEngineSnapshot(); ok {
				t.Error("speech model exposes a chat batch engine")
			}
			if _, err := krn.Chat(t.Context(), model.D{"messages": model.Messages(model.TextMessage(model.RoleUser, "Hello"))}); err == nil {
				t.Error("speech model accepted a chat request")
			}
			if _, err := krn.Embeddings(t.Context(), model.D{"input": "Hello"}); err == nil {
				t.Error("speech model accepted an embedding request")
			}
			if _, err := krn.Rerank(t.Context(), model.D{"query": "Hello", "documents": []string{"World"}}); err == nil {
				t.Error("speech model accepted a rerank request")
			}
		})
	}

	t.Run("incompatible settings", func(t *testing.T) {
		for _, tt := range []struct {
			name   string
			option model.Option
		}{
			{"context", model.WithContextWindow(4096)},
			{"prefill", model.WithPrefillBatchSize(256)},
			{"physical batch", model.WithPhysicalBatchSize(256)},
			{"shared KV", model.WithKVCacheMode(model.KVCacheShared)},
			{"IMC", model.WithIncrementalCache(true)},
			{"quantized KV", model.WithCacheTypeV(model.GGMLTypeQ8_0)},
		} {
			t.Run(tt.name, func(t *testing.T) {
				krn, err := kronk.NewWithContext(t.Context(), append(opts, tt.option)...)
				if krn != nil {
					unloadErr := krn.Unload(context.Background())
					t.Fatalf("incompatible speech settings constructed a handle; unload: %v", unloadErr)
				}
				if !errors.Is(err, model.ErrInvalidRequest) {
					t.Errorf("load: got %v, want ErrInvalidRequest", err)
				}
			})
		}
	})

	t.Run("missing decoder", func(t *testing.T) {
		mdl, err := model.NewModel(t.Context(), model.NewConfig(model.WithModelFiles([]string{file})))
		if mdl != nil {
			unloadErr := mdl.Unload(context.Background())
			t.Fatalf("speech without a decoder constructed a model; unload: %v", unloadErr)
		}
		if !errors.Is(err, model.ErrSpeechUnsupported) {
			t.Errorf("load: got %v, want ErrSpeechUnsupported", err)
		}
	})
}
