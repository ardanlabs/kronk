# Ultimate AI

Go with your own intelligence

### Description

Running open-source models is about much more than loading weights and sending a prompt. Every model server must make the same kinds of decisions: which model and quantization fit the available hardware, how requests are admitted and batched, how prompts are rendered, how context is cached, how tokens are sampled, and how concurrent generations share compute without sharing state.

This full-day lecture and hands-on workshop teaches the foundations of open-source model serving through Kronk, a model SDK and server written for Go. Kronk gives us a concrete system to examine, but the concepts apply broadly to model servers built on inference engines such as llama.cpp.

We begin with the architecture of an inference stack and small working programs. From there, we learn how to navigate Hugging Face, compare dense, mixture-of-experts, and hybrid models, choose a GGUF quantization, and estimate the memory needed for model weights, context, cache, and runtime buffers.

Then we follow a generation request through the entire serving lifecycle: admission control, model-specific prompt rendering, incremental message-cache selection, scheduling, slot assignment, KV-cache restoration, prompt prefill, batched decoding, sampling, parsing, and streaming. Along the way, we connect the settings exposed by a model server to the work they control and the trade-offs they create in memory use, throughput, latency, and output quality.

By the end of the day, you will be able to reason about how an open-source model server works, configure one intentionally, diagnose performance and capacity problems, and use Kronk to run models on hardware you control.

### What a Student Is Expected to Learn

By the end of this workshop, you will be able to:

- Explain the layers between an application, a model server or SDK, a native inference engine, model artifacts, and compute hardware.
- Find and evaluate open-source models on Hugging Face.
- Compare dense, mixture-of-experts, and hybrid model architectures.
- Choose a GGUF quantization and account for the memory used by weights, context, KV cache, and runtime buffers.
- Explain how context size, batch size, sequence count, queue depth, and timeouts affect capacity, latency, and throughput.
- Trace a request from admission and prompt rendering through prefill, decoding, sampling, parsing, and streaming.
- Explain how incremental message caching and KV-cache restoration avoid repeated computation.
- Configure sampling and structured-output controls with an understanding of their effect on generation.
- Apply these concepts when operating Kronk or evaluating another open-source model server.

### Hardware Requirements

Don't worry if you don't have all the hardware listed below. The instructor will provide what you need to follow along and run the examples.

- Mac M1+ series with at least 16 GB RAM.
- Any Linux or Windows laptop with a dedicated GPU and at least 8 GB of VRAM (16 GB preferred).
- Access to a cloud instance with a dedicated GPU and at least 8 GB of VRAM (16 GB preferred).

### Prerequisites

- Several months of experience writing Go.
- A working Go development environment on the device you will bring to class.

### Recommended Preparation

- Clone the [Kronk repository](https://github.com/ardanlabs/kronk).
- Read the notes in the makefile and install the required tooling before class.
- Contact the instructor, Bill Kennedy, if you need assistance preparing your environment.

### Outline

- The Open-Source Inference Stack
  - Why run models on hardware you control?
  - Kronk's embedded SDK and network model-server paths
  - Go APIs, native inference engines, model artifacts, and compute backends
- Hands-On Example Programs
  - Load and run a model from Go
  - Connect SDK settings to observable model behavior
- Hugging Face and Model Selection
  - Repositories, model families, capabilities, and GGUF files
  - Dense, mixture-of-experts, and hybrid architectures
  - Quantization trade-offs: model quality, file size, RAM, and VRAM
  - Memory planning for weights, context, KV cache, and runtime buffers
- The Generation Inference Lifecycle
  - Receive a request and acquire admission
  - Render a model-specific prompt and tool definitions
  - Select and reserve an incremental message-cache session
  - Submit work, wait for an execution slot, and preserve cancellation
  - Restore reusable KV state and prefill uncached prompt tokens
  - Batch concurrent sequences while keeping request state isolated
  - Decode, constrain, sample, parse, and stream generated tokens
  - Finish the response and release slots, cache reservations, and admission permits
- Configuration and Performance Trade-offs
  - Context size, batch size, sequence count, queue depth, and timeouts
  - Prompt caching, throughput, latency, and memory pressure
  - Sampling parameters, grammars, structured output, and output quality
  - Using metrics and request behavior to diagnose a model server
