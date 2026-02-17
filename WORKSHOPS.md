# Kronk: Hardware accelerated local inference

### Type: Talk

In this talk Bill will introduce Kronk, a new SDK that allows you to write AI based apps without the need of a model server. If you have Apple Metal (Mac), CUDA (NVIDIA), or Vulkan, Kronk can tap into that GPU power instead of grinding through the work on the CPU alone.

To dog food the SDK, Bill wrote a Model Server that is optimized to run your local AI workloads with performance in mind. During the talk, Bill will show how you can use Agents like Cline and Kilo Code to run local agentic workloads to perform basic work.

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
