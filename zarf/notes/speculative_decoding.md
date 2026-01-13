## Speculative Decoding

https://github.com/ggml-org/llama.cpp/tree/537d4240d4f4dbd7f2eac1e3bf0452194fbb8e39/examples/speculative

Speculative Decoding is an inference optimization that uses a small, fast "draft" model to predict multiple tokens ahead, then has the main target model verify them in parallel in a single forward pass.

Why it helps:

LLM inference is memory-bound, not compute-bound—GPUs have spare cycles while waiting on memory
Standard autoregressive decoding generates one token per forward pass, underutilizing hardware
Speculative decoding can achieve 2-3× speedups by accepting multiple tokens per pass when the draft aligns with the target
How it works:

Draft model proposes K tokens quickly
Target model verifies all K tokens in one parallel forward pass
Accept the longest matching prefix; target generates the next token itself
Repeat
The llama.cpp implementation you linked uses this pattern with a smaller draft model (like a 0.5B-1B variant of the main model) to accelerate a larger target model. For Kronk, this would mean loading two GGUF models—one large target and one small draft—and coordinating their generation loops.
