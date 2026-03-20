# Ultimate AI Internals

### Description

Running AI models locally means no API costs, no data leaving your machine, and no vendor lock-in — but integrating local inference into Go applications has traditionally been painful. In this talk, Bill will introduce Kronk, a Go SDK that lets you embed local model inference directly into your applications with full GPU acceleration — no CGO required. Whether it's chat, vision, audio, embeddings, or tool calling, Kronk gives you the same power as a model server without needing one. To prove it, Bill built a Model Server entirely on top of the SDK, complete with caching, batch processing, and agent support. You'll see live demos from writing your first chat app to driving a coding agent with a local model.

This is a lecture and hands-on full-day workshop where you'll go from zero to running open-source models directly inside your Go applications — no cloud APIs, no external servers, no data leaving your machine.

You'll start by loading a model and running your first inference with the Kronk SDK. Then you'll learn all the internals of the Kronk SDK which will teach you how model servers work. From this knowledge you will learn how to configure models for your hardware — GPU layers, KV cache placement, batch sizes, and context windows — so you get the best performance out of whatever machine you're running on. With the model tuned, you'll take control of its output through sampling parameters: temperature, top-k, top-p, repetition penalties, and grammar constraints that guarantee structured JSON responses.

Next you'll see how Kronk's caching systems — System Prompt Cache (SPC) and Incremental Message Cache (IMC) — eliminate redundant computation and make multi-turn conversations fast. You'll watch a conversation go from full prefill on every request to only processing the newest message.

With the foundation solid, you'll build real applications: a Retrieval-Augmented Generation (RAG) pipeline that grounds model responses in your own documents using embeddings and vector search, and a natural-language-to-SQL system where the model generates database queries from plain English — with grammar constraints ensuring the output is always valid, executable SQL.

Each part builds on the last.

By the end of the day, you won't just understand how AI models works — you'll have built applications that load models, cache intelligently, retrieve context, and generate code, all running locally on your own hardware.

### What a Student Is Expected to Learn

By the end of this workshop, you'll leave with working code, a deep understanding of local model inference in Go, and hands-on experience across the full stack: model configuration, performance tuning, intelligent caching, retrieval-augmented generation, and structured code generation. 🚀

### Hardware Requirements

Don't worry if you don't have the full hardware required for this.
The instructor will provide everything you need to follow along and be able to run the examples.

- Mac M1 series with at least 16 GB RAM (pref 32GB+).
- Any Linux/Windows laptop with a dedicated GPU with at least 8GB VRAM (not system RAM) (pref 16GB).
- Access to a cloud-based instance with a dedicated GPU with at least 8GB VRAM (pref 16GB).

### Prerequisites

- It's expected that you will have been coding in Go for several months.
- A working Go environment running on the device you will be bringing to class.

### Recommended Preparation

- Please clone the main repo (https://github.com/ardanlabs/kronk) for the class.
- Please read the notes in the makefile for installing all the tooling and testing the code before class.
- Please email the instructor, Bill Kennedy, for assistance.

### Outline

- Why Local Inference? (Privacy, latency, cost, no vendor lock-in, offline)
  - What is Kronk? (Go SDK + optional Model Server)
  - Architecture: SDK-first design, non-CGO via yzma
  - Show the layered architecture diagram
- Hello World — Question example (simplest SDK usage)
  - Walk through the code, show it running
- Architecture and Configuration
  - Navigation Hugging Face and Model Types
    - Model Types
    - Quantization Formats
  - VRAM Calculations
    - Understandig the formula
    - Model Weights, Layer Offering
    - KV cache
    - Context window and Batch Sizes
  - Batch Engine Architecure
    - Work Tray and GPU
    - Slots and Sequences
  - Caching System Semantics
    - System Prompt Caching
    - Incremental Message Caching
  - Sampling parameters
    - Temperature, top_p, top_k, repetition penalty
- Tool Calling with a Local Model
  - Use the chat example's get_weather function
  - Show a local model deciding to call tools
- Vision App
  - What projectors are and why vision models need them
  - Memory overhead: model + projector + KV cache
- Kronk Model Server (KMS)
  - Catalog system — kronk catalog pull, verified models
  - Show the BUI and all the tools/apps
  - Chat App with a coding model
    - SPC (System Prompt Caching) — show performance improvement
    - IMC (Incremental Message Caching) — show performance improvement
  - Batch processing — concurrent requests with n_seq_max slots
  - Quick flash of observability: Prometheus metrics / Statsviz
- AI Agent Integration
  - Cline driving real coding work through KMS
  - MCP service with Brave Search — local model doing web searches
  - Mention compatibility: Claude Code, OpenWebUI, any OpenAI client
- Production Concerns (brief)
  - JWT authentication
  - Embeddings + Reranking for the full RAG pipeline
- Conclusion
  - Limitations
  - Future work
  - How to get involved
