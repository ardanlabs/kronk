![Kronk logo](./images/project/kronk_banner.jpg?v5)

# Kronk

[![Go Reference](https://pkg.go.dev/badge/github.com/ardanlabs/kronk.svg)](https://pkg.go.dev/github.com/ardanlabs/kronk)
[![Go version](https://img.shields.io/github/go-mod/go-version/ardanlabs/kronk)](https://github.com/ardanlabs/kronk)
[![Linux](https://github.com/ardanlabs/kronk/actions/workflows/linux.yml/badge.svg)](https://github.com/ardanlabs/kronk/actions/workflows/linux.yml)
[![llama.cpp release](https://img.shields.io/github/v/release/ggml-org/llama.cpp?label=llama.cpp)](https://github.com/ggml-org/llama.cpp/releases)

Kronk is a Go SDK and model server for hardware-accelerated local inference. It
provides high-level Go APIs over native inference engines without requiring Python or
a separate model-serving stack:

- **Kronk** runs text, vision, embedding, and reranking models through
  [llama.cpp](https://github.com/ggml-org/llama.cpp) and
  [yzma](https://github.com/hybridgroup/yzma).
- **Bucky** provides speech-to-text through
  [whisper.cpp](https://github.com/ggml-org/whisper.cpp) and
  [bucky](https://github.com/ardanlabs/bucky).
- **Malina** is an experimental image-generation SDK built on
  [stable-diffusion.cpp](https://github.com/leejet/stable-diffusion.cpp) and
  [malina](https://github.com/ardanlabs/malina).

The Kronk model server exposes OpenAI-compatible APIs for Chat Completions,
Responses, embeddings, reranking, and audio transcription, plus an
Anthropic-compatible Messages API. It also includes a browser interface, model
management, security, observability, and integrations with OpenWebUI, OpenCode, and
Claude Code. Malina is currently SDK-only and is not integrated into the model server.

Visit [kronkai.com](https://kronkai.com) or read the
[manual](https://www.kronkai.com/manual) for complete documentation.

## Quick Start

The recommended installation method on macOS and Linux is Homebrew:

```shell
brew tap ardanlabs/kronk
brew trust ardanlabs/kronk
brew install kronk

kronk server start
```

You can also install the CLI with Go on a supported platform:

```shell
go install github.com/ardanlabs/kronk/cmd/kronk@latest

kronk server start
```

Open [http://localhost:11435](http://localhost:11435) to manage models and use the
Browser UI. The first model or SDK example you run can download the compatible native
libraries and model files automatically.

For container deployment, persistent storage, and production setup, see
[Container Quick Start](.manual/chapter-02-installation.md#23-container-quick-start).

## SDK or Model Server

The model server is built on the same public SDKs available to Go applications.

| Use the SDK when you need                      | Use the model server when you need                  |
| ---------------------------------------------- | --------------------------------------------------- |
| Inference inside a Go process                  | HTTP APIs for one or more clients                   |
| Direct control over model loading and lifetime | OpenAI- and Anthropic-compatible APIs               |
| No separate server process                     | Browser-based model management and testing          |
| Application-specific caching and concurrency   | Authentication, rate limiting, metrics, and tracing |

The Kronk SDK supports text generation, streaming, reasoning, tool calls, vision,
embeddings, reranking, concurrent processing, and incremental message caching. Bucky
supports file transcription, translation, channel-separated diarization, and live
streaming transcription. Experimental Malina supports text-to-image, image-to-image,
single-checkpoint and multi-file diffusion pipelines, and Motion-JPEG encoding.

## Platform Support

Hardware acceleration depends on the operating system, architecture, inference
engine, and library bundle. Kronk downloads native libraries that are compatible with
the installed release.

| OS      | CPU architectures | Available GPU backends          |
| ------- | ----------------- | ------------------------------- |
| Linux   | amd64, arm64      | CUDA, Vulkan, HIP, ROCm, SYCL   |
| macOS   | arm64             | Metal                           |
| Windows | amd64             | CUDA, Vulkan, HIP, SYCL, OpenCL |

Not every backend is available for every SDK or architecture. Use the CLI or SDK
library manager as the source of truth for combinations supported by your installed
version. See [Installation and Quick Start](.manual/chapter-02-installation.md) for
current requirements.

## Project Status

Kronk follows the native engines it integrates, and upstream changes can require
coordinated releases of Yzma, Bucky, Malina, and Kronk. Use each subsystem's downloader
instead of mixing native libraries from unrelated releases; every Kronk release is
bound to known-compatible library versions.

> [!WARNING]
> Malina is experimental. Its public API is subject to change, and it is not yet a
> Kronk model-server backend.

See [Breaking Changes](BREAKING_CHANGES.md), the
[release history](https://github.com/ardanlabs/kronk/releases), and the
[open issues](https://github.com/ardanlabs/kronk/issues) for current status.

## Documentation and Examples

- [Manual](https://www.kronkai.com/manual)
- [Go API reference](https://pkg.go.dev/github.com/ardanlabs/kronk)
- [Examples directory](examples/)
- [Model catalog and management](.manual/chapter-08-model-server.md)
- [Bucky: Audio Transcription](.manual/chapter-18-bucky.md)
- [Malina: Image Generation](.manual/chapter-19-malina.md)

Representative examples:

```shell
make example-question  # Ask a local language model a question.
make example-agent     # Run a small coding agent.
make example-vision    # Prompt a vision model with an image.
make example-bucky     # Transcribe an audio file with Bucky.
make example-malina    # Generate an image with experimental Malina.
```

Examples download compatible libraries and models on their first run. Browse the
[complete examples module](examples/) for chat, Responses, embeddings, reranking,
RAG, streaming transcription, image-to-image generation, model pools, session stores,
and lower-level yzma usage.

## Community and Support

Use [GitHub Issues](https://github.com/ardanlabs/kronk/issues) for bugs, feature
requests, and planned work. If you are interested in contributing or need help, email
[Bill Kennedy](mailto:bill@ardanlabs.com) or [Ardan Labs](mailto:hello@ardanlabs.com).

## Owner Information

```text
Name:     Bill Kennedy
Company:  Ardan Labs
Title:    Managing Partner
Email:    bill@ardanlabs.com
BlueSky:  https://bsky.app/profile/goinggo.net
LinkedIn: www.linkedin.com/in/william-kennedy-5b318778/
Twitter:  https://x.com/goinggodotnet
```

## Travel Schedule

Come find me at any of these cities or events this year. I will be giving workshops
and talks about Kronk.

| Dates           | Event                      | Location              | Comments       |
| --------------- | -------------------------- | --------------------- | -------------- |
| Jan 29th - 2nd  | AI Plumbers Fringe, FOSDEM | Brussels, Belgium     | Talk           |
| Mar 4th - 5th   | Ardan Connect              | São Paulo, Brazil     | Workshop       |
| Apr 20th - 25th | Gophercamp 2026            | Brno, Czech Republic  | Workshop, Talk |
| Apr 27th - 29th | AI Dev 26                  | San Francisco, USA    | Attendee       |
| May 17th - 23rd | GopherCon Singapore        | Singapore             | Workshop, Talk |
| Jun 8th - 12th  | Genetec Corporate Training | Montreal, Canada      | Workshop       |
| Jun 14th - 19th | GopherCon EU               | Berlin, Germany       | Workshop, Talk |
| JULY            | Summer Vacation            | Huntsville, AL        | Rest           |
| Aug 3rd - 6th   | GopherCon USA              | Seattle, Washington   | Workshop, Talk |
| Aug 11th - 13th | GopherCon UK               | London, England       | Workshop, Talk |
| Sep 1st - 4th   | GopherCon LATAM            | Florianópolis, Brazil | Workshop, Talk |
| Sep 23rd        | Meetup NYC                 | NYC, NY               | Talk           |
| Oct 6th - 9th   | Crusoe Corporate Training  | San Francisco, USA    | Workshop       |
| Oct 12th - 18th | GopherCon Africa           | Kenya, East Africa    | Workshop, Talk |
| Oct 29th - 4th  | GoLab (GopherCon Italy)    | Bologna, Italy        | Workshop, Talk |

Copyright 2025-2026 Ardan Labs
