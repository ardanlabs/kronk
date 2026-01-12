package kronk_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// initChatTest creates a new Kronk instance for tests that need their own
// model lifecycle (e.g., concurrency tests that test unload behavior).
func initChatTest(t *testing.T, mp models.Path, tooling bool) (*kronk.Kronk, model.D) {
	krn, err := kronk.New(modelInstances, model.Config{
		ModelFiles: mp.ModelFiles,
		CacheTypeK: model.GGMLTypeF16,
		CacheTypeV: model.GGMLTypeF16,
		NBatch:     1024,
		NUBatch:    256,
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

// =============================================================================

var (
	krnThinkToolChat *kronk.Kronk
	krnGPTChat       *kronk.Kronk
	dChatNoTool      model.D
	dChatTool        model.D
	dChatToolGPT     model.D
)

func initChatModels() error {
	var err error

	fmt.Println("Loading krnThinkToolChat (Qwen3-8B-Q8_0)...")
	krnThinkToolChat, err = kronk.New(modelInstances, model.Config{
		ModelFiles: mpThinkToolChat.ModelFiles,
		CacheTypeK: model.GGMLTypeF16,
		CacheTypeV: model.GGMLTypeF16,
		NBatch:     1024,
		NUBatch:    256,
	})
	if err != nil {
		return fmt.Errorf("loading ThinkToolChat model: %w", err)
	}

	if os.Getenv("GITHUB_ACTIONS") != "true" {
		fmt.Println("Loading krnGPTChat (gpt-oss-20b-Q8_0)...")
		krnGPTChat, err = kronk.New(modelInstances, model.Config{
			ModelFiles: mpGPTChat.ModelFiles,
		})
		if err != nil {
			return fmt.Errorf("loading GPTChat model: %w", err)
		}
	}

	dChatNoTool = model.D{
		"messages": []model.D{
			{
				"role":    "user",
				"content": "Echo back the word: Gorilla",
			},
		},
		"max_tokens": 2048,
	}

	dChatTool = model.D{
		"messages": []model.D{
			{
				"role":    "user",
				"content": "What is the weather in London, England?",
			},
		},
		"max_tokens": 2048,
		"tools": []model.D{
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
		},
	}

	dChatToolGPT = model.D{
		"messages": []model.D{
			{
				"role":    "user",
				"content": "What is the weather in London, England?",
			},
		},
		"max_tokens": 2048,
		"tools": []model.D{
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
		},
	}

	return nil
}

func unloadChatModels() {
	ctx := context.Background()

	if krnThinkToolChat != nil {
		fmt.Println("Unloading krnThinkToolChat...")
		if err := krnThinkToolChat.Unload(ctx); err != nil {
			fmt.Printf("failed to unload ThinkToolChat: %v\n", err)
		}
	}

	if krnGPTChat != nil {
		fmt.Println("Unloading krnGPTChat...")
		if err := krnGPTChat.Unload(ctx); err != nil {
			fmt.Printf("failed to unload GPTChat: %v\n", err)
		}
	}
}

// =============================================================================

var (
	krnSimpleVision *kronk.Kronk
	dMedia          model.D
)

func initMediaModels() error {
	if _, err := os.Stat(imageFile); err != nil {
		return fmt.Errorf("error accessing file %q: %w", imageFile, err)
	}

	mediaBytes, err := os.ReadFile(imageFile)
	if err != nil {
		return fmt.Errorf("error reading file %q: %w", imageFile, err)
	}

	fmt.Println("Loading krnSimpleVision (Qwen2.5-VL-3B-Instruct-Q8_0)...")
	krnSimpleVision, err = kronk.New(modelInstances, model.Config{
		ModelFiles: mpSimpleVision.ModelFiles,
		ProjFile:   mpSimpleVision.ProjFile,
	})
	if err != nil {
		return fmt.Errorf("loading SimpleVision model: %w", err)
	}

	dMedia = model.D{
		"messages":   model.RawMediaMessage("What is in this picture?", mediaBytes),
		"max_tokens": 2048,
	}

	return nil
}

func unloadMediaModels() {
	ctx := context.Background()

	if krnSimpleVision != nil {
		fmt.Println("Unloading krnSimpleVision...")
		if err := krnSimpleVision.Unload(ctx); err != nil {
			fmt.Printf("failed to unload SimpleVision: %v\n", err)
		}
	}
}

// =============================================================================

var (
	dResponseNoTool model.D
	dResponseTool   model.D
)

func initResponseInputs() {
	dResponseNoTool = model.D{
		"messages": []model.D{
			{
				"role":    "user",
				"content": "Echo back the word: Gorilla",
			},
		},
		"max_tokens": 2048,
	}

	dResponseTool = model.D{
		"messages": []model.D{
			{
				"role":    "user",
				"content": "What is the weather in London, England?",
			},
		},
		"max_tokens": 2048,
		"tools": []model.D{
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
		},
	}
}
