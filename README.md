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
[![macOS](https://github.com/ardanlabs/kronk/actions/workflows/macos.yml/badge.svg)](https://github.com/ardanlabs/kronk/actions/workflows/macos.yml)

## Owner Information

```
Name:    Bill Kennedy
Company: Ardan Labs
Title:   Managing Partner
Email:   bill@ardanlabs.com
Twitter: goinggodotnet
```

### Models

Kronk uses models in the GGUF format supported by llamacpp. You can find many models in GGUF format on Hugging Face (over 147k at last count):

https://huggingface.co/models?library=gguf&sort=trending

### Support

Kronk currently has support for over 94% of llamacpp functionality. See [ROADMAP.md](https://github.com/ardanlabs/yzma/blob/main/ROADMAP.md) on the yzma project for the complete list.

You can use multimodal models (image/audio) and text language models with full hardware acceleration on Linux, on macOS, and on Windows.

| OS      | CPU          | GPU                             |
| ------- | ------------ | ------------------------------- |
| Linux   | amd64, arm64 | CUDA, Vulkan, HIP, ROCm, SYCL   |
| macOS   | arm64        | Metal                           |
| Windows | amd64        | CUDA, Vulkan, HIP, SYCL, OpenCL |

Whenever there is a new release of llamacpp, the tests for yzma are run automatically. Kronk runs tests once a day and will check for updates to llamacpp. This helps us stay up to date with the latest code and models.

### Examples

You can find examples in the ArdanLabs AI training repo at example13:

https://github.com/ardanlabs/ai-training/tree/main/cmd/examples/example13

### Sample Example

This is an example from the ArdanLabs AI training repo at [example13-step1](https://github.com/ardanlabs/ai-training/tree/main/cmd/examples/example13/step1/main.go)

```go
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/ai-training/cmd/examples/example13/install"
	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/model"
	"github.com/hybridgroup/yzma/pkg/download"
)

const (
	modelURL = "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf?download=true"
	// modelURL  = "https://huggingface.co/unsloth/gpt-oss-20b-GGUF/resolve/main/gpt-oss-20b-Q8_0.gguf?download=true"
	libPath   = "zarf/llamacpp"
	modelPath = "zarf/models"
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
	defer krn.Unload()

	// -------------------------------------------------------------------------

	tools := tools()

	var messages []model.ChatMessage

	for {
		messages, err = userInput(messages)
		if err != nil {
			return fmt.Errorf("user input: %w", err)
		}

		messages, err = func() ([]model.ChatMessage, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			ch, err := performChat(ctx, krn, messages, tools)
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
	if err := install.LlamaCPP(libPath, download.CPU, true); err != nil {
		return "", fmt.Errorf("unable to install llamacpp: %w", err)
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

	const modelInstances = 1

	krn, err := kronk.New(modelInstances, model.Config{
		ModelFile: modelFile,
	})

	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Println("- contextWindow:", krn.ModelConfig().ContextWindow)
	fmt.Println("- embeddings   :", krn.ModelConfig().Embeddings)
	fmt.Println("- isGPT        :", krn.ModelInfo().IsGPT)

	return krn, nil
}

func userInput(messages []model.ChatMessage) ([]model.ChatMessage, error) {
	fmt.Print("\nUSER> ")

	reader := bufio.NewReader(os.Stdin)

	userInput, err := reader.ReadString('\n')
	if err != nil {
		return messages, fmt.Errorf("unable to read user input: %w", err)
	}

	messages = append(messages, model.ChatMessage{
		Role:    "user",
		Content: userInput,
	})

	return messages, nil
}

func tools() []model.Tool {
	tool := model.NewToolFunction(
		"get_weather",
		"Get the weather for a place",
		model.ToolParameter{
			Name:        "location",
			Type:        "string",
			Description: "The location to get the weather for, e.g. San Francisco, CA",
		},
	)

	return []model.Tool{tool}
}

func performChat(ctx context.Context, krn *kronk.Kronk, messages []model.ChatMessage, tools []model.Tool) (<-chan model.ChatResponse, error) {
	ch, err := krn.ChatStreaming(ctx, model.ChatRequest{
		Messages: messages,
		Tools:    tools,
		Params: model.Params{
			MaxTokens: 2048,
		},
	})

	if err != nil {
		return nil, fmt.Errorf("chat streaming: %w", err)
	}

	return ch, nil
}

func modelResponse(krn *kronk.Kronk, messages []model.ChatMessage, ch <-chan model.ChatResponse) ([]model.ChatMessage, error) {
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
			messages = append(messages, model.ChatMessage{
				Role:    "assistant",
				Content: resp.Choice[0].Delta.Content,
			})
			break loop

		case model.FinishReasonTool:
			fmt.Println()
			fmt.Printf("\u001b[92mModel Asking For Tool Call:\nToolID[%s]: %s(%s)\u001b[0m\n",
				resp.Choice[0].Delta.ToolCalls[0].ID,
				resp.Choice[0].Delta.ToolCalls[0].Name,
				resp.Choice[0].Delta.ToolCalls[0].Arguments,
			)

			messages = append(messages, model.ChatMessage{
				Role: "tool",
				Content: fmt.Sprintf("Tool call %s: %s(%v)",
					resp.Choice[0].Delta.ToolCalls[0].ID,
					resp.Choice[0].Delta.ToolCalls[0].Name,
					resp.Choice[0].Delta.ToolCalls[0].Arguments),
			})
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
			}

			fmt.Printf("%s", resp.Choice[0].Delta.Content)
		}
	}

	// -------------------------------------------------------------------------

	contextTokens := lr.Usage.InputTokens + lr.Usage.CompletionTokens
	contextWindow := krn.ModelConfig().ContextWindow
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\n\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Output: %d  Window: %d (%.0f%% of %.0fK) TPS: %.2f\u001b[0m\n",
		lr.Usage.InputTokens, lr.Usage.ReasoningTokens, lr.Usage.CompletionTokens, lr.Usage.OutputTokens, contextTokens, percentage, of, lr.Usage.TokensPerSecond)

	return messages, nil
}
```

This example can produce the following output:

```
$ export LD_LIBRARY_PATH=${LD_LIBRARY_PATH}:zarf/llamacpp
$ go run cmd/examples/example13/step1/*.go

Output:

- check llamacpp installation: ✓
  - latest version : b7192
  - current version: b7192
- check "Qwen3-8B-Q8_0" installation: ✓
- contextWindow: 40960
- embeddings   : false
- isGPT        : false

USER> hello model

MODEL> Okay, the user said "hello model". I need to respond appropriately. Since there's no specific query here, just a greeting, I should acknowledge their greeting and offer assistance. Let me check if any tools are needed. The available tool is get_weather, but the user didn't ask for weather. So, no function call required. Just a friendly reply.

Hello! How can I assist you today? If you have any questions or need information, feel free to ask!
Input: 141  Reasoning: 74  Completion: 24  Output: 98  Window: 165 (0% of 40K) TPS: 45.40

USER> what is the weather in NYC

MODEL> Okay, the user is asking for the weather in NYC. Let me check the tools provided. There's a function called get_weather that takes a location parameter. The location needs to be a string like "San Francisco, CA". The user mentioned NYC, which is New York City. I should format the location as "New York City, NY" to match the expected input. I'll call the get_weather function with that location. No other tools are available, so this is the only function needed. Let me make sure the JSON is correctly structured with the name and arguments.

Model Asking For Tool Call:
ToolID[8bd768c8-2b23-4c84-8baa-bcf8dcbc4f43]: get_weather(map[location:New York City, NY])

Input: 181  Reasoning: 117  Completion: 23  Output: 140  Window: 204 (0% of 40K) TPS: 46.36

USER>
```
