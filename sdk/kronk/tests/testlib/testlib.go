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
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// Settings controls test behavior.
var (
	TestDuration  = 60 * 5 * time.Second
	Goroutines    = 2
	RunInParallel = false
	ImageFile     string
	AudioFile     string
)

// Model paths resolved during Setup.
var (
	MPThinkToolChat models.Path
	MPGPTChat       models.Path
	MPHybridChat    models.Path
	MPMoEChat       models.Path
	MPSimpleVision  models.Path
	MPAudio         models.Path
	MPEmbed         models.Path
	MPRerank        models.Path
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
	resolveModel(mdls, "Qwen2.5-VL-3B-Instruct-Q8_0", &MPSimpleVision)
	resolveModel(mdls, "embeddinggemma-300m-qat-Q8_0", &MPEmbed)
	resolveModel(mdls, "bge-reranker-v2-m3-Q8_0", &MPRerank)
	resolveModel(mdls, "gpt-oss-20b-Q8_0", &MPGPTChat)
	resolveModel(mdls, "Qwen2-Audio-7B.Q8_0", &MPAudio)
	resolveModel(mdls, "Qwen3-Coder-Next-UD-Q6_K_XL", &MPHybridChat)
	resolveModel(mdls, "Qwen3.5-35B-A3B-UD-Q8_K_XL", &MPMoEChat)

	printInfo(mdls)

	ctx := context.Background()

	ctlg, err := catalog.New()
	if err != nil {
		fmt.Printf("unable to create catalog system: %s", err)
		os.Exit(1)
	}

	fmt.Println("Downloading Catalog and Templates...")
	if err := ctlg.Download(ctx); err != nil {
		fmt.Printf("unable to download catalog: %s", err)
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

	krn, err := kronk.New(cfg)
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
	krn, err := kronk.New(model.Config{
		ModelFiles:    mp.ModelFiles,
		ContextWindow: 32768,
		NBatch:        1024,
		NUBatch:       256,
		CacheTypeK:    model.GGMLTypeF16,
		CacheTypeV:    model.GGMLTypeF16,
		NSeqMax:       2,
	})

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
		ModelFiles:    MPThinkToolChat.ModelFiles,
		ContextWindow: 8192,
		NBatch:        2048,
		NUBatch:       512,
		CacheTypeK:    model.GGMLTypeQ8_0,
		CacheTypeV:    model.GGMLTypeQ8_0,
		NSeqMax:       2,
	}
}

func CfgGPTChat() model.Config {
	return model.Config{
		ModelFiles:    MPGPTChat.ModelFiles,
		ContextWindow: 8192,
		NBatch:        2048,
		NUBatch:       512,
		CacheTypeK:    model.GGMLTypeQ8_0,
		CacheTypeV:    model.GGMLTypeQ8_0,
		NSeqMax:       2,
	}
}

func CfgSimpleVision() model.Config {
	return model.Config{
		ModelFiles:    MPSimpleVision.ModelFiles,
		ProjFile:      MPSimpleVision.ProjFile,
		ContextWindow: 8192,
		NBatch:        2048,
		NUBatch:       2048,
		CacheTypeK:    model.GGMLTypeQ8_0,
		CacheTypeV:    model.GGMLTypeQ8_0,
	}
}

func CfgEmbed() model.Config {
	return model.Config{
		ModelFiles:     MPEmbed.ModelFiles,
		ContextWindow:  2048,
		NBatch:         2048,
		NUBatch:        512,
		CacheTypeK:     model.GGMLTypeQ8_0,
		CacheTypeV:     model.GGMLTypeQ8_0,
		FlashAttention: model.FlashAttentionEnabled,
	}
}

func CfgRerank() model.Config {
	return model.Config{
		ModelFiles:     MPRerank.ModelFiles,
		ContextWindow:  2048,
		NBatch:         2048,
		NUBatch:        512,
		CacheTypeK:     model.GGMLTypeQ8_0,
		CacheTypeV:     model.GGMLTypeQ8_0,
		FlashAttention: model.FlashAttentionEnabled,
	}
}

func CfgAudio() model.Config {
	return model.Config{
		ModelFiles:    MPAudio.ModelFiles,
		ProjFile:      MPAudio.ProjFile,
		ContextWindow: 8192,
		NBatch:        2048,
		NUBatch:       2048,
		CacheTypeK:    model.GGMLTypeQ8_0,
		CacheTypeV:    model.GGMLTypeQ8_0,
	}
}

func CfgMoEChat() model.Config {
	return model.Config{
		ModelFiles:    MPMoEChat.ModelFiles,
		ContextWindow: 8192,
		NBatch:        2048,
		NUBatch:       2048,
		CacheTypeK:    model.GGMLTypeF16,
		CacheTypeV:    model.GGMLTypeF16,
		NSeqMax:       2,
	}
}

func CfgHybridChat() model.Config {
	return model.Config{
		ModelFiles:    MPHybridChat.ModelFiles,
		ContextWindow: 8192,
		NBatch:        2048,
		NUBatch:       512,
		CacheTypeK:    model.GGMLTypeF16,
		CacheTypeV:    model.GGMLTypeF16,
		NSeqMax:       2,
	}
}
