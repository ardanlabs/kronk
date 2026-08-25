# Model usage inventory

This inventory records why Kronk keeps each model used by model-backed tests,
benchmarks, and examples. Update it whenever one of those references changes.
The project catalog remains the source of download metadata; this file is the
source of intent.

## Selection policy

- Use the smallest model and quantization that reliably exercises the required
  runtime path.
- Reuse one model across suites when it preserves architecture and capability
  coverage.
- Keep a larger model only when a smaller model cannot satisfy the assertions
  or does not expose the same parser, modality, or speculation implementation.
- Do not weaken correctness assertions merely to accommodate a smaller model.
- Prefer canonical catalog IDs in code and documentation. Repository/tag
  selectors are appropriate only for pull commands whose files have ambiguous
  names across Hugging Face repositories.

## Test models

`make install-test-models` installs the full local set. The approximate sizes
below include cataloged model, projection, and MTP companion files, but exclude
runtime KV cache. The Kronk models total about 52.0 GiB, down from about 98.0
GiB for the previous set. Whisper and Stable Diffusion are additional.

| Model | Approx. size | Required coverage |
| --- | ---: | --- |
| `unsloth/Qwen3-0.6B-Q8_0` | 0.60 GiB | Small Qwen behavior, truncated tool-call parsing, classic speculative-draft model |
| `unsloth/Qwen3.5-0.8B-Q8_0` | 0.95 GiB | Vision, vision IMC, and multimodal API coverage |
| `mradermacher/Qwopus3.5-4B-Coder.Q4_K_M` | 3.22 GiB | Hybrid chat/Responses, tools, grammar, batching, and Hybrid vision IMC |
| `unsloth/gemma-4-26B-A4B-it-UD-Q4_K_M` | 17.32 GiB | MoE chat/vision/IMC and Gemma shared-KV companion-file MTP |
| `unsloth/mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL` | 12.55 GiB | Embedded hybrid MTP, including multi-slot verification |
| `ggml-org/Qwen2.5-Omni-3B-Q4_K_M` | 4.40 GiB | Audio input through the llama multimodal path |
| `unsloth/gpt-oss-20b-Q8_0` | 11.28 GiB | GPT-OSS parser, reasoning, tools, streaming, and cache behavior |
| `unsloth/Qwen3-1.7B-Q4_K_M` | 1.03 GiB | General text APIs, tools, grammar, IMC, concurrency, and classic-draft target |
| `nomic-ai/nomic-embed-text-v1.5.Q8_0` | 0.14 GiB | Embedding sequence batching and embeddings API |
| `gpustack/bge-reranker-v2-m3-Q8_0` | 0.59 GiB | Cross-encoder reranking and rerank sequence batching |
| `ggml-tiny.bin` | 0.07 GiB | Whisper transcription and streaming |
| `sd-1.5` | bundle | Stable Diffusion image generation and Malina concurrency |

The 0.8B Qwen3.5 model owns ordinary vision coverage but exhausts its output
budget in reasoning during the generic Hybrid text suite. Qwopus remains the
smallest cataloged Hybrid model verified across the Hybrid text and vision IMC
suites.

Gemma 4 and GPT-OSS are both MoE models but are not duplicates. Gemma owns the
shared-KV MTP and MoE multimodal paths; GPT-OSS owns a distinct response parser.
The smaller Nomic model replaces Qwen3-Embedding for tests. BGE remains because
the smaller embedding models in the catalog are not equivalent cross-encoder
rerankers.

### Test ownership

- SDK model resolution and configs: `sdk/kronk/tests/testlib/testlib.go`
- Full local downloads: `.make/install.mk` (`install-test-models`)
- CI downloads: `.github/test-models.txt` and `.make/install.mk`
  (`install-test-gh-models`)
- Server API model IDs: `cmd/server/api/services/kronk/tests`
- Catalog metadata and capabilities: `sdk/tools/defaults/yaml/catalog.yaml`

## Benchmark models

Benchmarks are selected for representative coding behavior rather than minimum
download size. They are independent of the test-model reduction above.

The OpenCode coding benchmark discovers its models from
`zarf/kms/model_config.yaml`. Every eligible model ID ends in `/AGENT`; there is
no separate hard-coded benchmark model list. Benchmark ownership and operating
instructions are in `.tools/codegen-benchmark`.

## Example models

Examples favor small, approachable defaults. Low-level Yzma examples may list
additional direct Hugging Face URLs to demonstrate model loading.

| Model or bundle | Examples |
| --- | --- |
| `unsloth/Qwen3-0.6B-Q8_0` | chat, question, response, grammar, session-store, pool, Yzma text steps |
| `unsloth/Qwen3.5-0.8B-Q8_0` | vision, concurrency, pool, Yzma multimodal steps |
| `Qwen/Qwen3-Embedding-0.6B-Q8_0.gguf` | embedding, RAG, Yzma embedding step |
| `gpustack/bge-reranker-v2-m3-Q8_0` | rerank and Yzma rerank step |
| `ggml-org/Qwen2.5-Omni-3B-Q8_0` | audio |
| `ornith-ai/Ornith-1.5-9B-Q4_K_M` | agent |
| `ggml-tiny.bin` / `tiny` | Bucky transcription, streaming, and diarization |
| `sd-1.5` | Malina text-to-image and image-to-image |
| `flux2-klein-9b` | Malina FLUX.2 |

## Update checklist

1. Confirm the replacement exposes the same architecture, modality, parser, or
   speculation path; model type alone is not sufficient.
2. Run the affected model-backed suite before removing the old model.
3. Update every owner listed above and this inventory in the same change.
4. Run `TestEmbeddedCatalogCapabilities` when catalog capability metadata
   changes.
5. Search for stale references:

   ```shell
   rg -n '<old-model-id>' . --hidden --glob '!session-*.md' --glob '!**/runs/**'
   ```

6. Recalculate cataloged download size when changing the full test set. Include
   `file_sizes`, `mmproj_size`, and `mtp_size`.
