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

## Project Status

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

## Install Kronk

To install the Kronk tool run the following command:

```shell
$ go install github.com/ardanlabs/kronk/cmd/kronk@latest

$ kronk --help
```

## Issues/Features

Here is the existing [Issues/Features](https://github.com/ardanlabs/kronk/issues) for the project and the things being worked on or things that would be nice to have.

If you are interested in helping in any way, please send an email to [Bill Kennedy](mailto:bill@ardanlabs.com).

## Features

Here is the existing [FEATURES](FEATURES.md) list for the project. The features are not in any particular order.

## Architecture

The architecture of Kronk is designed to be simple and scalable.

Watch this [video](https://www.youtube.com/live/gjSrYkYc-yo) to learn more about the project and the architecture.

### SDK

The Kronk SDK allows you to write applications that can diectly interact with local open source GGUF models (supported by llama.cpp) that provide inference for text and media (vision and audio).

![api arch](./images/design/sdk.png?v1)

Check out the [examples](#examples) section below.

### Kronk Model Server (KMS)

If you want an OpenAI compatible model server, the Kronk model server leverages the power of the Kronk API to give you a concurrent and scalable web api.

Run `make kronk-server` to check it out.

#### Parallel Inference Options

Kronk supports concurrent request handling through the `NSeqMax` configuration value:

- **Text models**: `NSeqMax` controls parallel sequence processing within a single model instance. Multiple chat requests are batched together and processed simultaneously, improving throughput for high-concurrency workloads.

- **Sequential models** (embeddings, reranking, vision, audio): `NSeqMax` creates that many model instances in a pool. Each instance handles one request at a time, but multiple instances allow concurrent request handling.

#### Sequential Inference Archtecture

Sequential inference for multi-modal, embedding, and rerank models: multiple model instances (A and B) each handle requests through dedicated goroutines, sharing the underlying llama.cpp backend with one request processed at a time per instance.

You have the option in this mode to load multiple instances of the same model for parallel processing.

![sequential inference](./images/design/sequential-inference.png?v1)

#### Parallel Inference Archtecture

Parallel inference flow showing requests queued in a channel (NSeqMax × 2 capacity), batched in groups of 2, processed through concurrent goroutines, and executed via llama.cpp on the chat model.

![parallel inference](./images/design/parallel-inference.png?v1)

#### Model Caching

You can configure the number of Models that stay loaded in memory (default: 3) through configuration how long they will stay in memory (default: 5m) when not being used. This allows you to manage the resouces on the hardware.

For more details on the settings, run the following command after installing Kronk:

```shell
kronk server --help
```

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

## Kronk Model Server

The model server is OpenAI compatible and you can use OpenWebUI to interact with it. To start the Kronk model server run:

```shell
kronk server start
```

You will need to load a model if this is the first time you're using the system. To download a starter model run the catalog list command to see the current catalog of models:

```shell
kronk catalog list --local

CATALOG              MODEL ID                            PULLED   ENDPOINT
Audio-Text-to-Text   Qwen2-Audio-7B.Q8_0                 yes      chat_completion
Embedding            embeddinggemma-300m-qat-Q8_0        yes      embeddings
Image-Text-to-Text   gemma-3-4b-it-q4_0                  yes      chat_completion
Image-Text-to-Text   Qwen2.5-VL-3B-Instruct-Q8_0         yes      chat_completion
Text-Generation      gpt-oss-20b-Q8_0                    yes      chat_completion
Text-Generation      Llama-3.3-70B-Instruct-Q8_0         yes      chat_completion
Text-Generation      Qwen3-8B-Q8_0                       yes      chat_completion
Text-Generation      Qwen3-Coder-30B-A3B-Instruct-Q8_0   yes      chat_completion
```

Then download the `Qwen3-8B-Q8_0` model using the catalog pull command:

```shell
kronk catalog pull Qwen3-8B-Q8_0 --local
```

If you want to play with OpenWebUI, run the following commands:

```shell
make install-owu
make owu-up
```

The open your browser to `localhost:8081` or open another terminal window and run:

```shell
make owu-browse
```

## Sample API Program - Question Example

```go
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
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
		return fmt.Errorf("unable to installation system: %w", err)
	}

	krn, err := newKronk(mp)
	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	defer func() {
		fmt.Println("\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload model: %v", err)
		}
	}()

	if err := question(krn); err != nil {
		fmt.Println(err)
	}

	return nil
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
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
	// templates.Catalog().DownloadModel("Qwen3-8B-Q8_0")

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(model.Config{
		ModelFiles: mp.ModelFiles,
		CacheTypeK: model.GGMLTypeQ8_0,
		CacheTypeV: model.GGMLTypeQ8_0,
		NSeqMax:    2,
	})

	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\n\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("  - contextWindow:", krn.ModelConfig().ContextWindow)
	fmt.Println("  - embeddings   :", krn.ModelInfo().IsEmbedModel)
	fmt.Println("  - isGPT        :", krn.ModelInfo().IsGPTModel)

	return krn, nil
}

func question(krn *kronk.Kronk) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	question := "Hello model"

	fmt.Println()
	fmt.Println("QUESTION:", question)
	fmt.Println()

	d := model.D{
		"messages": model.DocumentArray(
			model.TextMessage(model.RoleUser, question),
		),
		"temperature": 0.7,
		"top_p":       0.9,
		"top_k":       40,
		"max_tokens":  2048,
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return fmt.Errorf("chat streaming: %w", err)
	}

	// -------------------------------------------------------------------------

	var reasoning bool

	for resp := range ch {
		switch resp.Choice[0].FinishReason() {
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

QUESTION: Hello model

Okay, the user said "Hello model." I need to respond appropriately. First, I should acknowledge their greeting. Since they mentioned "model," maybe they're referring to me as a language model. I should clarify that I'm Qwen, a large language model developed by Alibaba Cloud. I should keep the response friendly and open-ended, inviting them to ask questions or share topics they're interested in. Let me make sure the tone is welcoming and helpful. Also, check for any possible misunderstandings. They might be testing if I recognize the term "model," so confirming my identity as Qwen is important. Alright, time to put it all together in a natural, conversational way.

! I'm Qwen, a large language model developed by Alibaba Cloud. How can I assist you today? 😊 Whether you have questions, need help with something, or just want to chat, feel free to let me know!
Unloading Kronk
```
