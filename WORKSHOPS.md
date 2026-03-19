# Kronk: Hardware accelerated local inference

### Description

Running AI models locally means no API costs, no data leaving your machine, and no vendor lock-in — but integrating local inference into Go applications has traditionally been painful. In this talk, Bill will introduce Kronk, a Go SDK that lets you embed local model inference directly into your applications with full GPU acceleration — no CGO required. Whether it's chat, vision, audio, embeddings, or tool calling, Kronk gives you the same power as a model server without needing one. To prove it, Bill built a Model Server entirely on top of the SDK, complete with caching, batch processing, and agent support. You'll see live demos from writing your first chat app to driving a coding agent with a local model.

### Talk Outline

- Why Local Inference? (Privacy, latency, cost, no vendor lock-in, offline)
  - What is Kronk? (Go SDK + optional Model Server)
  - Architecture: SDK-first design, non-CGO via yzma
  - Show the layered architecture diagram
- Hello World — Question example (simplest SDK usage)
  - Walk through the code, show it running
- Configuration Matters
  - GPU layer offloading (n_gpu_layers) — CPU vs GPU performance
  - KV cache quantization (cache_type_k/v) — VRAM savings
  - Context window and batch sizes (n_batch, n_ubatch) — tradeoffs
  - Sampling parameters: temperature, top_p, top_k, repetition penalty
  - Show side-by-side output differences
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

---

# Ultimate Private AI

### Type: Workshop

This is a hands-on, full-day workshop where you'll go from zero to running open-source models directly inside your Go applications — no cloud APIs, no external servers, no data leaving your machine.

You'll start by loading a model and running your first inference with the Kronk SDK. Then you'll learn how to configure models for your hardware — GPU layers, KV cache placement, batch sizes, and context windows — so you get the best performance out of whatever machine you're running on. With the model tuned, you'll take control of its output through sampling parameters: temperature, top-k, top-p, repetition penalties, and grammar constraints that guarantee structured JSON responses.

Next you'll see how Kronk's caching systems — System Prompt Cache (SPC) and Incremental Message Cache (IMC) — eliminate redundant computation and make multi-turn conversations fast. You'll watch a conversation go from full prefill on every request to only processing the newest message.

With the foundation solid, you'll build real applications: a Retrieval-Augmented Generation (RAG) pipeline that grounds model responses in your own documents using embeddings and vector search, and a natural-language-to-SQL system where the model generates database queries from plain English — with grammar constraints ensuring the output is always valid, executable SQL.

Each part builds on the last.

By the end of the day, you won't just understand how private AI works — you'll have built applications that load models, cache intelligently, retrieve context, and generate code, all running locally on your own hardware.

## What a Student Is Expected to Learn

By the end of this workshop, you'll leave with working code, a deep understanding of local model inference in Go, and hands-on experience across the full stack: model configuration, performance tuning, intelligent caching, retrieval-augmented generation, and structured code generation. 🚀

## Hardware Requirements

Don't worry if you don't have the full hardware required for this.
The instructor will provide everything you need to follow along and be able to run the examples.

- Mac M1 series with at least 16 GB RAM (pref 32GB+).
- Any Linux/Windows laptop with a dedicated GPU with at least 8GB VRAM (not system RAM) (pref 16GB).
- Access to a cloud-based instance with a dedicated GPU with at least 8GB VRAM (pref 16GB).

## Prerequisites

- It's expected that you will have been coding in Go for several months.
- A working Go environment running on the device you will be bringing to class.

## Recommended Preparation

- Please clone the main repo (https://github.com/ardanlabs/kronk) for the class.
- Please read the notes in the makefile for installing all the tooling and testing the code before class.
- Please email the instructor, Bill Kennedy, for assistance.

---

## Part 1: First Inference — Loading Models and Running Prompts in Go

- **Understanding the Kronk SDK** — Learn how Kronk wraps llama.cpp via Yzma's non-CGO FFI bindings to give you hardware-accelerated inference directly in Go — no server process, no HTTP overhead, no data leaving your machine.
- **Loading Your First Model** — Download a GGUF model from the catalog, load it into memory, and run your first chat completion entirely from Go code.
- **Understanding GGUF Quantization** — Learn what quantization levels (Q4_K_M, Q6_K, Q8_0, f16) mean in practice — the trade-offs between model quality, speed, and VRAM usage — so you can pick the right model for your hardware.
- **Streaming Responses** — Process tokens as they're generated using Kronk's streaming API, building responsive applications that don't block waiting for full completions.
- **Building a Simple Chat Loop** — Wire up a multi-turn conversation in Go, managing message history and context as the conversation grows.

---

## Part 2: Tune It — Model Configuration and GPU Optimization

- **GPU Layer Offloading** — Control how many model layers live on the GPU versus CPU. Learn to maximize GPU utilization when the full model doesn't fit in VRAM, and understand the performance cliff when layers spill to CPU.
- **KV Cache Placement** — Decide whether the model's short-term memory lives on GPU (fast) or CPU (saves VRAM). Understand when to move it off the GPU and what it costs.
- **Batch Size Tuning** — Configure `n_batch` and `n_ubatch` to control how the model chews through your prompts. Match batch sizes to your workload: small and fast for interactive chat, large and throughput-optimized for RAG pipelines.
- **Context Window Sizing** — Set the right context window for your use case and understand the VRAM cost. Learn when you need 8K tokens versus 32K, and how to use YaRN to extend context windows 2-4x beyond the model's training length.
- **KV Cache Quantization** — Reduce VRAM consumption by quantizing the KV cache from f16 to q8_0 or q4_0, with minimal impact on output quality. Free up memory for larger context windows or bigger models.
- **Flash Attention** — Enable flash attention for faster inference with lower memory usage. Understand when it helps and what models support it.

---

## Part 3: Control It — Sampling Parameters and Structured Output

- **Temperature and Creativity** — Understand what temperature actually does to the probability distribution. Learn when to crank it up for creative writing and when to drop it to near-zero for deterministic, factual output.
- **Top-K and Top-P Sampling** — Control the diversity of generated text by limiting the token pool. Learn how nucleus sampling (top-p) adapts to the model's confidence, and when to combine it with top-k for tighter control.
- **Repetition Penalties** — Stop models from getting stuck in loops. Configure repeat penalties, DRY (Don't Repeat Yourself) n-gram detection, and penalty windows to keep output fresh without killing coherent structure.
- **Grammar Constraints** — Force the model to produce valid JSON, booleans, integers, or any custom format using GBNF grammars. Guarantee that every response is machine-parseable — no regex, no retries, no prayer.
- **JSON Schema Constraints** — Define a JSON schema and let Kronk auto-convert it to a grammar. Get typed, validated output that maps directly to your Go structs.
- **Thinking and Reasoning Modes** — Enable model reasoning for complex problems, or disable it for fast direct responses. Understand how `enable_thinking` and `reasoning_effort` change model behavior.

---

## Part 4: Cache It — System Prompt Cache and Incremental Message Cache

- **Why Caching Matters** — See the real cost of prefill: every request without caching reprocesses the entire conversation from scratch. Measure the latency difference between cached and uncached requests.
- **System Prompt Cache (SPC)** — Decode the system prompt once, store the KV state in RAM, and restore it into every request. Eliminate the most common source of redundant computation in multi-user and chat interface scenarios.
- **Incremental Message Cache (IMC)** — Dedicate KV cache slots to conversations and extend the cache incrementally on each turn. After the first request, only the newest message gets prefilled — everything else is cached.
- **Multi-Slot IMC for Agents** — Configure multiple cache slots for sub-agent architectures. Give each agent its own cached conversation branch so concurrent agents don't thrash each other's caches.
- **Cache Invalidation and Debugging** — Understand when and why caches invalidate. Use Kronk's logging to watch hash matching, token prefix fallback, and slot selection in real time.
- **Choosing the Right Strategy** — SPC for stateless multi-user APIs. IMC for agentic workflows and long-running conversations. Learn the decision framework and see both in action.

---

## Part 5: Ground It — Retrieval-Augmented Generation (RAG) in Go

- **Understanding RAG** — Models don't know your data. Learn how to dynamically inject relevant context into prompts so the model generates accurate, grounded responses instead of hallucinating.
- **Generating Embeddings** — Use Kronk's embedding models to convert documents and queries into vector representations — all locally, no API calls, no data leaving your network.
- **Building a Document Pipeline** — Chunk documents, generate embeddings, and store them for retrieval. Learn chunking strategies that preserve meaning and maximize retrieval quality.
- **Vector Search and Retrieval** — Search your embedded documents by semantic similarity. Find the most relevant context for a user's query and inject it into the prompt.
- **End-to-End RAG Application** — Build a complete RAG pipeline in Go: ingest documents, embed them, retrieve context, and generate grounded responses — all running on your local hardware with the Kronk SDK.

---

## Part 6: Generate It — Natural Language to SQL with Grammar Constraints

- **The Problem** — Users want to ask questions in plain English. Databases speak SQL. Teach a local model to bridge that gap — privately, with no data sent to the cloud.
- **Schema-Aware Prompting** — Inject your database schema into the system prompt so the model understands your tables, columns, types, and relationships. Learn prompt engineering techniques that produce correct SQL.
- **Grammar-Constrained SQL Generation** — Use GBNF grammars to guarantee the model's output is syntactically valid SQL. No post-processing, no regex cleanup — every response is executable.
- **Executing Generated Queries** — Take the model's SQL output and run it against a real database. Handle results, format responses, and close the loop from natural language question to data answer.
- **Safety and Validation** — Restrict the model to SELECT queries, validate table and column names against your schema, and implement guardrails that prevent destructive operations — because the model generates the SQL, but your code decides what runs.

---

Classroom WiFi router choice

**Plug the GL-AXT1800 into the venue's ethernet** (most training venues have a wired drop or you can ask for one). The router creates your private WiFi network. Everyone — you and the 30 students — connects to it.

The setup gives you both things:

1. **Internet access** through the wired uplink — students can pull the catalog, libs, and anything else they need from the internet
2. **Fast local network** — model downloads from your machine stay on the private WiFi (LAN traffic never touches the internet uplink), so all 30 students can pull multi-GB models at full WiFi 6 speed

The key insight is that LAN-to-LAN traffic on the router never hits the WAN uplink. When a student curls `http://<your-ip>:11435/download/...`, that traffic stays entirely on the local network.

**Steps:**

1. Plug the router's WAN port into the venue's ethernet
2. Connect everyone to the router's WiFi
3. Run Kronk with `KRONK_DOWNLOAD_ENABLED=true` on your machine
4. Give students your LAN IP (e.g., `192.168.8.100`) — check it with `ifconfig en0`
5. Students use `http://192.168.8.100:11435/download/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf` in their BUI download screen

**No venue ethernet?** You can also set the router in repeater mode — it connects to the public WiFi as its uplink and creates your private network on top. Same result, just the internet path is slightly slower (WiFi-to-WiFi), but the local model downloads are still LAN-speed.
