![kronk logo](./images/project/kronk_banner.jpg?v5)

Copyright 2025-2026 Ardan Labs

hello@ardanlabs.com

https://kronkai.com

# Kronk

This project lets you use Go for hardware accelerated local inference with llama.cpp and whisper.cpp directly integrated into your Go applications via the [yzma](https://github.com/hybridgroup/yzma) and [bucky](https://github.com/ardanlabs/bucky) modules. Kronk provides a high-level API that feels similar to using an OpenAI compatible API.

This project also provides a model server for chat completions, responses, messages, embeddings, reranking, and audio transcription. The server is compatible with OpenWebUI, OpenCode, and the Claude Code project.

To see all the documentation, clone the project and run the Kronk Model Server:

```shell
$ make kronk-server

$ make website
```

You can also install Kronk, run the Kronk Model Server, and open the browser to localhost:11435

On macOS or Linux with Homebrew:

```shell
$ brew tap ardanlabs/kronk
$ brew trust ardanlabs/kronk
$ brew install kronk

$ kronk server start
```

Or with Go:

```shell
$ go install github.com/ardanlabs/kronk/cmd/kronk@latest

$ kronk server start
```

Read the [Manual](./manual) to learn more about running the Kronk Model Server.

## Project Status

[![Go Reference](https://pkg.go.dev/badge/github.com/ardanlabs/kronk.svg)](https://pkg.go.dev/github.com/ardanlabs/kronk)
[![Go Report Card](https://goreportcard.com/badge/github.com/ardanlabs/kronk)](https://goreportcard.com/report/github.com/ardanlabs/kronk)
[![go.mod Go version](https://img.shields.io/github/go-mod/go-version/ardanlabs/kronk)](https://github.com/ardanlabs/kronk)
[![llama.cpp Release](https://img.shields.io/github/v/release/ggml-org/llama.cpp?label=llama.cpp)](https://github.com/ggml-org/llama.cpp/releases)

[![Linux](https://github.com/ardanlabs/kronk/actions/workflows/linux.yml/badge.svg)](https://github.com/ardanlabs/kronk/actions/workflows/linux.yml)

Sometimes there are breaking changes to llama.cpp that require an update to yzma and Kronk. Here are some of the known compatible versions:

As of May 15th, 2026 please use version b9163 until we can fix the problems with b9165+

You can use this environment variable: `export KRONK_LIB_VERSION=b9163`

| llama.cpp | yzma    | kronk  |
| --------- | ------- | ------ |
| b8864     | v1.12.0 | 1.23.1 |
| b8865+    | v1.13.0 | 1.23.2 |
| b9180+    | v1.14.0 | 1.25.8 |
| b9460+    | v1.15.0 | 1.26.7 |
| b9549+    | v1.16.1 | 1.27.4 |
| b9562+    | v1.17.0 | 1.27.6 |
| b9616+    | v1.17.1 | 1.27.9 |

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

The recommended way to install Kronk on macOS or Linux is with Homebrew:

```shell
$ brew tap ardanlabs/kronk
$ brew trust ardanlabs/kronk
$ brew install kronk

$ kronk --help
```

To upgrade later:

```shell
$ brew upgrade kronk
```

You can also install via Go on any supported platform:

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

The Kronk SDK allows you to write applications that can diectly interact with local open source GGUF models (supported by llama.cpp) that provide inference for text and media (vision and audio). The Bucky SDK provides the same surface for speech-to-text via whisper.cpp — see the [Bucky chapter](.manual/chapter-18-bucky.md).

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./images/project/sdk-dark.png">
  <source media="(prefers-color-scheme: light)" srcset="./images/project/sdk-light.png">
  <img alt="Kronk SDK Architecture" src="./images/project/sdk-light.png">
</picture>

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

[AGENT](examples/agent/main.go) - This example shows you how to write a small coding agent.

```shell
make example-agent
```

[AUDIO](examples/audio/main.go) - This example shows you how to execute a simple prompt against an audio model.

```shell
make example-audio
```

[BUCKY](examples/bucky/main.go) - This example shows you how to transcribe an audio file with the bucky SDK (whisper.cpp under the hood). See the manual chapter [Bucky (Audio Transcription)](.manual/chapter-18-bucky.md) for the full subsystem reference.

```shell
make example-bucky
```

[BUCKY-STREAM](examples/bucky-stream/main.go) - This example shows you how to do live microphone transcription with the bucky streaming SDK: partials are revised in place and finals commit as you speak. Say "STOP" to end. See [Streaming Transcription](.manual/chapter-18-bucky.md#189-streaming-transcription-sdk) in the manual.

```shell
make example-bucky-stream
```

[BUCKY-DIAR](examples/bucky-diar/main.go) - This example shows you how to do channel-separated speaker diarization with the bucky SDK: each speaker is recorded on a dedicated channel, and `TranscribeChannelsFile` transcribes every channel on its own and merges the results into one time-sorted transcript tagged by speaker. See [Channel-Separated Diarization](.manual/chapter-18-bucky.md#188-sdk-quick-start) in the manual.

```shell
make example-bucky-diar
```

[CHAT](examples/chat/main.go) - This example shows you how to chat with the chat-completion api.

```shell
make example-chat
```

[CONCURRENCY](examples/concurrency/main.go) - This example shows you how to leverage concurrency using vision models.

```shell
make example-concurrency
```

[EMBEDDING](examples/embedding/main.go) - This example shows you a basic program using Kronk to perform an embedding operation.

```shell
make example-embedding
```

[GRAMMAR](examples/grammar/main.go) - This example shows how to use GBNF grammars to constrain model output.

```shell
make example-grammar
```

[POOL](examples/pool/main.go) - This example shows you how to use the pool package to manage multipl models in memory at the same time.

```shell
make example-pool
```

[QUESTION](examples/question/main.go) - This example shows you how to ask a simple question with the chat-completion api.

```shell
make example-question
```

[RAG](examples/rag/main.go) - This example shows you a complete RAG application.

```shell
make example-rag
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
// This example shows how to use GBNF grammars to constrain model output.
// Grammars force the model to only produce tokens that match the specified
// pattern, guaranteeing structured output.
//
// Run the example like this from the root of the project:
// $ make example-grammar

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

var grammarJSONObject = `root ::= object
value ::= object | array | string | number | "true" | "false" | "null"
object ::= "{" ws ( string ":" ws value ("," ws string ":" ws value)* )? ws "}"
array ::= "[" ws ( value ("," ws value)* )? ws "]"
string ::= "\"" ([^"\\] | "\\" ["\\bfnrt/] | "\\u" [0-9a-fA-F]{4})* "\""
number ::= "-"? ("0" | [1-9][0-9]*) ("." [0-9]+)? ([eE] [+-]? [0-9]+)?
ws ::= [ \t\n\r]*`

// modelSource is the model to download. It may be a HuggingFace URL,
// a canonical "provider/modelID", or a bare model id.
var modelSource = "unsloth/Qwen3-0.6B-Q8_0"

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

	// -------------------------------------------------------------------------
	// Example 1: Using a grammar preset (GrammarJSONObject)

	fmt.Println("=== Example 1: Grammar Preset (JSON Object) ===")
	if err := grammarPreset(krn); err != nil {
		fmt.Println(err)
	}

	// -------------------------------------------------------------------------
	// Example 2: Using a JSON Schema to auto-generate grammar

	fmt.Println("\n=== Example 2: JSON Schema ===")
	if err := jsonSchema(krn); err != nil {
		fmt.Println(err)
	}

	// -------------------------------------------------------------------------
	// Example 3: Custom grammar for constrained choices

	fmt.Println("\n=== Example 3: Custom Grammar (Sentiment Analysis) ===")
	if err := customGrammar(krn); err != nil {
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

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to init models: %w", err)
	}

	fmt.Println("Downloading model:", modelSource)

	mp, err := mdls.Download(ctx, kronk.FmtLogger, modelSource)
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install model: %w", err)
	}

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	fmt.Println("loading model...")

	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(
		model.WithModelFiles(mp.ModelFiles),
		model.WithAutoTune(true),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\n\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow  :", krn.ModelConfig().ContextWindow())
	fmt.Printf("- k/v            : %s/%s\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
	fmt.Println("- flashAttention :", krn.ModelConfig().FlashAttention)
	fmt.Println("- nBatch         :", krn.ModelConfig().NBatch())
	fmt.Println("- nuBatch        :", krn.ModelConfig().NUBatch())
	fmt.Println("- modelType      :", krn.ModelInfo().Type)
	fmt.Println("- isGPT          :", krn.ModelInfo().IsGPTModel)
	fmt.Println("- template       :", krn.ModelInfo().Template.FileName)
	fmt.Println("- grammar        :", krn.ModelConfig().DefaultParams.Grammar != "")
	fmt.Println("- nSeqMax        :", krn.ModelConfig().NSeqMax())
	fmt.Println("- vramTotal      :", krn.ModelInfo().VRAMTotal/(1024*1024), "MiB")
	fmt.Println("- slotMemory     :", krn.ModelInfo().SlotMemory/(1024*1024), "MiB")
	fmt.Println("- modelSize      :", krn.ModelInfo().Size/(1000*1000), "MB")
	fmt.Println("- imc            :", krn.ModelConfig().IncrementalCache())
	if n := krn.ModelConfig().PtrNGpuLayers; n != nil {
		fmt.Println("- nGPULayers     :", *n)
	} else {
		fmt.Println("- nGPULayers     : all")
	}
	if sm := krn.ModelConfig().PtrSplitMode; sm != nil {
		fmt.Println("- splitMode      :", sm)
	} else {
		fmt.Println("- splitMode      : auto")
	}

	return krn, nil
}

// grammarPreset demonstrates using a built-in grammar preset to force JSON output.
func grammarPreset(krn *kronk.Kronk) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	prompt := "List 3 programming languages with their year of creation. Respond in JSON format."

	fmt.Println("PROMPT:", prompt)
	fmt.Println()

	d := model.D{
		"messages": model.DocumentArray(
			model.TextMessage(model.RoleUser, prompt),
		),
		"grammar":         grammarJSONObject,
		"enable_thinking": false, // Grammar requires output to match from first token
		"temperature":     0.7,
		"max_tokens":      512,
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return fmt.Errorf("chat streaming: %w", err)
	}

	fmt.Print("RESPONSE: ")

	for resp := range ch {
		switch resp.Choices[0].FinishReason() {
		case model.FinishReasonError:
			return fmt.Errorf("error from model: %s", resp.Choices[0].Delta.Content)

		case model.FinishReasonStop:
			fmt.Println()
			return nil

		default:
			fmt.Print(resp.Choices[0].Delta.Content)
		}
	}

	return nil
}

// jsonSchema demonstrates using a JSON Schema to auto-generate a grammar.
// This gives you more control over the exact structure of the output.
func jsonSchema(krn *kronk.Kronk) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	prompt := "Describe the Go programming language."

	fmt.Println("PROMPT:", prompt)
	fmt.Println()

	// Define the expected output structure using JSON Schema.
	schema := model.D{
		"type": "object",
		"properties": model.D{
			"name": model.D{
				"type": "string",
			},
			"year": model.D{
				"type": "integer",
			},
			"paradigm": model.D{
				"type": "string",
				"enum": []string{"procedural", "object-oriented", "functional", "concurrent"},
			},
			"compiled": model.D{
				"type": "boolean",
			},
		},
		"required": []string{"name", "year", "paradigm", "compiled"},
	}

	d := model.D{
		"messages": model.DocumentArray(
			model.TextMessage(model.RoleUser, prompt),
		),
		"json_schema":     schema,
		"enable_thinking": false, // Grammar requires output to match from first token
		"temperature":     0.7,
		"max_tokens":      256,
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return fmt.Errorf("chat streaming: %w", err)
	}

	fmt.Print("RESPONSE: ")

	for resp := range ch {
		switch resp.Choices[0].FinishReason() {
		case model.FinishReasonError:
			return fmt.Errorf("error from model: %s", resp.Choices[0].Delta.Content)

		case model.FinishReasonStop:
			fmt.Println()
			return nil

		default:
			fmt.Print(resp.Choices[0].Delta.Content)
		}
	}

	return nil
}

// customGrammar demonstrates writing a custom GBNF grammar to constrain
// output to specific choices. This is useful for classification tasks.
func customGrammar(krn *kronk.Kronk) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Custom grammar that only allows specific sentiment values.
	// The model MUST output one of these exact strings.
	sentimentGrammar := `root ::= sentiment
sentiment ::= "positive" | "negative" | "neutral"`

	prompt := `Analyze the sentiment of this text and respond with exactly one word.

Text: "I absolutely love this product! It exceeded all my expectations and I would recommend it to everyone."

Sentiment:`

	fmt.Println("PROMPT:", prompt)
	fmt.Println()

	d := model.D{
		"messages": model.DocumentArray(
			model.TextMessage(model.RoleUser, prompt),
		),
		"grammar":         sentimentGrammar,
		"enable_thinking": false, // Grammar requires output to match from first token
		"temperature":     0.0,
		"max_tokens":      16,
	}

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return fmt.Errorf("chat streaming: %w", err)
	}

	fmt.Print("RESPONSE: ")

	for resp := range ch {
		switch resp.Choices[0].FinishReason() {
		case model.FinishReasonError:
			return fmt.Errorf("error from model: %s", resp.Choices[0].Delta.Content)

		case model.FinishReasonStop:
			fmt.Println()
			return nil

		default:
			fmt.Print(resp.Choices[0].Delta.Content)
		}
	}

	return nil
}
```

This example can produce the following output:

```shell
$ make example-question
cd examples && go run ./question/main.go
loading model...
- system info:
	REPACK:on, MTL:EMBED_LIBRARY, CPU:NEON, ARM_FMA:on, DOTPROD:on, LLAMAFILE:on, ACCELERATE:on,
- contextWindow  : 32768
- k/v            : f16/f16
- flashAttention : auto
- nBatch         : 2048
- nuBatch        : 512
- modelType      : dense
- isGPT          : false
- template       : tokenizer.chat_template
- grammar        : false
- nSeqMax        : 1
- vramTotal      : 4493 MiB
- slotMemory     : 3584 MiB
- modelSize      : 633 MB
- imc            : true
- nGPULayers     : all
- splitMode      : layer

QUESTION: Hello model

Okay, the user just said "Hello model". I need to respond appropriately. Since they didn't ask a question, I should acknowledge their message and offer help. Let me make sure the response is friendly and open-ended. I should check if there's anything specific they need, but keep it general. Maybe mention availability and willingness to assist further. Alright, that should work.

! How can I assist you today? 😊
Unloading Kronk
```

## Travel Schedule

Come find me in any of these cities or events this year. I will be giving workshops and talks about Kronk

| Dates           | Event                      | Location              | Comments       |
| --------------- | -------------------------- | --------------------- | -------------- |
| Jan 29th - 2nd  | AI Plumbers Fringe, FOSDEM | Brussels, Belgium     | Talk           |
| Mar 4th - 5th   | Ardan Connect              | São Paulo, Brazil     | Workshop       |
| Apr 20th - 25th | Gophercamp 2026            | Brno, Czech Republic  | Workshop, Talk |
| Apr 27th - 29th | AI Dev 26                  | San Francisco, USA    | Attendee       |
| May 17th - 23rd | Gophercon Signapore        | Singapore             | Workshop, Talk |
| Jun 1st - 5th   | Crusoe Corporate Training  | San Francisco, USA    | Workshop       |
| Jun 8th - 12th  | Genetec Corporate Training | Montreal, Canada      | Workshop       |
| Jun 14th - 19th | GopherCon EU               | Berlin, Germany       | Workshop, Talk |
| JULY            | Summer Vacation            | Huntsville, AL        | Rest           |
| Aug 3rd - 6th   | GopherCon USA              | Seattle, Washington   | Workshop, Talk |
| Aug 11th - 13th | GopherCon UK               | London, England       | Workshop, Talk |
| Sep 1st - 4th   | GopherCon LATAM            | Florianópolis, Brazil | Workshop, Talk |
| Oct 12th - 18th | GopherCon Africa           | Kenya, East Africa    | Workshop, Talk |
| Oct 29th - 4th  | GoLab (GopherCon Italy)    | Bologna, Italy        | Workshop, Talk |
