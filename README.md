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
	"github.com/hybridgroup/yzma/pkg/download"
)

const (
	modelURL  = "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf?download=true"
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

	var messages []kronk.ChatMessage

	for {
		messages, err = userInput(messages)
		if err != nil {
			return fmt.Errorf("user input: %w", err)
		}

		messages, err = func() ([]kronk.ChatMessage, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			ch, err := performChat(ctx, krn, messages)
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

	const concurrency = 1

	krn, err := kronk.New(concurrency, modelFile, "", kronk.ModelConfig{})
	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Println("- contextWindow:", krn.ModelConfig().ContextWindow)
	fmt.Println("- embeddings   :", krn.ModelConfig().Embeddings)

	return krn, nil
}

func userInput(messages []kronk.ChatMessage) ([]kronk.ChatMessage, error) {
	fmt.Print("\nUSER> ")

	reader := bufio.NewReader(os.Stdin)

	userInput, err := reader.ReadString('\n')
	if err != nil {
		return messages, fmt.Errorf("unable to read user input: %w", err)
	}

	messages = append(messages, kronk.ChatMessage{
		Role:    "user",
		Content: userInput,
	})

	return messages, nil
}

func performChat(ctx context.Context, krn *kronk.Kronk, messages []kronk.ChatMessage) (<-chan kronk.ChatResponse, error) {
	ch, err := krn.ChatStreaming(ctx, messages, kronk.Params{
		MaxTokens: 2048,
	})
	if err != nil {
		return nil, fmt.Errorf("chat streaming: %w", err)
	}

	return ch, nil
}

func modelResponse(krn *kronk.Kronk, messages []kronk.ChatMessage, ch <-chan kronk.ChatResponse) ([]kronk.ChatMessage, error) {
	fmt.Print("\nMODEL> ")

	var lr kronk.ChatResponse
	for resp := range ch {
		if resp.Choice[0].FinishReason == kronk.FinishReasonError {
			return messages, fmt.Errorf("error from model: %s", resp.Choice[0].GeneratedText)
		}

		fmt.Print(resp.Choice[0].Delta.Content)
		lr = resp
	}

	messages = append(messages, kronk.ChatMessage{
		Role:    "assistant",
		Content: lr.Choice[0].GeneratedText,
	})

	// -------------------------------------------------------------------------

	contextTokens := lr.Usage.InputTokens + lr.Usage.CompletionTokens
	contextWindow := krn.ModelConfig().ContextWindow
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\n\n\u001b[90mInput: %d  Output: %d  Context: %d (%.0f%% of %.0fK) TPS: %.2f\u001b[0m\n",
		lr.Usage.InputTokens, lr.Usage.OutputTokens, contextTokens, percentage, of, lr.Usage.TokensPerSecond)

	return messages, nil
}
```

This example can produce the following output:

```
$ export LD_LIBRARY_PATH=${LD_LIBRARY_PATH}:zarf/llamacpp
$ go run cmd/examples/example13/step1/*.go

Output:

- check llamacpp installation: ✓
  - latest version : b7151
  - current version: b7151
- check "Qwen3-8B-Q8_0" installation: ✓
- contextWindow: 40960
- embeddings   : false

USER> hello model

MODEL> <think>
Okay, the user said "hello model". I need to respond appropriately. First, I should acknowledge their greeting. Since they used "model", maybe they're referring to me as an AI model. I should confirm that and offer assistance. Let me make sure to keep the tone friendly and open. I can ask how I can help them today. Also, maybe check if they have any specific questions or need information on a particular topic. Keep it simple and welcoming.
</think>

Hello! I'm Qwen, a large language model developed by Alibaba Cloud. How can I assist you today? Whether you have questions, need information, or just want to chat, feel free to let me know! 😊

Input: 22  Output: 143  Context: 165 (0% of 40K) TPS: 45.97

USER>
```
