package testlib

import (
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// GrammarJSONObject is a GBNF grammar for JSON objects.
var GrammarJSONObject = `root ::= object
value ::= object | array | string | number | "true" | "false" | "null"
object ::= "{" ws ( string ":" ws value ("," ws string ":" ws value)* )? ws "}"
array ::= "[" ws ( value ("," ws value)* )? ws "]"
string ::= "\"" ([^"\\] | "\\" ["\\bfnrt/] | "\\u" [0-9a-fA-F]{4})* "\""
number ::= "-"? ("0" | [1-9][0-9]*) ("." [0-9]+)? ([eE] [+-]? [0-9]+)?
ws ::= [ \t\n\r]*`

// Test input data initialized during Setup.
var (
	DChatNoTool      model.D
	DChatTool        model.D
	DChatToolGPT     model.D
	DMedia           model.D
	DAudio           model.D
	DResponseNoTool  model.D
	DResponseTool    model.D
	DChatNoToolArray model.D
	DMediaArray      model.D
	DGrammarJSON     model.D
)

func initInputs() error {

	// Text-based inputs.

	DChatNoTool = model.D{
		"messages": []model.D{
			{
				"role":    "user",
				"content": "Echo back the word: Gorilla",
			},
		},
		"max_tokens": 2048,
	}

	DChatTool = model.D{
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

	DChatToolGPT = model.D{
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

	DResponseNoTool = model.D{
		"messages": []model.D{
			{
				"role":    "user",
				"content": "Echo back the word: Gorilla",
			},
		},
		"max_tokens": 2048,
	}

	DResponseTool = model.D{
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

	DChatNoToolArray = model.D{
		"messages": []model.D{
			model.TextMessageArray("user", "Echo back the word: Gorilla"),
		},
		"max_tokens": 2048,
	}

	DGrammarJSON = model.D{
		"messages": []model.D{
			{
				"role":    "user",
				"content": "List 3 programming languages with their year of creation. Respond in JSON format.",
			},
		},
		"grammar":     GrammarJSONObject,
		"temperature": 0.7,
		"max_tokens":  512,
	}

	// Media inputs (need image file).

	if _, err := os.Stat(ImageFile); err == nil {
		mediaBytes, err := os.ReadFile(ImageFile)
		if err != nil {
			return fmt.Errorf("error reading image file %q: %w", ImageFile, err)
		}

		DMedia = model.D{
			"messages":   model.RawMediaMessage("What is in this picture?", mediaBytes),
			"max_tokens": 2048,
		}

		DMediaArray = model.D{
			"messages":   model.ImageMessage("What is in this picture?", mediaBytes, "jpeg"),
			"max_tokens": 2048,
		}
	}

	// Audio inputs (need audio file).

	if _, err := os.Stat(AudioFile); err == nil {
		audioBytes, err := os.ReadFile(AudioFile)
		if err != nil {
			return fmt.Errorf("error reading audio file %q: %w", AudioFile, err)
		}

		DAudio = model.D{
			"messages":   model.RawMediaMessage("Please describe what you hear in the following audio clip.", audioBytes),
			"max_tokens": 2048,
		}
	}

	return nil
}
