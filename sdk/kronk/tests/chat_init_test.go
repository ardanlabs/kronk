package kronk_test

import (
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

// =============================================================================
// Test input data - initialized in TestMain

var (
	dChatNoTool      model.D
	dChatTool        model.D
	dChatToolGPT     model.D
	dMedia           model.D
	dAudio           model.D
	dResponseNoTool  model.D
	dResponseTool    model.D
	dChatNoToolArray model.D
	dMediaArray      model.D
)

func initChatTestInputs() error {
	if _, err := os.Stat(imageFile); err != nil {
		return fmt.Errorf("error accessing file %q: %w", imageFile, err)
	}

	mediaBytes, err := os.ReadFile(imageFile)
	if err != nil {
		return fmt.Errorf("error reading file %q: %w", imageFile, err)
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

	dMedia = model.D{
		"messages":   model.RawMediaMessage("What is in this picture?", mediaBytes),
		"max_tokens": 2048,
	}

	if _, err := os.Stat(audioFile); err == nil {
		audioBytes, err := os.ReadFile(audioFile)
		if err != nil {
			return fmt.Errorf("error reading file %q: %w", audioFile, err)
		}

		dAudio = model.D{
			"messages":   model.RawMediaMessage("Please describe what you hear in the following audio clip.", audioBytes),
			"max_tokens": 2048,
		}
	}

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

	dChatNoToolArray = model.D{
		"messages": []model.D{
			model.TextMessageArray("user", "Echo back the word: Gorilla"),
		},
		"max_tokens": 2048,
	}

	dMediaArray = model.D{
		"messages":   model.ImageMessage("What is in this picture?", mediaBytes, "jpeg"),
		"max_tokens": 2048,
	}

	return nil
}
