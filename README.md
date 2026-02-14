![kronk logo](./images/project/kronk_banner.jpg?v5)

Copyright 2025 Ardan Labs  
hello@ardanlabs.com

# Kronk

This project lets you use Go for hardware accelerated local inference with llama.cpp directly integrated into your applications via the [yzma](https://github.com/hybridgroup/yzma) module. Kronk provides a high-level API that feels similar to using an OpenAI compatible API.

This project also provides a model server for chat completions, responses, messages, embeddings, and reranking. The server is compatible with the OpebWebUI, Cline, and the Claude Code project.

Here is the current [catalog](https://github.com/ardanlabs/kronk_catalogs) of models that have been verified to work with Kronk.

To see all the documentation, clone the project and run the Kronk Model Server:

```shell
$ make kronk-server

$ make website
```

You can also install Kronk, run the Kronk Model Server, and open the browser to localhost:8080

```shell
$ go install github.com/ardanlabs/kronk/cmd/kronk@latest

$ kronk server start
```

Read the [Manual](./MANUAL.md) to learn more about running the Kronk Model Server.

## Project Status

[![Go Reference](https://pkg.go.dev/badge/github.com/ardanlabs/kronk.svg)](https://pkg.go.dev/github.com/ardanlabs/kronk)
[![Go Report Card](https://goreportcard.com/badge/github.com/ardanlabs/kronk)](https://goreportcard.com/report/github.com/ardanlabs/kronk)
[![go.mod Go version](https://img.shields.io/github/go-mod/go-version/ardanlabs/kronk)](https://github.com/ardanlabs/kronk)
[![llama.cpp Release](https://img.shields.io/github/v/release/ggml-org/llama.cpp?label=llama.cpp)](https://github.com/ggml-org/llama.cpp/releases)

[![Linux](https://github.com/ardanlabs/kronk/actions/workflows/linux.yml/badge.svg)](https://github.com/ardanlabs/kronk/actions/workflows/linux.yml)

## Owner Information

```
Name:     Bill Kennedy
Company:  Ardan Labs
Title:    Managing Partner
Email:    bill@ardanlabs.com
BlueSky:  https://bsky.app/profile/goinggo.net
LinkedIn: www.linkedin.com/in/william-kennedy-5b318778/
Twitter:  https://x.com/goinggodotnet
```

## Install Kronk

To install the Kronk tool run the following command:

```shell
$ go install github.com/ardanlabs/kronk/cmd/kronk@latest

$ kronk --help
```

## Issues/Features

Here is the existing [Issues/Features](https://github.com/ardanlabs/kronk/issues) for the project and the things being worked on or things that would be nice to have.

If you are interested in helping in any way, please send an email to [Bill Kennedy](mailto:bill@ardanlabs.com).

## Architecture

The architecture of Kronk is designed to be simple and scalable.

Watch this [video](https://www.youtube.com/live/gjSrYkYc-yo) to learn more about the project and the architecture.

### SDK

The Kronk SDK allows you to write applications that can diectly interact with local open source GGUF models (supported by llama.cpp) that provide inference for text and media (vision and audio).

![api arch](./images/design/sdk.png?v1)

Check out the [examples](#examples) section below.

## Models

Kronk uses models in the GGUF format supported by llama.cpp. You can find many models in GGUF format on Hugging Face (over 147k at last count):

models?library=gguf&sort=trending

## Support

Kronk currently has support for over 94% of llama.cpp functionality thanks to yzma. See the yzma [ROADMAP.md](https://github.com/hybridgroup/yzma/blob/main/ROADMAP.md) for the complete list.

You can use multimodal models (image/audio) and text language models with full hardware acceleration on Linux, on macOS, and on Windows.

| OS      | CPU          | GPU                             |
| ------- | ------------ | ------------------------------- |
| Linux   | amd64, arm64 | CUDA, Vulkan, HIP, ROCm, SYCL   |
| macOS   | arm64        | Metal                           |
| Windows | amd64        | CUDA, Vulkan, HIP, SYCL, OpenCL |

Whenever there is a new release of llama.cpp, the tests for yzma are run automatically. Kronk runs tests once a day and will check for updates to llama.cpp. This helps us stay up to date with the latest code and models.

## API Examples

There are examples in the examples direction:

_The first time you run these programs the system will download and install the model and libraries._

[AUDIO](examples/audio/main.go) - This example shows you how to execute a simple prompt against an audio model.

```shell
make example-audio
```

[CHAT](examples/chat/main.go) - This example shows you how to chat with the chat-completion api.

```shell
make example-chat
```

[EMBEDDING](examples/embedding/main.go) - This example shows you a basic program using Kronk to perform an embedding operation.

```shell
make example-embedding
```

[GRAMMAR](examples/grammar/main.go) - This example shows how to use GBNF grammars to constrain model output.

```shell
make example-grammar
```

[QUESTION](examples/question/main.go) - This example shows you how to ask a simple question with the chat-completion api.

```shell
make example-question
```

[RERANK](examples/rerank/main.go) - This example shows you how to use a rerank model.

```shell
make example-rerank
```

[RESPONSE](examples/response/main.go) - This example shows you how to chat with the response api.

```shell
make example-question
```

[VISION](examples/vision/main.go) - This example shows you how to execute a simple prompt against a vision model.

```shell
make example-vision
```

[YZMA](examples/yzma/main.go) - This example shows you how to use the yzma api at it's basic level.

```shell
make example-yzma
```

You can find more examples in the ArdanLabs AI training repo at [Example13](https://github.com/ardanlabs/ai-training/tree/main/cmd/examples/example13).

## Sample API Program - Question Example

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

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

const modelURL = "Qwen/Qwen3-8B-GGUF/Qwen3-8B-Q8_0.gguf"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("run: unable to installation system: %w", err)
	}

	krn, err := newKronk(mp)
	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	defer func() {
		fmt.Println("\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("run: failed to unload model: %v", err)
		}
	}()

	if err := chat(krn); err != nil {
		return err
	}

	return nil
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithVersion(defaults.LibVersion("")),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	// -------------------------------------------------------------------------
	// This is not mandatory if you won't be using models from the catalog. That
	// being said, if you are using a model that is part of the catalog with
	// a corrected jinja file, having the catalog system up to date will allow
	// the system to pull that jinja file.

	ctlg, err := catalog.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to create catalog system: %w", err)
	}

	if err := ctlg.Download(ctx); err != nil {
		return models.Path{}, fmt.Errorf("unable to download catalog: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	mp, err := mdls.Download(ctx, kronk.FmtLogger, modelURL, "")
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install model: %w", err)
	}

	// -------------------------------------------------------------------------

	// You could also download this model using the catalog system.
	// mp, err := ctlg.DownloadModel(ctx, kronk.FmtLogger, "Qwen3-8B-Q8_0")
	// if err != nil {
	// 	return models.Path{}, fmt.Errorf("unable to download model: %w", err)
	// }

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	fmt.Println("loading model...")

	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	cfg := model.Config{
		ModelFiles: mp.ModelFiles,
	}

	krn, err := kronk.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\n\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow:", krn.ModelConfig().ContextWindow)
	fmt.Printf("- k/v          : %s/%s\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
	fmt.Println("- nBatch       :", krn.ModelConfig().NBatch)
	fmt.Println("- nuBatch      :", krn.ModelConfig().NUBatch)
	fmt.Println("- embeddings   :", krn.ModelInfo().IsEmbedModel)
	fmt.Println("- isGPT        :", krn.ModelInfo().IsGPTModel)
	fmt.Println("- template     :", krn.ModelInfo().Template.FileName)
	fmt.Println("- grammar      :", krn.ModelConfig().DefaultParams.Grammar != "")

	return krn, nil
}

func chat(krn *kronk.Kronk) error {
	messages := model.DocumentArray()

	var systemPrompt = `
		You are a helpful AI assistant. You are designed to help users answer
		questions, create content, and provide information in a helpful and
		accurate manner. Always follow the user's instructions carefully and
		respond with clear, concise, and well-structured answers. You are a
		helpful AI assistant. You are designed to help users answer questions,
		create content, and provide information in a helpful and accurate manner.
		Always follow the user's instructions carefully and respond with clear,
		concise, and well-structured answers. You are a helpful AI assistant.
		You are designed to help users answer questions, create content, and
		provide information in a helpful and accurate manner. Always follow the
		user's instructions carefully and respond with clear, concise, and
		well-structured answers.`

	messages = append(messages,
		model.TextMessage(model.RoleSystem, systemPrompt),
	)

	for {
		var err error
		messages, err = userInput(messages)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("run:user input: %w", err)
		}

		messages, err = func() ([]model.D, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()

			d := model.D{
				"messages":   messages,
				"tools":      toolDocuments(),
				"max_tokens": 2048,
			}

			ch, err := performChat(ctx, krn, d)
			if err != nil {
				return nil, fmt.Errorf("run: unable to perform chat: %w", err)
			}

			messages, err = modelResponse(krn, messages, ch)
			if err != nil {
				return nil, fmt.Errorf("run: model response: %w", err)
			}

			return messages, nil
		}()

		if err != nil {
			return fmt.Errorf("run: unable to perform chat: %w", err)
		}
	}
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
		model.TextMessage(model.RoleUser, userInput),
	)

	return messages, nil
}

func toolDocuments() []model.D {
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

		if len(resp.Choice) == 0 {
			continue
		}

		switch resp.Choice[0].FinishReason() {
		case model.FinishReasonError:
			return messages, fmt.Errorf("error from model: %s", resp.Choice[0].Delta.Content)

		case model.FinishReasonStop:
			break loop

		case model.FinishReasonTool:
			fmt.Println()
			if krn.ModelInfo().IsGPTModel {
				fmt.Println()
			}

			fmt.Printf("\u001b[92mModel Asking For Tool Calls:\n\u001b[0m")

			for _, tool := range resp.Choice[0].Delta.ToolCalls {
				fmt.Printf("\u001b[92mToolID[%s]: %s(%s)\n\u001b[0m",
					tool.ID,
					tool.Function.Name,
					tool.Function.Arguments,
				)

				messages = append(messages,
					model.TextMessage("tool", fmt.Sprintf("Tool call %s: %s(%v)\n",
						tool.ID,
						tool.Function.Name,
						tool.Function.Arguments),
					),
				)
			}

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
				if krn.ModelInfo().IsGPTModel {
					fmt.Println()
				}
			}

			fmt.Printf("%s", resp.Choice[0].Delta.Content)
		}
	}

	// -------------------------------------------------------------------------

	contextTokens := lr.Usage.PromptTokens + lr.Usage.CompletionTokens
	contextWindow := krn.ModelConfig().ContextWindow
	percentage := (float64(contextTokens) / float64(contextWindow)) * 100
	of := float32(contextWindow) / float32(1024)

	fmt.Printf("\n\n\u001b[90mInput: %d  Reasoning: %d  Completion: %d  Output: %d  Window: %d (%.0f%% of %.0fK) TPS: %.2f\u001b[0m\n",
		lr.Usage.PromptTokens, lr.Usage.ReasoningTokens, lr.Usage.CompletionTokens, lr.Usage.OutputTokens, contextTokens, percentage, of, lr.Usage.TokensPerSecond)

	return messages, nil
}
```

This example can produce the following output:

```shell
make example-question

CGO_ENABLED=0 go run examples/question/main.go
download-libraries: status[check libraries version information] arch[arm64] os[darwin] processor[cpu]
download-libraries: status[check llama.cpp installation] arch[arm64] os[darwin] processor[cpu] latest[b7406] current[b7406]
download-libraries: status[already installed] latest[b7406] current[b7406]
download-model: model-url[Qwen/Qwen3-8B-GGUF/Qwen3-8B-Q8_0.gguf] proj-url[] model-id[Qwen3-8B-Q8_0]:
download-model: waiting to check model status...:
download-model: status[already exists]:
loading model...

QUESTION: Hello model

Okay, the user said "Hello model." I need to respond appropriately. First, I should acknowledge their greeting. Since they mentioned "model," maybe they're referring to me as a language model. I should clarify that I'm Qwen, a large language model developed by Alibaba Cloud. I should keep the response friendly and open-ended, inviting them to ask questions or share topics they're interested in. Let me make sure the tone is welcoming and helpful. Also, check for any possible misunderstandings. They might be testing if I recognize the term "model," so confirming my identity as Qwen is important. Alright, time to put it all together in a natural, conversational way.

! I'm Qwen, a large language model developed by Alibaba Cloud. How can I assist you today? 😊 Whether you have questions, need help with something, or just want to chat, feel free to let me know!
Unloading Kronk
```
