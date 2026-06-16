// Package testlib provides shared test infrastructure for Kronk model test packages.
package testlib

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// Settings controls test behavior.
var (
	TestDuration  = 60 * 5 * time.Second
	Goroutines    = 2
	MaxRetries    = 3
	RunInParallel = false
	ImageFile     string
	AudioFile     string
)

// Model paths resolved during Setup.
var (
	MPThinkToolChat models.Path
	MPGPTChat       models.Path
	MPHybridVision  models.Path
	MPSimpleVision  models.Path
	MPMoEVision     models.Path
	MPAudio         models.Path
	MPEmbed         models.Path
	MPRerank        models.Path
	MPMTP           models.Path
	MPDraft         models.Path
)

// Setup initializes the test environment. Call from each package's TestMain.
func Setup() {
	gw := os.Getenv("GITHUB_WORKSPACE")
	ImageFile = filepath.Join(gw, "examples/samples/giraffe.jpg")
	AudioFile = filepath.Join(gw, "examples/samples/jfk.wav")

	if os.Getenv("GITHUB_ACTIONS") == "true" {
		Goroutines = 1
	}

	if os.Getenv("RUN_IN_PARALLEL") == "yes" {
		RunInParallel = true
	}

	fmt.Println("Initializing models system...")
	mdls, err := models.New()
	if err != nil {
		fmt.Printf("creating models system: %s\n", err)
		os.Exit(1)
	}

	resolveModel(mdls, "Qwen3-8B-Q8_0", &MPThinkToolChat)
	resolveModel(mdls, "Qwen3.5-0.8B-Q8_0", &MPSimpleVision)
	resolveModel(mdls, "gemma-4-26B-A4B-it-UD-Q4_K_M", &MPMoEVision)
	resolveModel(mdls, "embeddinggemma-300m-qat-Q8_0", &MPEmbed)
	resolveModel(mdls, "bge-reranker-v2-m3-Q8_0", &MPRerank)
	resolveModel(mdls, "gpt-oss-20b-Q8_0", &MPGPTChat)
	resolveModel(mdls, "Qwen2.5-Omni-3B-Q8_0", &MPAudio)
	resolveModel(mdls, "Qwen3.6-35B-A3B-UD-Q4_K_M", &MPHybridVision)
	resolveModel(mdls, "mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL", &MPMTP)
	resolveModel(mdls, "Qwen3-0.6B-Q8_0", &MPDraft)

	printInfo(mdls)

	fmt.Println("Seeding jinja templates...")
	if err := defaults.WriteJinjaFiles("", ""); err != nil {
		fmt.Printf("Failed to write jinja templates: %s\n", err)
		os.Exit(1)
	}

	fmt.Println("Init Kronk...")
	if err := kronk.Init(); err != nil {
		fmt.Printf("Failed to init the llama.cpp library: error: %s\n", err)
		os.Exit(1)
	}

	fmt.Println("Initializing test inputs...")
	if err := initInputs(); err != nil {
		fmt.Printf("Failed to init test inputs: %s\n", err)
		os.Exit(1)
	}
}

func resolveModel(mdls *models.Models, name string, mp *models.Path) {
	if dp, err := mdls.FullPath(name); err == nil {
		fmt.Printf("RetrieveModel %s...\n", name)
		*mp = dp
	}
}

func printInfo(mdls *models.Models) {
	fmt.Println("libpath          :", libs.Path(""))
	fmt.Println("useLibVersion    :", defaults.LibVersion(""))
	fmt.Println("modelPath        :", mdls.Path())
	fmt.Println("imageFile        :", ImageFile)
	fmt.Println("processor        :", "cpu")
	fmt.Println("goroutines       :", Goroutines)
	fmt.Println("maxRetries       :", MaxRetries)
	fmt.Println("testDuration     :", TestDuration)
	fmt.Println("RUN_IN_PARALLEL  :", RunInParallel)

	l, err := libs.New(libs.WithVersion(defaults.LibVersion("")))
	if err != nil {
		fmt.Printf("Failed to construct the libs api: %v\n", err)
		os.Exit(1)
	}

	currentVersion, err := l.InstalledVersion()
	if err != nil {
		fmt.Printf("Failed to retrieve version info: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Installed version: %s\n", currentVersion)
}

// =========================================================================

// WithModel creates a Kronk instance for the duration of fn, handling cleanup.
func WithModel(t *testing.T, cfg model.Config, fn func(t *testing.T, krn *kronk.Kronk)) {
	t.Helper()

	krn, err := kronk.New(model.WithConfig(cfg))
	if err != nil {
		t.Fatalf("unable to load model %v: %v", cfg.ModelFiles, err)
	}

	t.Cleanup(func() {
		t.Logf("active streams: %d", krn.ActiveStreams())
		t.Log("unloading model")
		if err := krn.Unload(context.Background()); err != nil {
			t.Errorf("failed to unload model: %v", err)
		}
	})

	fn(t, krn)
}

// InitChatTest creates a new Kronk instance for tests that need their own
// model lifecycle (e.g., concurrency tests that test unload behavior).
func InitChatTest(t *testing.T, mp models.Path, tooling bool) (*kronk.Kronk, model.D) {
	krn, err := kronk.New(model.WithConfig(model.Config{
		ModelFiles:       mp.ModelFiles,
		PtrContextWindow: new(32768),
		PtrNBatch:        new(1024),
		PtrNUBatch:       new(256),
		CacheTypeK:       model.GGMLTypeF16,
		CacheTypeV:       model.GGMLTypeF16,
		PtrNSeqMax:       new(2),
	}))

	if err != nil {
		t.Fatalf("unable to load model: %v: %v", mp.ModelFiles, err)
	}

	question := "Echo back the word: Gorilla"
	if tooling {
		question = "What is the weather in London, England?"
	}

	d := model.D{
		"messages": []model.D{
			{
				"role":    "user",
				"content": question,
			},
		},
		"max_tokens": 2048,
	}

	if tooling {
		switch krn.ModelInfo().IsGPTModel {
		case true:
			d["tools"] = []model.D{
				{
					"type": "function",
					"function": model.D{
						"name":        "get_weather",
						"description": "Get the current weather for a location",
						"parameters": model.D{
							"type": "object",
							"properties": model.D{
								"location": model.D{
									"type":        "string",
									"description": "The location to get the weather for, e.g. San Francisco, CA",
								},
							},
							"required": []any{"location"},
						},
					},
				},
			}

		default:
			d["tools"] = []model.D{
				{
					"type": "function",
					"function": model.D{
						"name":        "get_weather",
						"description": "Get the current weather for a location",
						"arguments": model.D{
							"location": model.D{
								"type":        "string",
								"description": "The location to get the weather for, e.g. San Francisco, CA",
							},
						},
					},
				},
			}
		}
	}

	return krn, d
}

// =========================================================================
// Config builders for each model type.

func CfgThinkToolChat() model.Config {
	return model.Config{
		ModelFiles:       MPThinkToolChat.ModelFiles,
		PtrContextWindow: new(8192),
		PtrNBatch:        new(2048),
		PtrNUBatch:       new(512),
		CacheTypeK:       model.GGMLTypeQ8_0,
		CacheTypeV:       model.GGMLTypeQ8_0,
		PtrNSeqMax:       new(2),
	}
}

func CfgGPTChat() model.Config {
	return model.Config{
		ModelFiles:       MPGPTChat.ModelFiles,
		PtrContextWindow: new(8192),
		PtrNBatch:        new(2048),
		PtrNUBatch:       new(512),
		CacheTypeK:       model.GGMLTypeQ8_0,
		CacheTypeV:       model.GGMLTypeQ8_0,
		PtrNSeqMax:       new(2),
	}
}

func CfgSimpleVision() model.Config {
	return model.Config{
		ModelFiles:       MPSimpleVision.ModelFiles,
		ProjFile:         MPSimpleVision.ProjFile,
		PtrContextWindow: new(8192),
		PtrNBatch:        new(2048),
		PtrNUBatch:       new(2048),
		CacheTypeK:       model.GGMLTypeQ8_0,
		CacheTypeV:       model.GGMLTypeQ8_0,
	}
}

func CfgSimpleVisionIMC() model.Config {
	return model.Config{
		ModelFiles:          MPSimpleVision.ModelFiles,
		ProjFile:            MPSimpleVision.ProjFile,
		PtrContextWindow:    new(8192),
		PtrNBatch:           new(2048),
		PtrNUBatch:          new(2048),
		CacheTypeK:          model.GGMLTypeQ8_0,
		CacheTypeV:          model.GGMLTypeQ8_0,
		PtrIncrementalCache: new(true),
		PtrNSeqMax:          new(1),
	}
}

func CfgMoEVisionIMC() model.Config {
	return model.Config{
		ModelFiles:          MPMoEVision.ModelFiles,
		ProjFile:            MPMoEVision.ProjFile,
		PtrContextWindow:    new(8192),
		PtrNBatch:           new(2048),
		PtrNUBatch:          new(2048),
		CacheTypeK:          model.GGMLTypeF16,
		CacheTypeV:          model.GGMLTypeF16,
		PtrIncrementalCache: new(true),
		PtrNSeqMax:          new(1),
	}
}

func CfgEmbed() model.Config {
	return model.Config{
		ModelFiles:       MPEmbed.ModelFiles,
		PtrContextWindow: new(2048),
		PtrNBatch:        new(2048),
		PtrNUBatch:       new(512),
		CacheTypeK:       model.GGMLTypeF16,
		CacheTypeV:       model.GGMLTypeF16,
	}
}

func CfgRerank() model.Config {
	return model.Config{
		ModelFiles:       MPRerank.ModelFiles,
		PtrContextWindow: new(2048),
		PtrNBatch:        new(2048),
		PtrNUBatch:       new(512),
		CacheTypeK:       model.GGMLTypeF16,
		CacheTypeV:       model.GGMLTypeF16,
	}
}

func CfgAudio() model.Config {
	return model.Config{
		ModelFiles:       MPAudio.ModelFiles,
		ProjFile:         MPAudio.ProjFile,
		PtrContextWindow: new(8192),
		PtrNBatch:        new(2048),
		PtrNUBatch:       new(2048),
		// Keep K/V at F16. Audio multimodal models are unusually
		// sensitive to KV-cache quantization: audio tokens encode
		// fine-grained acoustic structure, and the noise introduced by
		// Q8_0 K/V degrades attention scores enough that decoding
		// collapses into a degenerate repetition attractor under
		// concurrent load. Matches the example program (which uses
		// defaults) and the other quality-sensitive configs.
		CacheTypeK: model.GGMLTypeF16,
		CacheTypeV: model.GGMLTypeF16,
	}
}

func CfgMoEVision() model.Config {
	return model.Config{
		ModelFiles:       MPMoEVision.ModelFiles,
		PtrContextWindow: new(8192),
		PtrNBatch:        new(2048),
		PtrNUBatch:       new(2048),
		CacheTypeK:       model.GGMLTypeF16,
		CacheTypeV:       model.GGMLTypeF16,
		PtrNSeqMax:       new(2),
	}
}

func CfgHybridChat() model.Config {
	return model.Config{
		ModelFiles:       MPHybridVision.ModelFiles,
		PtrContextWindow: new(8192),
		PtrNBatch:        new(2048),
		PtrNUBatch:       new(512),
		CacheTypeK:       model.GGMLTypeF16,
		CacheTypeV:       model.GGMLTypeF16,
		PtrNSeqMax:       new(2),
	}
}

// CfgMTPChat returns a single-slot chat config for the Qwen3.6-35B-A3B
// MTP target. The MTP drafter auto-enables based on the GGUF's
// nextn_predict_layers metadata, so no explicit DraftModel block is
// needed. Use CfgMTPChatMultiSlot for the multi-slot variant that
// exercises the Pass 2A/2B split and the multi-slot prefill
// contiguity constraint in processBatch.
func CfgMTPChat() model.Config {
	return model.Config{
		ModelFiles:       MPMTP.ModelFiles,
		PtrContextWindow: new(8192),
		PtrNBatch:        new(2048),
		PtrNUBatch:       new(512),
		CacheTypeK:       model.GGMLTypeF16,
		CacheTypeV:       model.GGMLTypeF16,
		PtrNSeqMax:       new(1),
	}
}

// CfgMTPChatMultiSlot returns a multi-slot chat config for the
// Qwen3.6-35B-A3B MTP target. NSeqMax=2 exercises code paths that are
// trivially unreachable at NSeqMax=1:
//
//   - The Pass 2A / Pass 2B split in processBatch (Phase A read-only
//     verify across spec slots, then Phase B finalize) — multi-slot
//     hybrid + MTP could otherwise crash inside llama_sampler_sample
//     when one slot's hybrid restore wiped another slot's logits.
//
//   - The "one prefill chunk per slot per round" cap in processBatch
//     that keeps each slot's pre-norm rows contiguous in e.batch so
//     mirrorTargetBatchToMTPDraft mirrors the right rows.
//
// Hybrid target requires f16 KV and disabled flash-attention (see
// config.go), inherited from the single-slot config.
func CfgMTPChatMultiSlot() model.Config {
	return model.Config{
		ModelFiles:       MPMTP.ModelFiles,
		PtrContextWindow: new(8192),
		PtrNBatch:        new(2048),
		PtrNUBatch:       new(512),
		CacheTypeK:       model.GGMLTypeF16,
		CacheTypeV:       model.GGMLTypeF16,
		PtrNSeqMax:       new(2),
	}
}

// CfgClassicDraftChat returns a single-slot chat config that uses a
// TRADITIONAL separate-GGUF draft model for speculative decoding: the
// Qwen3-8B target paired with the vocab-matched Qwen3-0.6B draft. This
// exercises the classic drafter path (loadDraftModel / generateDraftTokens
// / the classic rollback + unload-with-ModelFree branches), which is
// distinct from the MTP path. NSeqMax=1 because the separate draft context
// is created single-sequence (see loadDraftModel).
func CfgClassicDraftChat() model.Config {
	return model.Config{
		ModelFiles:       MPThinkToolChat.ModelFiles,
		PtrContextWindow: new(8192),
		PtrNBatch:        new(2048),
		PtrNUBatch:       new(512),
		CacheTypeK:       model.GGMLTypeQ8_0,
		CacheTypeV:       model.GGMLTypeQ8_0,
		PtrNSeqMax:       new(1),
		DraftModel: &model.DraftModelConfig{
			ModelFiles: MPDraft.ModelFiles,
			NDraft:     4,
		},
	}
}

func CfgHybridVisionIMC() model.Config {
	return model.Config{
		ModelFiles:          MPHybridVision.ModelFiles,
		ProjFile:            MPHybridVision.ProjFile,
		PtrContextWindow:    new(4096),
		PtrNBatch:           new(2048),
		PtrNUBatch:          new(512),
		CacheTypeK:          model.GGMLTypeQ8_0,
		CacheTypeV:          model.GGMLTypeQ8_0,
		PtrIncrementalCache: new(true),
		PtrNSeqMax:          new(1),
	}
}
