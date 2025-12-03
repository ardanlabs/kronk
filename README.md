![yzma logo](./images/project/kronk_banner.jpg?v5)

# Kronk

This project lets you use Go for hardware accelerated local inference with llama.cpp directly integrated into your applications via the [yzma](https://github.com/hybridgroup/yzma) module. Kronk provides a high-level API that feels similar to using an OpenAI compatible API.

Copyright 2025 Ardan Labs  
hello@ardanlabs.com

[![Go Reference](https://pkg.go.dev/badge/github.com/ardanlabs/kronk.svg)](https://pkg.go.dev/github.com/ardanlabs/kronk)
[![Go Report Card](https://goreportcard.com/badge/github.com/ardanlabs/kronk)](https://goreportcard.com/report/github.com/ardanlabs/kronk)
[![go.mod Go version](https://img.shields.io/github/go-mod/go-version/ardanlabs/kronk)](https://github.com/ardanlabs/kronk)
[![llama.cpp Release](https://img.shields.io/github/v/release/ggml-org/llama.cpp?label=llama.cpp)](https://github.com/ggml-org/llama.cpp/releases)

[![Linux](https://github.com/ardanlabs/kronk/actions/workflows/linux.yml/badge.svg)](https://github.com/ardanlabs/kronk/actions/workflows/linux.yml)

## Owner Information

```
Name:    Bill Kennedy
Company: Ardan Labs
Title:   Managing Partner
Email:   bill@ardanlabs.com
Twitter: goinggodotnet
```

## Models

Kronk uses models in the GGUF format supported by llama.cpp. You can find many models in GGUF format on Hugging Face (over 147k at last count):

https://huggingface.co/models?library=gguf&sort=trending

## Support

Kronk currently has support for over 94% of llama.cpp functionality thanks to yzma. See the yzma [ROADMAP.md](https://github.com/ardanlabs/yzma/blob/main/ROADMAP.md) for the complete list.

You can use multimodal models (image/audio) and text language models with full hardware acceleration on Linux, on macOS, and on Windows.

| OS      | CPU          | GPU                             |
| ------- | ------------ | ------------------------------- |
| Linux   | amd64, arm64 | CUDA, Vulkan, HIP, ROCm, SYCL   |
| macOS   | arm64        | Metal                           |
| Windows | amd64        | CUDA, Vulkan, HIP, SYCL, OpenCL |

Whenever there is a new release of llama.cpp, the tests for yzma are run automatically. Kronk runs tests once a day and will check for updates to llama.cpp. This helps us stay up to date with the latest code and models.

## Examples

There are examples in the examples direction:

_The first time you run these programs the system will download and install the model and libraries._

[AUDIO](examples/audio/main.go) - This example shows you how to execute a simple prompt against an audio model.  
**$ make example-audio**

[CHAT](examples/chat/main.go) - This example shows you how to create a simple chat application against an inference model using kronk. Thanks to Kronk and yzma, reasoning and tool calling is enabled.  
**$ make example-chat**

[EMBEDDING](examples/embedding/main.go) - This example shows you how to use an embedding model.  
**$ make example-embedding**

[QUESTION](examples/question/main.go) - This example shows you a basic program of using Kronk to ask a model a question.  
**$ make example-question**

[VISION](examples/vision/main.go) - This example shows you how to execute a simple prompt against a vision model.  
**$ make example-vision**

[WEB](examples/web/main.go) - This example shows you a web service that provides a chat endpoint for asking questions to a model with a browser based chat UI.  
**$ make example-web**

You can find more examples in the ArdanLabs AI training repo at [Example13](https://github.com/ardanlabs/ai-training/tree/main/cmd/examples/example13).

## Sample - Question Example

```go
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/examples/install"
	"github.com/ardanlabs/kronk/model"
	"github.com/hybridgroup/yzma/pkg/download"
)

const (
	modelURL = "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf?download=true"
	// modelURL  = "https://huggingface.co/unsloth/gpt-oss-20b-GGUF/resolve/main/gpt-oss-20b-Q8_0.gguf?download=true"
	libPath        = "tests/libraries"
	modelPath      = "tests/models"
	modelInstances = 1
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	modelFile, err := installSystem()
	if err != nil {
		return fmt.Errorf("unable to installation system: %w", err)
	}

	krn, err := newKronk(modelFile)
	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}
	defer func() {
		fmt.Println("\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload model: %v", err)
		}
	}()

	// -------------------------------------------------------------------------

	messages := model.DocumentArray()

	for {
		messages, err = userInput(messages)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("user input: %w", err)
		}

		messages, err = func() ([]model.D, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			d := model.D{
				"messages":    messages,
				"tools":       tools(krn.ModelInfo().IsGPT),
				"max_tokens":  2048,
				"temperature": 0.7,
				"top_p":       0.9,
				"top_k":       40,
			}

			ch, err := performChat(ctx, krn, d)
			if err != nil {
				return nil, fmt.Errorf("unable to perform chat: %w", err)
			}

			messages, err = modelResponse(krn, messages, ch)
			if err != nil {
				return nil, fmt.Errorf("model response: %w", err)
			}

			return messages, nil
		}()

		if err != nil {
			return fmt.Errorf("unable to perform chat: %w", err)
		}
	}
}

func installSystem() (string, error) {
	if err := install.Libraries(libPath, download.CPU, true); err != nil {
		return "", fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	modelFile, err := install.Model(modelURL, modelPath)
	if err != nil {
		return "", fmt.Errorf("unable to install model: %w", err)
	}

	return modelFile, nil
}

func newKronk(modelFile string) (*kronk.Kronk, error) {
	if err := kronk.Init(libPath, kronk.LogSilent); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(modelInstances, model.Config{
		ModelFile: modelFile,
	})

	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\n\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow:", krn.ModelConfig().ContextWindow)
	fmt.Println("- embeddings   :", krn.ModelConfig().Embeddings)
	fmt.Println("- isGPT        :", krn.ModelInfo().IsGPT)

	return krn, nil
}

func userInput(messages []model.D) ([]model.D, error) {
	fmt.Print("\nUSER> ")

	reader := bufio.NewReader(os.Stdin)

	userInput, err := reader.ReadString('\n')
	if err != nil {
		return messages, fmt.Errorf("unable to read user input: %w", err)
	}

	if userInput == "quit\n" {
		return nil, io.EOF
	}

	messages = append(messages,
		model.TextMessage("user", userInput),
	)

	return messages, nil
}

func tools(isGPT bool) []model.D {
	if isGPT {
		return model.DocumentArray(
			model.D{
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
			model.D{
				"type": "function",
				"function": model.D{
					"name":        "invoke_cli_command",
					"description": "Use this anytime you need to run a CLI command of any kind",
					"parameters": model.D{
						"type": "object",
						"properties": model.D{
							"call": model.D{
								"type":        "string",
								"description": "The full set of parameters to pass to the CLI command",
							},
						},
						"required": []any{"call"},
					},
				},
			},
		)
	}

	return model.DocumentArray(
		model.D{
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
		model.D{
			"type": "function",
			"function": model.D{
				"name":        "invoke_cli_command",
				"description": "Use this anytime you need to run a CLI command of any kind",
				"arguments": model.D{
					"call": model.D{
						"type":        "string",
						"description": "The full set of parameters to pass to the CLI command",
					},
				},
				"required": []any{"call"},
			},
		},
	)
}

func performChat(ctx context.Context, krn *kronk.Kronk, d model.D) (<-chan model.ChatResponse, error) {
	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return nil, fmt.Errorf("chat streaming: %w", err)
	}

	return ch, nil
}

func modelResponse(krn *kronk.Kronk, messages []model.D, ch <-chan model.ChatResponse) ([]model.D, error) {
	fmt.Print("\nMODEL> ")

	var reasoning bool
	var lr model.ChatResponse

loop:
	for resp := range ch {
		lr = resp

		switch resp.Choice[0].FinishReason {
		case model.FinishReasonError:
			return messages, fmt.Errorf("error from model: %s", resp.Choice[0].Delta.Content)

		case model.FinishReasonStop:
			messages = append(messages,
				model.TextMessage("assistant", resp.Choice[0].Delta.Content),
			)
			break loop

		case model.FinishReasonTool:
			fmt.Println()
			if krn.ModelInfo().IsGPT {
				fmt.Println()
			}

			fmt.Printf("\u001b[92mModel Asking For Tool Call:\nToolID[%s]: %s(%s)\u001b[0m",
				resp.Choice[0].Delta.ToolCalls[0].ID,
				resp.Choice[0].Delta.ToolCalls[0].Name,
				resp.Choice[0].Delta.ToolCalls[0].Arguments,
			)

			messages = append(messages,
				model.TextMessage("tool", fmt.Sprintf("Tool call %s: %s(%v)",
					resp.Choice[0].Delta.ToolCalls[0].ID,
					resp.Choice[0].Delta.ToolCalls[0].Name,
					resp.Choice[0].Delta.ToolCalls[0].Arguments),
				),
			)
			break loop

		default:
			if resp.Choice[0].Delta.Reasoning != "" {
				fmt.Printf("\u001b[91m%s\u001b[0m", resp.Choice[0].Delta.Reasoning)
				reasoning = true
				continue
			}

			if reasoning {
				reasoning = false

				fmt.Println()
				if krn.ModelInfo().IsGPT {
					fmt.Println()
				}
			}

			fmt.Printf("%s", resp.Choice[0].Delta.Content)
		}
	}

	// -------------------------------------------------------------------------

	contextTokens := lr.Usage.InputTokens + lr.Usage.CompletionTokens
	contextWindow := krn.ModelConfig().ContextWindow
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\n\n\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Output: %d  Window: %d (%.0f%% of %.0fK) TPS: %.2f\u001b[0m\n",
		lr.Usage.InputTokens, lr.Usage.ReasoningTokens, lr.Usage.CompletionTokens, lr.Usage.OutputTokens, contextTokens, percentage, of, lr.Usage.TokensPerSecond)

	return messages, nil
}
```

This example can produce the following output:

```
$ make example-question
export LD_LIBRARY_PATH=:tests/libraries && \
	CGO_ENABLED=0 go run examples/question/main.go

Output:

- check llama.cpp installation: ✓
  - latest version : b7248
  - current version: b7248
- check "Qwen3-8B-Q8_0" installation: ✓
- system info:
	CPU:NEON, ARM_FMA:on, FP16_VA:on, DOTPROD:on, LLAMAFILE:on, ACCELERATE:on, REPACK:on, Metal:EMBED_LIBRARY,
- contextWindow: 40960
- embeddings   : false
- isGPT        : false

USER> hello model

MODEL> Okay, the user said "hello model". Let me think about how to respond.

First, I need to check if there's any function I should call here. The available tools are get_weather and invoke_cli_command. The user's message is just a greeting, so probably no function is needed.

The user might just be testing or starting a conversation. My role is to greet them back and offer help. I should keep it friendly and open-ended. Let me make sure there's no hidden request in their message. Nope, it's straightforward.

So, the best move is to acknowledge their greeting and ask how I can assist. That's standard for such interactions. No tool calls required here
.

Hello! How can I assist you today?

Input: 199  Reasoning: 143  Completion: 10  Output: 153  Window: 209 (1% of 40K) TPS: 46.54

USER>
```
