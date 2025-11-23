![Kronk logo](images/project/kronk_logo.png?v4)

# Kronk

This project lets you use Go for hardware accelerated local inference with llama.cpp directly integrated into your applications. It provides a high level API based on the [yzma](https://github.com/hybridgroup/yzma) module.

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
	"strings"
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

	krn, err := kronk.New(concurrency, modelFile, "", kronk.ModelConfig{
		ContextWindow: 0,
		MaxTokens:     0,
		Embeddings:    false,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Println("- contextWindow:", krn.ModelConfig().ContextWindow)
	fmt.Println("- maxTokens    :", krn.ModelConfig().MaxTokens)
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
		TopK: 1.0,
		TopP: 0.9,
		Temp: 0.7,
	})
	if err != nil {
		return nil, fmt.Errorf("chat streaming: %w", err)
	}

	return ch, nil
}

func modelResponse(krn *kronk.Kronk, messages []kronk.ChatMessage, ch <-chan kronk.ChatResponse) ([]kronk.ChatMessage, error) {
	fmt.Print("\nMODEL> ")

	var finalResponse strings.Builder
	var contextTokens int
	var inputTokens int
	var outputTokens int

	now := time.Now()

	for msg := range ch {
		if msg.Err != nil {
			return messages, fmt.Errorf("error from model: %w", msg.Err)
		}

		fmt.Print(msg.Response)
		finalResponse.WriteString(msg.Response)

		contextTokens = msg.Tokens.Context
		inputTokens = msg.Tokens.Input
		outputTokens += msg.Tokens.Output
	}

	// -------------------------------------------------------------------------

	elapsedSeconds := time.Since(now).Seconds()
	tokensPerSecond := float64(outputTokens) / elapsedSeconds

	contextWindow := krn.ModelConfig().ContextWindow
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\n\n\u001b[90mInput: %d  Output: %d  Context: %d (%.0f%% of %.0fK) TPS: %.2f\u001b[0m\n",
		inputTokens, outputTokens, contextTokens, percentage, of, tokensPerSecond)

	messages = append(messages, kronk.ChatMessage{
		Role:    "assistant",
		Content: finalResponse.String(),
	})

	return messages, nil
}
```

This example can produce the following output:

````
$ export LD_LIBRARY_PATH=${LD_LIBRARY_PATH}:zarf/llamacpp
$ go run cmd/examples/example13/step1/*.go

Output:

- check llamacpp installation: ✓
- check "Qwen3-8B-Q8_0" installation: ✓
- contextWindow: 40960
- maxTokens    : 512
- embeddings   : false

USER> hello model

MODEL> <think>
Okay, the user said "hello model." I need to respond appropriately. First, I should acknowledge their greeting. Since they mentioned "model," maybe they're referring to me as a language model. I should clarify that I'm Qwen, a large language model developed by Alibaba Cloud. I should keep the response friendly and open-ended, inviting them to ask questions or share topics they're interested in. Also, I should make sure the tone is welcoming and helpful. Let me check if there's any specific context I need to consider. The user might be testing the model or just starting a conversation. I'll keep it simple and positive.
</think>

Hello! I'm Qwen, a large language model developed by Alibaba Cloud. How can I assist you today? Whether you have questions, need help with something, or just want to chat, feel free to let me know! 😊

Input: 22  Output: 180  Context: 202 (0% of 40K) TPS: 43.11

USER> write a hello world program in Go and only show the code

MODEL> <think>
Okay, the user asked for a "hello world" program in Go and wants only the code shown. Let me recall the basic structure of a Go program.

First, the package declaration. In Go, the main package is required for executable programs. So I'll start with "package main".

Next, the import section. The "fmt" package is needed for printing to the console. So I'll include "import "fmt"".

Then the main function. The main function is the entry point. So "func main() { ... }".

Inside the main function, use fmt.Println("Hello, World!") to print the message. That's the standard hello world.

Wait, the user said only show the code. So I need to make sure there's no extra text. Just the code block. Let me check the syntax again to avoid any errors. Yes, that's correct. The code should be concise and straightforward. No explanations, just the code. Alright, that should do it.
</think>

```go
package main

import "fmt"

func main() {
    fmt.Println("Hello, World!")
}
```

Input: 430  Output: 228  Context: 658 (2% of 40K) TPS: 42.97

USER>
````
