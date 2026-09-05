// Package testlib provides shared test infrastructure for Kronk model test packages.
package testlib

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
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
	MPThinkToolChat  models.Path
	MPGPTChat        models.Path
	MPHybridVision   models.Path
	MPLFMChat        models.Path
	MPSimpleVision   models.Path
	MPMoEVision      models.Path
	MPAudio          models.Path
	MPEmbedBatchSeq  models.Path
	MPRerankBatchSeq models.Path
	MPMTP            models.Path
	MPDraft          models.Path
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

	resolveModel(mdls, "unsloth/Qwen3-1.7B-Q4_K_M", &MPThinkToolChat)
	resolveModel(mdls, "unsloth/Qwen3.5-0.8B-Q8_0", &MPSimpleVision)
	resolveModel(mdls, "unsloth/gemma-4-26B-A4B-it-UD-Q4_K_M", &MPMoEVision)
	resolveModel(mdls, "nomic-ai/nomic-embed-text-v1.5.Q8_0", &MPEmbedBatchSeq)
	resolveModel(mdls, "gpustack/bge-reranker-v2-m3-Q8_0", &MPRerankBatchSeq)
	resolveModel(mdls, "unsloth/gpt-oss-20b-Q8_0", &MPGPTChat)
	resolveModel(mdls, "ggml-org/Qwen2.5-Omni-3B-Q4_K_M", &MPAudio)
	resolveModel(mdls, "mradermacher/Qwopus3.5-4B-Coder.Q4_K_M", &MPHybridVision)
	resolveModel(mdls, "unsloth/LFM2-700M-Q8_0", &MPLFMChat)
	resolveModel(mdls, "unsloth/mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL", &MPMTP)
	resolveModel(mdls, "unsloth/Qwen3-0.6B-Q8_0", &MPDraft)

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

	// After Init, never before: ggml enumerates its devices when the library
	// loads, so this is the first moment the executing backend is knowable.
	fmt.Println("processor        :", Processor())

	fmt.Println("Initializing test inputs...")
	if err := initInputs(); err != nil {
		fmt.Printf("Failed to init test inputs: %s\n", err)
		os.Exit(1)
	}
}

// resolveModel populates mp when the model is present on disk. A miss is not
// fatal — CI installs only the subset in .github/test-models.txt — but it is
// printed, so a runner with no models cannot look like a passing run.
func resolveModel(mdls *models.Models, name string, mp *models.Path) {
	dp, err := mdls.FullPath(name)
	if err != nil {
		fmt.Printf("RetrieveModel %s... MISSING (%s)\n", name, err)
		return
	}

	fmt.Printf("RetrieveModel %s...\n", name)
	*mp = dp
}

func printInfo(mdls *models.Models) {
	fmt.Println("libpath          :", libs.Path(""))
	fmt.Println("useLibVersion    :", defaults.LibVersion(""))
	fmt.Println("modelPath        :", mdls.Path())
	fmt.Println("imageFile        :", ImageFile)
	// The BUNDLE, which need not be the backend it executes on — see
	// Processor, which Setup prints once kronk.Init has enumerated devices.
	fmt.Println("libBundle        :", filepath.Base(libs.Path("")))
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

// Processor reports the ggml backend the suites execute on (devices.Backend),
// not the bundle name on disk; the two diverge. No config here pins
// NGpuLayers, so suites offload to whatever GPU ggml found
// (sdk/kronk/model/model.go:522-523). Falls back to the bundle name pre-Init.
func Processor() string {
	if backend := devices.Backend(); backend != "" {
		return backend
	}

	return filepath.Base(libs.Path(""))
}

// SkipOnBackends skips a test failing because of a defect in one of the named
// ggml backends rather than in Kronk; reason is printed with the skip.
// Backends are Kronk processor names matched against Processor — the executing
// backend, not the bundle. Never use it for a Kronk bug seen on one backend.
func SkipOnBackends(t *testing.T, reason string, backends ...string) {
	t.Helper()

	if processor := Processor(); slices.Contains(backends, processor) {
		t.Skipf("%s backend: %s", processor, reason)
	}
}

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
		ModelFiles:          mp.ModelFiles,
		PtrContextWindow:    new(32768),
		PtrPrefillBatchSize: new(256),
		CacheTypeK:          model.GGMLTypeF16,
		CacheTypeV:          model.GGMLTypeF16,
		PtrNSeqMax:          new(2),
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
	}

	return krn, d
}

// =========================================================================
// Config builders for each model type.

func CfgThinkToolChat() model.Config {
	return model.Config{
		ModelFiles:          MPThinkToolChat.ModelFiles,
		PtrContextWindow:    new(8192),
		PtrPrefillBatchSize: new(512),
		CacheTypeK:          model.GGMLTypeQ8_0,
		CacheTypeV:          model.GGMLTypeQ8_0,
		PtrNSeqMax:          new(2),
	}
}

func CfgGPTChat() model.Config {
	return model.Config{
		ModelFiles:          MPGPTChat.ModelFiles,
		PtrContextWindow:    new(8192),
		PtrPrefillBatchSize: new(512),
		CacheTypeK:          model.GGMLTypeQ8_0,
		CacheTypeV:          model.GGMLTypeQ8_0,
		PtrNSeqMax:          new(2),
	}
}

func CfgSimpleVision() model.Config {
	return model.Config{
		ModelFiles:          MPSimpleVision.ModelFiles,
		ProjFile:            MPSimpleVision.ProjFile,
		PtrContextWindow:    new(8192),
		PtrPrefillBatchSize: new(2048),
		CacheTypeK:          model.GGMLTypeQ8_0,
		CacheTypeV:          model.GGMLTypeQ8_0,
	}
}

func CfgSimpleVisionIMC() model.Config {
	return model.Config{
		ModelFiles:          MPSimpleVision.ModelFiles,
		ProjFile:            MPSimpleVision.ProjFile,
		PtrContextWindow:    new(8192),
		PtrPrefillBatchSize: new(2048),
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
		PtrPrefillBatchSize: new(2048),
		CacheTypeK:          model.GGMLTypeF16,
		CacheTypeV:          model.GGMLTypeF16,
		PtrIncrementalCache: new(true),
		PtrNSeqMax:          new(1),
	}
}

// CfgEmbedBatchSeq returns the Nomic sequence-batch configuration.
func CfgEmbedBatchSeq() model.Config {
	return model.Config{
		ModelFiles:          MPEmbedBatchSeq.ModelFiles,
		PtrContextWindow:    new(8192),
		PtrPrefillBatchSize: new(2048),
		CacheTypeK:          model.GGMLTypeF16,
		CacheTypeV:          model.GGMLTypeF16,
		PtrNSeqMax:          new(4),
	}
}

// CfgRerankBatchSeq returns the BGE sequence-batch configuration.
func CfgRerankBatchSeq() model.Config {
	return model.Config{
		ModelFiles:          MPRerankBatchSeq.ModelFiles,
		PtrContextWindow:    new(2048),
		PtrPrefillBatchSize: new(512),
		CacheTypeK:          model.GGMLTypeF16,
		CacheTypeV:          model.GGMLTypeF16,
		PtrNSeqMax:          new(4),
	}
}

func CfgAudio() model.Config {
	return model.Config{
		ModelFiles:          MPAudio.ModelFiles,
		ProjFile:            MPAudio.ProjFile,
		PtrContextWindow:    new(8192),
		PtrPrefillBatchSize: new(2048),
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
		ModelFiles:          MPMoEVision.ModelFiles,
		PtrContextWindow:    new(8192),
		PtrPrefillBatchSize: new(2048),
		CacheTypeK:          model.GGMLTypeF16,
		CacheTypeV:          model.GGMLTypeF16,
		PtrNSeqMax:          new(2),
	}
}

func CfgHybridChat() model.Config {
	return model.Config{
		ModelFiles:          MPHybridVision.ModelFiles,
		PtrContextWindow:    new(8192),
		PtrPrefillBatchSize: new(512),
		CacheTypeK:          model.GGMLTypeF16,
		CacheTypeV:          model.GGMLTypeF16,
		PtrNSeqMax:          new(2),
	}
}

// CfgLFMChat returns a two-slot LFM2 configuration with IMC enabled so the
// model-backed suite exercises per-sequence recurrent-state save and restore.
func CfgLFMChat() model.Config {
	return model.Config{
		ModelFiles:          MPLFMChat.ModelFiles,
		PtrContextWindow:    new(4096),
		PtrPrefillBatchSize: new(512),
		CacheTypeK:          model.GGMLTypeF16,
		CacheTypeV:          model.GGMLTypeF16,
		PtrNSeqMax:          new(2),
		PtrIncrementalCache: new(true),
		PtrCacheMinTokens:   new(1),
		Speculation:         model.SpeculationDisabled,
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
		ModelFiles:          MPMTP.ModelFiles,
		PtrContextWindow:    new(8192),
		PtrPrefillBatchSize: new(512),
		CacheTypeK:          model.GGMLTypeF16,
		CacheTypeV:          model.GGMLTypeF16,
		PtrNSeqMax:          new(1),
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
// The cache settings match the single-slot MTP configuration.
func CfgMTPChatMultiSlot() model.Config {
	return model.Config{
		ModelFiles:          MPMTP.ModelFiles,
		PtrContextWindow:    new(8192),
		PtrPrefillBatchSize: new(512),
		CacheTypeK:          model.GGMLTypeF16,
		CacheTypeV:          model.GGMLTypeF16,
		PtrNSeqMax:          new(2),
	}
}

// CfgClassicDraftChat returns a single-slot chat config that uses a
// TRADITIONAL separate-GGUF draft model for speculative decoding: the
// Qwen3-1.7B target paired with the vocab-matched Qwen3-0.6B draft. This
// exercises the classic drafter path (loadDraftModel / generateDraftTokens
// / the classic rollback + unload-with-ModelFree branches), which is
// distinct from the MTP path. NSeqMax=1 because the separate draft context
// is created single-sequence (see loadDraftModel).
func CfgClassicDraftChat() model.Config {
	return model.Config{
		ModelFiles:          MPThinkToolChat.ModelFiles,
		PtrContextWindow:    new(8192),
		PtrPrefillBatchSize: new(512),
		CacheTypeK:          model.GGMLTypeQ8_0,
		CacheTypeV:          model.GGMLTypeQ8_0,
		PtrNSeqMax:          new(1),
		PtrDraftModel: &model.DraftModelConfig{
			ModelFiles: MPDraft.ModelFiles,
			NDraft:     4,
		},
	}
}

// CfgGemma4MTPChat returns a single-slot chat config for the Gemma4
// gemma4-assistant separate-file MTP drafter, using the same
// gemma-4-26B-A4B-it-UD-Q4_K_M target the vision tests use (MPMoEVision).
// The drafter is the "mtp-*.gguf" companion that ships alongside the main
// model; we wire it via MTPDrafterFile exactly as the runtime's kronkresolve
// does (out.MTPDrafterFile = fp.MTPFile). The loader auto-loads it as a
// shared-KV MTP head (ctx_other==target). F16 KV matches the other MTP /
// SWA configs. NSeqMax=1 for the single-slot path.
func CfgGemma4MTPChat() model.Config {
	return model.Config{
		ModelFiles:          MPMoEVision.ModelFiles,
		MTPDrafterFile:      MPMoEVision.MTPFile,
		PtrContextWindow:    new(8192),
		PtrPrefillBatchSize: new(512),
		CacheTypeK:          model.GGMLTypeF16,
		CacheTypeV:          model.GGMLTypeF16,
		PtrNSeqMax:          new(1),
	}
}

// CfgGemma4MTPChatMultiSlot is the NSeqMax=2 variant of CfgGemma4MTPChat.
// It exercises the shared-KV MTP head across multiple concurrent slots:
// the Pass 2A/2B split, the per-slot pre-norm capture, and fixed-position
// drafting under contention.
func CfgGemma4MTPChatMultiSlot() model.Config {
	return model.Config{
		ModelFiles:          MPMoEVision.ModelFiles,
		MTPDrafterFile:      MPMoEVision.MTPFile,
		PtrContextWindow:    new(8192),
		PtrPrefillBatchSize: new(512),
		CacheTypeK:          model.GGMLTypeF16,
		CacheTypeV:          model.GGMLTypeF16,
		PtrNSeqMax:          new(2),
	}
}

func CfgHybridVisionIMC() model.Config {
	return model.Config{
		ModelFiles:          MPHybridVision.ModelFiles,
		ProjFile:            MPHybridVision.ProjFile,
		PtrContextWindow:    new(4096),
		PtrPrefillBatchSize: new(512),
		CacheTypeK:          model.GGMLTypeQ8_0,
		CacheTypeV:          model.GGMLTypeQ8_0,
		PtrIncrementalCache: new(true),
		PtrNSeqMax:          new(1),
	}
}
