# Ultimate Local AI

### Description

Most teams reach for a Web API or stand up a Python service the moment they need a model. But self-hosted inference — running models on hardware you control — means no per-token costs, no data leaving your environment, no vendor lock-in, and the freedom to use the long tail of great open-source models that go well beyond the LLMs everyone is talking about. The problem is that doing this from Go has historically meant CGO, shelling out to Python, or making a network hop to something like Ollama. None of that feels like Go.

This is a lecture and hands-on full-day workshop where you'll go from zero to running open-source models directly inside your Go applications on your own local machine — no cloud APIs, no external servers, no data leaving your machine. Throughout the day, you will learn all the internals of the Kronk SDK which will teach you about model architectures, KV caching, batch processing, token/decoding, prompt caching, token sampling, and more.

With that solid foundation, you'll build real applications:

- Retrieval-Augmented Generation (RAG) pipeline that grounds model responses in your own documents using embeddings and vector search.

- Natural language to SQL system where the model generates database queries from plain English, with grammar constraints ensuring the output is always valid executable SQL.

By the end of the day, you won't just understand how AI model inference works — you'll have built applications that load models, cache intelligently, retrieve context, and generate code, all running locally on your own hardware.

### What a Student Is Expected to Learn

By the end of this workshop, you'll leave with working code, a deep understanding of model inference, and hands-on experience across the full stack: model configuration, performance tuning, intelligent caching, retrieval-augmented generation, and structured code generation.

### Hardware Requirements

Don't worry if you don't have the full hardware required for this.
The instructor will provide everything you need to follow along and be able to run the examples.

- Mac M1+ series with at least 16 GB RAM.
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
  - VRAM Calculations
  - Batch Engine Architecure
  - Caching System Semantics
  - Sampling parameters
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
  - Batch processing — concurrent requests with n_seq_max slots
  - Quick flash of observability: Prometheus metrics / Statsviz
- AI Agent Integration
  - Cline driving real coding work through KMS
  - MCP service with Brave Search — local model doing web searches
  - Mention compatibility: Claude Code, OpenWebUI, any OpenAI client
- RAG Application
  - Take Go Notebook and show how the model can use it to provide
    specific answers to questions.
- SQL Application
  - Create a relational database with data and using natural language
    query the database.
