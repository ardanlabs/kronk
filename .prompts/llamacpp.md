Perform an incremental llama.cpp upstream review for Kronk.

Goal:
Find all meaningful llama.cpp changes since the last successfully reviewed upstream version, determine their impact on Kronk and yzma, and identify required fixes, upgrade risks, performance opportunities, and features Kronk could adopt.

Review baseline:

- Last reviewed upstream llama.cpp version: `b10625`
- Initial baseline reason: this was the llama.cpp version locked when Kronk upgraded to yzma `v1.25.0`.
- Resolve and verify the exact upstream commit SHA for this version before comparing changes.
- This value, rather than Kronk's currently pinned llama.cpp version, is the start of the review range.

Research range:

1. Read Kronk’s current llama.cpp pin and yzma version.
2. Find the latest available upstream llama.cpp build and commit.
3. Compare the last-reviewed upstream version above through the latest upstream commit.
4. If upstream build numbers or tags do not map cleanly to commits, verify the exact SHAs before drawing conclusions.
5. If there are no new upstream commits, report that clearly and stop without changing the review baseline.

Investigation:

- Use authoritative llama.cpp source, commit history, release notes, and documentation.
- Inspect the yzma source for Kronk’s currently selected version to understand which llama.cpp APIs it exposes and which upstream version it supports.
- Inspect Kronk’s code to identify the affected execution paths. Do not assume an upstream change affects Kronk merely because it exists.
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
- Update the review baseline if the research is incomplete or an exact comparison range could not be established.

Deliverable:
Write or update `.agents/llama-cpp-upstream-review.md` with:

1. Executive summary
2. Exact comparison range
3. Current Kronk llama.cpp and yzma versions
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
- If and only if the review completed successfully, update the "Last reviewed upstream llama.cpp version" value in this prompt to the latest upstream version reviewed.
