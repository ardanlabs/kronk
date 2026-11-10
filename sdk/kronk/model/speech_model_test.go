package model_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func TestResolveSpeechConfig(t *testing.T) {
	for _, tt := range []struct {
		name string
		cfg  model.Config
	}{
		{"defaults", model.Config{}},
		{"overrides", model.Config{PtrNSeqMax: new(3), PtrSpeech: &model.SpeechConfig{MaxFrames: 137}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := model.ResolveSpeechConfig(tt.cfg)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.ContextWindow() != 2048 || cfg.PrefillBatchSize() != 512 || cfg.PhysicalBatchSize() != 512 || cfg.EffectiveNBatch() != 512 || cfg.EffectiveNUBatch() != 512 {
				t.Fatalf("speech limits: got context=%d prefill=%d physical=%d batch=%d ubatch=%d", cfg.ContextWindow(), cfg.PrefillBatchSize(), cfg.PhysicalBatchSize(), cfg.EffectiveNBatch(), cfg.EffectiveNUBatch())
			}
			if cfg.NSeqMax() != max(1, tt.cfg.NSeqMax()) || cfg.SpeechMaxFrames() != tt.cfg.SpeechMaxFrames() {
				t.Errorf("overrides: got slots=%d frames=%d", cfg.NSeqMax(), cfg.SpeechMaxFrames())
			}
			if cfg.CacheTypeK != model.GGMLTypeF16 || cfg.CacheTypeV != model.GGMLTypeF16 || cfg.IncrementalCache() || cfg.SpeculationMode() != model.SpeculationDisabled {
				t.Errorf("speech cache/speculation: got K=%s V=%s IMC=%t speculation=%s", cfg.CacheTypeK, cfg.CacheTypeV, cfg.IncrementalCache(), cfg.SpeculationMode())
			}
			loaded, err := model.ResolveSpeechConfig(cfg)
			if err != nil || loaded.NSeqMax() != cfg.NSeqMax() || loaded.SpeechMaxFrames() != cfg.SpeechMaxFrames() || loaded.EffectiveNUBatch() != cfg.EffectiveNUBatch() {
				t.Fatalf("resolving prepared config changed limits: %+v, %v", loaded, err)
			}
			cfg.PtrSpeech.MaxFrames = 1
			if tt.cfg.SpeechMaxFrames() != loaded.SpeechMaxFrames() {
				t.Error("resolution mutated caller-owned speech settings")
			}
		})
	}

	for _, tt := range []struct {
		name string
		cfg  model.Config
	}{
		{"context", model.Config{PtrContextWindow: new(8192)}},
		{"prefill", model.Config{PtrPrefillBatchSize: new(256)}},
		{"physical", model.Config{PtrPhysicalBatchSize: new(1024)}},
		{"cache", model.Config{CacheTypeV: model.GGMLTypeQ8_0}},
		{"shared KV", model.Config{PtrKVCacheMode: new(model.KVCacheShared)}},
		{"IMC", model.Config{PtrIncrementalCache: new(true)}},
		{"draft", model.Config{PtrDraftModel: &model.DraftModelConfig{}}},
		{"frames", model.Config{PtrSpeech: &model.SpeechConfig{MaxFrames: 513}}},
		{"aggregate context", model.Config{PtrNSeqMax: new(1 << 21)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := model.ResolveSpeechConfig(tt.cfg); !errors.Is(err, model.ErrInvalidRequest) {
				t.Fatalf("got %v, want ErrInvalidRequest", err)
			}
		})
	}
}

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
			// Exercise both direct loading and speech-aware AutoTune resolution.
			krn, err := kronk.NewWithContext(t.Context(), append(opts, model.WithNSeqMax(nSlots), model.WithAutoTune(nSlots == 2))...)
			if err != nil {
				t.Fatal(err)
			}
			unloaded := false
			t.Cleanup(func() {
				if !unloaded {
					if err := krn.Unload(context.Background()); err != nil {
						t.Errorf("unload: %v", err)
					}
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

			prompts := []string{"I played column four. Your turn!", "Watch out for the diagonal. You can still block it in column three."}
			references := make([]model.SpeechResponse, len(prompts))
			for i, prompt := range prompts {
				resp, err := krn.Speech(t.Context(), model.SpeechRequest{Input: prompt})
				if err != nil {
					t.Fatal(err)
				}
				if len(resp.Audio) < 44 || resp.SampleRate != 24000 || resp.Samples <= 0 {
					t.Fatalf("speech: got rate=%d samples=%d bytes=%d", resp.SampleRate, resp.Samples, len(resp.Audio))
				}
				wav := resp.Audio
				if string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" || binary.LittleEndian.Uint32(wav[4:8]) != uint32(len(wav)-8) ||
					binary.LittleEndian.Uint16(wav[20:22]) != 1 || binary.LittleEndian.Uint16(wav[22:24]) != 1 ||
					binary.LittleEndian.Uint16(wav[34:36]) != 16 || int64(len(wav)) != 44+2*resp.Samples {
					t.Fatal("speech is not a complete PCM16 mono WAV")
				}
				references[i] = resp
			}
			if bytes.Equal(references[0].Audio, references[1].Audio) {
				t.Fatal("asymmetric utterances returned identical audio")
			}
			for _, req := range []model.SpeechRequest{
				{}, {Input: " \n"}, {Input: "bad\x00text"}, {Input: string([]byte{0xff})},
				{Input: strings.Repeat("x", 65537)}, {Input: strings.Repeat("hello ", 3000)},
				{Input: prompts[0], Voice: "not-a-voice"},
			} {
				resp, err := krn.Speech(t.Context(), req)
				if !errors.Is(err, model.ErrInvalidRequest) || len(resp.Audio) != 0 {
					t.Errorf("invalid speech request: got %v, bytes=%d", err, len(resp.Audio))
				}
			}

			// Four callers also exercise queueing on the one- and two-slot runs.
			outputs := make([]model.SpeechResponse, 4)
			errorsByCall := make([]error, len(outputs))
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range outputs {
				wg.Go(func() {
					<-start
					outputs[i], errorsByCall[i] = krn.Speech(t.Context(), model.SpeechRequest{Input: prompts[i%len(prompts)], Voice: "default"})
				})
			}
			close(start)
			wg.Wait()
			for i, output := range outputs {
				want := references[i%len(prompts)]
				if errorsByCall[i] != nil || output.SampleRate != want.SampleRate || output.Samples != want.Samples || !bytes.Equal(output.Audio, want.Audio) {
					t.Fatalf("overlapping speech %d differs from serial: %v", i, errorsByCall[i])
				}
			}

			ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
			defer cancel()
			wg.Go(func() {
				outputs[0], errorsByCall[0] = krn.Speech(ctx, model.SpeechRequest{Input: prompts[1]})
			})
			wg.Go(func() {
				outputs[1], errorsByCall[1] = krn.Speech(t.Context(), model.SpeechRequest{Input: prompts[0]})
			})
			wg.Wait()
			if !errors.Is(errorsByCall[0], context.DeadlineExceeded) || len(outputs[0].Audio) != 0 {
				t.Errorf("canceled speech: got %v, bytes=%d", errorsByCall[0], len(outputs[0].Audio))
			}
			if errorsByCall[1] != nil || !bytes.Equal(outputs[1].Audio, references[0].Audio) {
				t.Errorf("cancellation changed neighboring speech: %v", errorsByCall[1])
			}
			resp, err := krn.Speech(t.Context(), model.SpeechRequest{Input: prompts[1]})
			if err != nil || !bytes.Equal(resp.Audio, references[1].Audio) || krn.ActiveStreams() != 0 {
				t.Fatalf("speech reuse after cancellation: %v, active=%d", err, krn.ActiveStreams())
			}
			owned := bytes.Clone(resp.Audio)
			if err := krn.Unload(context.Background()); err != nil {
				t.Fatal(err)
			}
			unloaded = true
			if !bytes.Equal(resp.Audio, owned) {
				t.Error("returned audio changed after unload")
			}
			if resp, err := krn.Speech(t.Context(), model.SpeechRequest{Input: prompts[0]}); err == nil || len(resp.Audio) != 0 {
				t.Error("speech succeeded after unload")
			}
		})
	}

	t.Run("frame limit", func(t *testing.T) {
		krn, err := kronk.NewWithContext(t.Context(), append(opts, model.WithSpeech(&model.SpeechConfig{MaxFrames: 1}))...)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := krn.Unload(context.Background()); err != nil {
				t.Error(err)
			}
		})
		for range 2 {
			resp, err := krn.Speech(t.Context(), model.SpeechRequest{Input: "I played column four. Your turn!"})
			if !errors.Is(err, model.ErrSpeechLimit) || len(resp.Audio) != 0 || krn.ActiveStreams() != 0 {
				t.Fatalf("frame limit/reuse: got %v, bytes=%d active=%d", err, len(resp.Audio), krn.ActiveStreams())
			}
		}
	})

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
