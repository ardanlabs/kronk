Perform an incremental llama.cpp and/or yzma upstream review for Kronk.

Goal:
Find all meaningful upstream changes since the last successfully reviewed versions, determine their impact on Kronk, and identify required fixes, upgrade risks, performance opportunities, and features Kronk could adopt.

Only review llama.cpp versions that have a completed release in
https://github.com/hybridgroup/llama-cpp-builder/releases. Do not recommend or
advance the baseline to a newer upstream llama.cpp version that this project has
not built.

Review mode:

- Accept `llama.cpp`, `yzma`, or `both` as the requested scope. Default to `both` when no scope is supplied.
- For `llama.cpp`, review upstream llama.cpp and the yzma binding boundary it may affect.
- For `yzma`, review yzma itself and any llama.cpp revision or API assumptions it changes.
- For `both`, review both ranges and analyze their compatibility together.

Review baseline:

- Last reviewed upstream llama.cpp version: `b10760` (`0f3a71be15af836d277c9f918adfafb45732677e`)
- Last reviewed yzma main commit: `385dd04a1671023c38d1a768589c5f59f93386f5`
- Resolve and verify both baseline revisions before comparing changes.
- These values, rather than Kronk's currently pinned dependencies, are the starts of their respective review ranges.

Research range:

1. Read Kronk’s current llama.cpp pin and yzma version from all Go modules and generated dependency files.
2. For the requested scope, find the latest upstream llama.cpp release/commit and/or yzma main commit.
3. Compare each applicable last-reviewed revision above through the latest applicable upstream revision.
4. If llama.cpp build numbers, tags, yzma pseudo-versions, or commits do not map cleanly, verify exact SHAs before drawing conclusions.
5. If an applicable upstream has no new commits, report that clearly and leave that baseline unchanged.

Investigation:

- Use authoritative llama.cpp source, commit history, release notes, and documentation.
- Use authoritative yzma source, commit history, Go module metadata, tests, and documentation.
- Inspect both Kronk’s selected yzma revision and latest yzma main to understand which llama.cpp APIs they expose and support.
- Inspect Kronk’s code to identify the affected execution paths. Do not assume an upstream change affects Kronk merely because it exists.
- Separate native `pkg/llama` and `pkg/mtmd` changes from platform-specific packages and build-tagged code such as `pkg/llamawasm`.
- Pay particular attention to:
  - Breaking or deprecated C/C++ API changes
  - Model architecture and GGUF support
  - Tokenization and chat templates
  - Sampling and generation behavior
  - Context management, KV cache, prompt caching, and slot behavior
  - Continuous batching and parallel sequence processing
  - Speculative decoding and MTP
  - Multimodal and embedding/reranking support
  - Grammar, structured output, and tool calling
  - CPU, Metal, CUDA, Vulkan, ROCm, and other backend changes
  - Quantization formats and numerical correctness
  - Memory use, model loading, startup time, and inference performance
  - Thread safety, crashes, data races, and correctness fixes
  - Observability or metrics Kronk could expose
  - Build, packaging, and library artifact changes
  - Security-relevant fixes

Classify every relevant finding as one of:

- REQUIRED: Kronk or yzma must change before upgrading.
- RECOMMENDED: A correctness, compatibility, or maintainability improvement.
- OPPORTUNITY: A new feature or measurable optimization Kronk could adopt.
- AUTOMATIC: Kronk benefits through yzma/llama.cpp without code changes.
- NOT APPLICABLE: Potentially notable upstream work that does not affect Kronk, with a brief reason.

For each REQUIRED, RECOMMENDED, or OPPORTUNITY finding, provide:

- Upstream build/version and commit
- What changed
- Why it affects Kronk
- The exact Kronk and/or yzma files and symbols involved
- Whether the current yzma release exposes the required API
- The smallest reasonable Kronk change
- Risks and suggested verification
- Evidence links to upstream commits, source, issues, or documentation

Do not:

- Treat commit titles or release-note summaries as sufficient evidence.
- Call something a breaking change or root cause without inspecting the relevant code.
- Recommend features Kronk already implements.
- Mix speculative ideas with verified findings.
- Modify Kronk production code during this review.
- Update dependency pins or generated files during this review.
- Update either review baseline if that upstream's research is incomplete or its exact comparison range could not be established.

Deliverable:
Provide the following information with a plan of changes to implement. Use the following
list to help determine this plan.

1. Are there any ABI changes that break YZMA or anything that breaks Kronk.
2. Exact comparison ranges and review mode
3. Current and latest llama.cpp and yzma revisions
4. Required changes
5. Recommended changes
6. Opportunities
7. Automatic benefits
8. Not-applicable notable changes
9. Suggested upgrade order
10. Focused validation and benchmark plan
11. Evidence links
12. A prioritized action table containing impact, effort, risk, and owning Kronk area

Be concise but complete. Prioritize findings that materially affect correctness, compatibility, performance, or product capability.

At the end:

- Present the key conclusions to me.
- Do not implement recommendations unless I explicitly ask.
- If the review identifies any REQUIRED yzma change, draft a ready-to-submit yzma
  GitHub issue containing a title, problem statement, reproduction, expected
  behavior, suggested fix, compatibility notes, and evidence links, then copy the
  complete issue text to the clipboard.
- If and only if an upstream review completed successfully, update that upstream's baseline in this prompt to the exact latest revision reviewed. Do not advance the other baseline unless it was also reviewed successfully.
