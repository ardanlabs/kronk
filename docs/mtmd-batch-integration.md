# MTMD Batch Engine Integration

This document covers the design and implementation of parallel processing for multi-modal (mtmd) requests in the batch engine.

## Status

**IMPLEMENTED** - mtmd requests now use the batch engine for parallel processing.

## Background

All mtmd requests now use the batch engine for parallel inference via `submitToBatchEngine`. There is no longer a sequential processing path.

---

## FAQ

### Does embedding decode block all other processing?

**Question:** The embedding decode is blocking — does this mean all other processing has to pause while we perform the embedding part of mtmd processing?

**Answer:** Not exactly.

**What CAN batch together:**
- Text tokens from multiple slots → single `llama.Decode` call
- Text tokens from slot A + text tokens from slot B in same batch ✓

**What CANNOT batch together:**
- Image embeddings use `batch.Embd` (float32 vectors)
- Text tokens use `batch.Token` (token IDs)
- These are different data types → can't mix in same decode call

**The flow would be:**

```
Iteration 1:
  - Slot A: text chunk → add to batch
  - Slot B: text chunk → add to batch  
  - Decode (batched text for A+B)

Iteration 2:
  - Slot A: image chunk → EncodeChunk, then decode embeddings (separate call)
  - Slot B: continues text → add to batch, decode

Iteration 3:
  - Slot A: text chunk (after image) → add to batch
  - Slot B: generating → add to batch
  - Decode (batched)
```

So other slots **don't pause** — they continue in parallel. The embedding decode for one slot just uses its own decode call rather than batching with text tokens from other slots.

The vision encoder step (`EncodeChunk`) does take time, but it's independent of the LLM context — it could theoretically run async, though that's a future optimization.

---

## Implementation Summary

### Changes Made

#### 1. Extended Slot Struct (`batch.go`)

Added new fields to `slot` for mtmd processing:
```go
// MTMD (multi-modal) fields for vision/audio processing.
inputChunks  mtmd.InputChunks // Tokenized chunks (text + media)
chunkIdx     int              // Current chunk being processed
bitmaps      []mtmd.Bitmap    // Bitmaps to free when done
useMRoPE     bool             // Model uses M-RoPE for positioning
useNonCausal bool             // Model uses non-causal attention for media
```

#### 2. Split startSlot into Text/Media Paths

- `startSlotText()` - handles text-only requests (original behavior)
- `startSlotMedia()` - handles media requests:
  - Creates bitmaps from `job.media`
  - Tokenizes with `mtmd.Tokenize` → `inputChunks`
  - Sets `useMRoPE`/`useNonCausal` flags

#### 3. New Chunk-Based Prefill

Added `addPrefillMediaChunk()` function that:
- Processes text chunks by adding tokens to batch (with M-RoPE support)
- Processes image chunks via `EncodeChunk` → `GetOutputEmbd` → decode embeddings

#### 4. New Decode Helpers

Added three decode helper functions:
- `decodeTextMRoPE()` - decodes text tokens with 4D M-RoPE positions
- `decodeEmbeddingsNormal()` - decodes embeddings with linear positions
- `decodeEmbeddingsMRoPE()` - decodes embeddings with 2D M-RoPE grid positions

#### 5. Updated processBatch

Added separate loop for media slots that calls `addPrefillMediaChunk()`.

#### 6. Enabled in chat.go

Changed condition from `object == ObjectChatText` to `object == ObjectChatText || object == ObjectChatMedia`.

---

## Key Implementation Details for MTMD

| Aspect | Batch Engine Implementation |
|--------|----------------------------|
| **Data Structure** | `mtmd.InputChunks` (text + image chunks) |
| **Prefill** | Iterate chunks: text via `Decode`, images via `EncodeChunk` + `GetOutputEmbd` + decode embeddings |
| **Position Handling** | M-RoPE requires 4D positions for images |
| **Attention** | Non-causal for image embeddings on some models |

---

---

## Design Considerations

### NBatch Coordination for Image Embeddings

**The constraint:** `NBatch` limits how many tokens can be processed in a single `llama.Decode` call.

**Why this matters for images:**

A single image can produce hundreds or thousands of embedding tokens. For example:
- A typical vision model might produce **576+ tokens** for one image
- Some high-resolution models produce even more

**The coordination challenge:**

For **text prefill**, the batch engine already handles this by chunking:
```go
chunkSize := min(remaining, nBatch, availableInBatch)
```

For **image embeddings**, we face a different situation:
1. Embeddings are decoded in a **separate** `llama.Decode` call (can't mix with text tokens)
2. If one image produces 576 embedding tokens and `NBatch=512`, we'd need to chunk the embedding decode too
3. The current step5 example decodes all embeddings in one call — this works when NBatch is large enough

**Potential approaches:**

1. **Require NBatch ≥ max image tokens** — simplest, but limits configuration flexibility
2. **Chunk embedding decode** — split large embedding batches like we do for text
3. **Process one image at a time** — serialize image processing within a slot, batch text across slots

**Current step5 behavior:** Decodes all image embeddings in one call without chunking. This assumes NBatch is sufficient for the model's image token count.

---

## Reference

- Working example: `examples/yzma/step5/main.go`
- Batch engine: `sdk/kronk/model/batch.go`
