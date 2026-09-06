Review the proposed llama.cpp and/or yzma upgrade for Kronk and produce a decision-ready report and implementation plan.

## Upgrade range

- Treat the `defaultVersion` change in `sdk/tools/libs/libs.go` as the proposed llama.cpp upgrade. Compare its previous committed value with the new value.
- If yzma was also changed, compare the previous and proposed versions in every Go module.
- Resolve build numbers, tags, pseudo-versions, and commits to exact upstream SHAs.
- Verify the proposed llama.cpp build has a completed release in `hybridgroup/llama-cpp-builder`. Do not recommend a build Kronk cannot download.

## Review

Use authoritative llama.cpp and yzma source, history, release notes, tests, and documentation. Inspect Kronk's actual usage before deciding that a change applies.

Determine:

1. Whether llama.cpp changed any ABI or C API used by yzma, including symbols, signatures, structs, enums, ownership, lifetimes, or behavioral contracts.
2. Whether the selected yzma version is compatible with the proposed llama.cpp build.
3. Whether upstream changes break or alter Kronk behavior, correctness, performance, model support, build, packaging, or supported backends.
4. Whether yzma must expose any new or changed llama.cpp APIs required by Kronk.
5. Whether Kronk or yzma should replace deprecated or superseded APIs with better current APIs.
6. Whether any other upstream change needs review or offers a meaningful benefit to Kronk.
7. Which version references must stay aligned, including root and example Go modules, generated dependency data, `sdk/tools/libs`, the README compatibility table and version text, and relevant manual documentation.

Pay particular attention to model loading and GGUF support, tokenization and chat templates, sampling, context and KV cache management, batching, speculative decoding/MTP, multimodal, embeddings/reranking, grammar and tool calling, CPU/GPU backends, quantization, memory and performance, thread safety, security, and library artifacts.

Do not infer impact from commit titles alone. Cite the source or diff that supports each material conclusion, and distinguish verified findings from unresolved risks.

## Report

Keep the report concise and include:

- Exact old and proposed llama.cpp and yzma revisions.
- A compatibility verdict: safe, blocked, or needs investigation.
- Required changes, separated by llama.cpp, yzma, Kronk, and documentation.
- Recommended API migrations or worthwhile opportunities.
- Notable upstream changes that do not apply to Kronk, only when useful to rule out a concern.
- A prioritized implementation order with affected files/symbols, rationale, risk, and focused validation needed after each step.
- Evidence links for every required change and compatibility conclusion.
- Open questions or items that could not be verified.

Do not modify code, dependencies, generated files, documentation, or this prompt, and do not run tests. Stop after the report and plan so I can choose what to implement and in what order.
