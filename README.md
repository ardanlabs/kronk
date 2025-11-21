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
	modelURL  = "https://huggingface.co/Qwen/Qwen2.5-0.5B-Instruct-GGUF/resolve/main/qwen2.5-0.5b-instruct-q8_0.gguf?download=true"
	libPath   = "zarf/llamacpp"
	modelPath = "zarf/models"
)

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	if err := install.LlamaCPP(libPath, download.CPU, true); err != nil {
		return fmt.Errorf("unable to install llamacpp: %w", err)
	}

	modelFile, err := install.Model(modelURL, modelPath)
	if err != nil {
		return fmt.Errorf("unable to install model: %w", err)
	}

	// -------------------------------------------------------------------------

	if err := kronk.Init(libPath, kronk.LogSilent); err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	const concurrency = 1

	llm, err := kronk.New(concurrency, modelFile, kronk.Config{
		ContextWindow: 1024 * 32,
	})
	if err != nil {
		return fmt.Errorf("unable to create inference model: %w", err)
	}
	defer llm.Unload()

	// -------------------------------------------------------------------------

	var messages []kronk.ChatMessage

	for {
		fmt.Print("\nUSER> ")

		reader := bufio.NewReader(os.Stdin)

		userInput, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("unable to read user input", err.Error())
			os.Exit(1)
		}

		messages = append(messages, kronk.ChatMessage{
			Role:    "user",
			Content: userInput,
		})

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		params := kronk.Params{
			TopK: 1.0,
			TopP: 0.9,
			Temp: 0.7,
		}

		ch, err := llm.ChatCompletions(ctx, messages, params)
		if err != nil {
			return fmt.Errorf("chat completions: %w", err)
		}

		fmt.Print("\nMODEL> ")

		var finalResponse strings.Builder
		for msg := range ch {
			if msg.Err != nil {
				return fmt.Errorf("error from model: %w", msg.Err)
			}

			fmt.Print(msg.Response)
			finalResponse.WriteString(msg.Response)
		}

		messages = append(messages, kronk.ChatMessage{
			Role:    "assistant",
			Content: finalResponse.String(),
		})

		fmt.Println()
	}
}
```

This example can produce the following output:

````
$ export LD_LIBRARY_PATH=${LD_LIBRARY_PATH}:zarf/llamacpp
$ go run cmd/examples/example13/step1/*.go 2>/dev/null

Output:

- check llamacpp installation: ✓
- check "qwen2.5-0.5b-instruct-q8_0" installation: ✓

USER> hello model

MODEL> Hello! How can I assist you today?

USER> can you write a hello world program in Go?

MODEL> Sure! Here's a simple "Hello, World!" program written in Go:

```go
package main

import "fmt"

func main() {
    fmt.Println("Hello, World!")
}
```

To run this program, you can use a Go compiler like `go build`:

```sh
go build
```

Then, you can run the compiled program:

```sh
./hello_world
```

This will output:

```
Hello, World!
```

USER>
````
