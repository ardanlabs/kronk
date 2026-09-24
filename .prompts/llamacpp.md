Review the proposed llama.cpp and/or yzma upgrade for Kronk and produce a decision-ready report and implementation plan.

This prompt has two outcomes:

1. **Review gate:** If the upgrade changes an ABI or C API used by yzma or Kronk, or adds an API that Kronk might reasonably adopt, stop after the report and implementation plan. Do not modify files or run tests. The change requires review before implementation.
2. **Routine upgrade:** If the proposed versions are compatible and require only coordinated llama.cpp and yzma version updates, implement the upgrade without waiting for approval. Keep all version references aligned, update generated dependency data and documentation derived from changed sources, and run the focused verification required by the repository instructions. Report the changes and verification results.

## Upgrade range

- Upgrade llama.cpp to the latest completed release published at `https://github.com/hybridgroup/llama-cpp-builder/releases` at the time this prompt is run. Compare the currently committed `defaultVersion` in `sdk/tools/libs/libs.go` with that release and update its authenticated manifest digest only if the routine-upgrade path applies.
- Upgrade yzma to the latest commit on the upstream `hybridgroup/yzma` `main` branch at the time this prompt is run. Always use the head of `main`, not the latest tag or the version currently selected by a Go module. Compare every Go module's currently committed yzma version with that commit and keep all modules aligned if the routine-upgrade path applies.
- Resolve build numbers, tags, pseudo-versions, and commits to exact upstream SHAs.
- Verify the proposed llama.cpp build has a completed release in `hybridgroup/llama-cpp-builder`. Do not recommend a build Kronk cannot download.

## Review

Use authoritative llama.cpp and yzma source, history, release notes, tests, and documentation. Inspect Kronk's actual usage before deciding that a change applies.

Audit the complete upstream commit range, commit by commit; do not sample commits or rely only on release summaries. Review every llama.cpp commit between the exact old and proposed revisions against Kronk's actual llama, MTMD, speculative decoding/MTP, batching, cache, sampler, tokenizer, grammar and tool-calling, model-loading, multimodal, and backend usage. If yzma changes, perform the same commit-by-commit audit for its exact old-to-proposed range. For each commit, inspect the source diff and classify it as:

- an internal fix or optimization Kronk inherits automatically;
- an API, ABI, behavioral-contract, model-support, or packaging change that requires Kronk or yzma work;
- a new capability or better API that Kronk could reasonably adopt; or
- unrelated to Kronk.

Account for every commit in the report. Commits with no Kronk impact may be grouped, but material changes and opportunities must be discussed individually with supporting evidence. Complete this audit before changing any files. The review itself is read-only; only proceed to implementation afterward when the routine-upgrade path applies.

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

When the review gate applies, do not modify code, dependencies, generated files, documentation, or this prompt, and do not run tests. Stop after the report and plan so I can choose what to implement and in what order.

When the routine-upgrade path applies, make the smallest complete version-alignment change after the review. Do not introduce API migrations, behavior changes, refactors, or unrelated cleanup under that path. Follow repository guidance for formatting, generated files, dependency updates, and focused checks; never run a prohibited full-repository test suite.
