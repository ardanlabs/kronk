# Ultimate AI

Go with your own intelligence

### Description

Running open-source models is about much more than loading weights and sending a prompt. Every model server must make the same kinds of decisions: which model and quantization fit the available hardware, how requests are admitted and batched, how prompts are rendered, how context is cached, how tokens are sampled, and how concurrent generations share compute without sharing state.

In this talk, I will review the kronk inference architecture and small working programs. From there we follow a generation request through the entire serving lifecycle: admission control, model-specific prompt rendering, incremental message-cache selection, scheduling, slot assignment, KV-cache restoration, prompt prefill, batched decoding, sampling, parsing, and streaming.

By the end of the talk, you will be able to reason at a high-level about how an open-source model server works, configure one intentionally, diagnose performance and capacity problems, and use Kronk to run models on hardware you control.
