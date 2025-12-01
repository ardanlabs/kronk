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

Kronk uses models in the GGUF format supported by llamacpp. You can find many models in GGUF format on Hugging Face (over 147k at last count):

https://huggingface.co/models?library=gguf&sort=trending

## Support

Kronk currently has support for over 94% of llamacpp functionality thanks to yzma. See the yzma [ROADMAP.md](https://github.com/ardanlabs/yzma/blob/main/ROADMAP.md) for the complete list.

You can use multimodal models (image/audio) and text language models with full hardware acceleration on Linux, on macOS, and on Windows.

| OS      | CPU          | GPU                             |
| ------- | ------------ | ------------------------------- |
| Linux   | amd64, arm64 | CUDA, Vulkan, HIP, ROCm, SYCL   |
| macOS   | arm64        | Metal                           |
| Windows | amd64        | CUDA, Vulkan, HIP, SYCL, OpenCL |

Whenever there is a new release of llamacpp, the tests for yzma are run automatically. Kronk runs tests once a day and will check for updates to llamacpp. This helps us stay up to date with the latest code and models.

## Examples

There are examples in the examples direction:

_The first time you run these programs the system will download and install the model and libraries._

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

## Sample Example (Chat)

```go
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/examples/install"
	"github.com/ardanlabs/kronk/model"
	"github.com/hybridgroup/yzma/pkg/download"
)

const (
	modelURL       = "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf?download=true"
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

	if err := kronk.Init(libPath, kronk.LogSilent); err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(modelInstances, model.Config{
		ModelFile:  modelFile,
		Embeddings: true,
	})

	if err != nil {
		return fmt.Errorf("unable to create inference model: %w", err)
	}

	defer func() {
		fmt.Println("\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload model: %v", err)
		}
	}()

	// -------------------------------------------------------------------------

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	question := "Hello model"

	fmt.Println()
	fmt.Println("QUESTION:", question)
	fmt.Println()

	d := model.D{
		"messages": model.DocumentArray(
			model.ChatMessage("user", question),
		),
	}

	params := model.Params{
		Temperature: 0.7,
		TopP:        0.9,
		TopK:        40,
		MaxTokens:   2048,
	}

	ch, err := krn.ChatStreaming(ctx, params, d)
	if err != nil {
		return fmt.Errorf("chat streaming: %w", err)
	}

	// -------------------------------------------------------------------------

	var reasoning bool

	for resp := range ch {
		switch resp.Choice[0].FinishReason {
		case model.FinishReasonError:
			return fmt.Errorf("error from model: %s", resp.Choice[0].Delta.Content)

		case model.FinishReasonStop:
			return nil

		default:
			if resp.Choice[0].Delta.Reasoning != "" {
				reasoning = true
				fmt.Printf("\u001b[91m%s\u001b[0m", resp.Choice[0].Delta.Reasoning)
				continue
			}

			if reasoning {
				reasoning = false
				fmt.Println()
				continue
			}

			fmt.Printf("%s", resp.Choice[0].Delta.Content)
		}
	}

	return nil
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
```

This example can produce the following output:

```
$ export LD_LIBRARY_PATH=${LD_LIBRARY_PATH}:zarf/llamacpp
$ go run cmd/examples/example13/step1/*.go

Output:

- check llamacpp installation: ✓
  - latest version : b7209
  - current version: b7209
- check "gpt-oss-20b-Q8_0" installation: ✓
- contextWindow: 131072
- embeddings   : false
- isGPT        : false

USER> hello model

MODEL> User says "hello model". We should respond politely. There's no function call needed.

Hello! How can I help you today?

Input: 140  Reasoning: 17  Completion: 9  Output: 26  Window: 149 (0% of 128K) TPS: 33.51

USER> what is the weather in London, England

MODEL> We need to use the get_weather function.

Model Asking For Tool Call:
ToolID[d960e0fc-4867-4a30-94aa-421e7e0c73b2]: get_weather(map[location:London, England])

Input: 168  Reasoning: 9  Completion: 14  Output: 23  Window: 182 (0% of 128K) TPS: 19.07

USER>
```
