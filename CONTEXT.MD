Design a high-level architecture for adding parallel LLM inference support to the Model package.
Here's the high-level design based on the oracle's analysis:

Parallel Inference Design for Model Package
Architecture Overview
┌─────────────────────────────────────────────────────────────────┐
│ Model │
│ ┌──────────────┐ ┌─────────────────────────────────────┐ │
│ │ChatStreaming │───►│ requestQueue │ │
│ │ (public) │ └──────────────┬──────────────────────┘ │
│ └──────────────┘ │ │
│ ▲ ▼ │
│ │ ┌─────────────────────────────┐ │
│ │ │ Batch Engine Goroutine │ │
│ │ │ (single owner of lctx) │ │
│ │ │ │ │
│ │ │ ┌─────┐ ┌─────┐ ┌─────┐ │ │
│ response │ │Slot0│ │Slot1│ │SlotN│ │ │
│ channels │ └──┬──┘ └──┬──┘ └──┬──┘ │ │
│ │ │ │ │ │ │ │
│ │ │ └───────┴───────┘ │ │
│ │ │ │ │ │
│ │ │ ┌──────▼──────┐ │ │
│ │ │ │ llama.Batch │ │ │
│ │ │ │ Decode() │ │ │
│ │ │ └─────────────┘ │ │
│ │ └─────────────────────────────┘ │
│ │ │ │
│ └────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘

1. New/Modified Structs
   Config addition:

type Config struct {
// ... existing fields
NParallel int // Number of parallel slots (default 1 = sequential)
}
New internal structs:

Struct Purpose
chatJob Validated request ready for processing (id, ctx, prompt, params, response channel)
slot Per-request state: seqID, sampler, processor, position tracking, builders
batchEngine Owns the processing loop, slots, request queue
Model additions (internal):

type Model struct {
// ... existing fields
mem llama.Memory // for MemorySeqRm/MemorySeqCp
batch llama.Batch // shared batch buffer
engine \*batchEngine // nil when NParallel <= 1
} 2. Request Flow
ChatStreaming (unchanged signature):

Validate document, prepare media context, create prompt (as today)
If parallel enabled AND text-only → create chatJob, enqueue to requestQueue
If media OR parallel disabled → use existing processChatRequest path
Batch Engine Loop (new):

Poll queue, assign jobs to free slots
Each iteration: build combined batch, single Decode(), per-slot sampling
Stream tokens via existing sendDeltaResponse
On completion: sendFinalResponse, clear slot's KV cache 3. Processor Per-Slot
Refactor processor to expose stateless step methods:

func (p *processor) StepStandard(content string) (response, eog bool)
func (p *processor) StepGPT(content string) (response, eog bool)
Each slot owns its own processor instance—no shared state between concurrent requests.

4. Media/Vision Handling
   Phase 1: Keep media requests on the sequential path (existing processChatRequest)

When object == ObjectChatMedia, bypass the batch engine
This avoids complexity with mtmd.Context sharing
Phase 2 (future): Could add media support to batch engine if needed

5. Lifecycle
   Phase Action
   NewModel If NParallel > 1: allocate batch, slots, start engine goroutine
   ChatStreaming Enqueue job (parallel) or process directly (sequential)
   Unload Signal shutdown, drain engine, wait for activeStreams, free resources
6. Backward Compatibility
   NParallel defaults to 1 → existing behavior unchanged
   ChatStreaming signature unchanged
   ChatResponse format unchanged
   Existing single-threaded callers work without modification
   Summary: The exported API stays the same except for the optional Config.NParallel field. All parallelism is internal. Media/vision stays on the sequential path initially.

Want me to break this down into implementation phases?

Yes please

The user wants me to break down the implementation into phases. Let me think about logical incremental steps that minimize risk and allow testing at each stage.

Implementation Phases
Phase 1: Foundation (No Behavior Change)
Goal: Add new structs and config without changing any behavior

Add NParallel int to Config struct
Add mem llama.Memory field to Model (populate in NewModel)
Define internal structs: chatJob, slot, batchEngine (empty implementations)
Adjust modelCtxParams to set NSeqMax based on NParallel
Test: All existing tests pass, behavior unchanged

Phase 2: Refactor Processor
Goal: Make processor usable per-slot without calling llama directly

Add StepStandard(content string, token llama.Token) (response, bool)
Add StepGPT(content string, token llama.Token) (response, bool)
These extract the state-machine logic from current standard/gpt methods
Keep existing methods working (they call the new Step methods internally)
Test: Existing tests still pass, processor logic unchanged

Phase 3: Batch Engine Skeleton
Goal: Working engine loop that handles single slot (NParallel=1)

Implement batchEngine with request queue, single slot, processing loop
Extract slot lifecycle helpers: startSlot, processSlot, finishSlot
Wire ChatStreaming to enqueue to engine when NParallel >= 1 AND text-only
Media requests bypass engine, use existing processChatRequest
Test: Single-slot engine produces identical output to current implementation

Phase 4: Multi-Slot Parallelism
Goal: True parallel inference with N slots

Extend engine to manage N slots
Implement batched decode: combine all active slots into single llama.Batch
Per-slot sampling from batch positions
Per-slot KV cache management via MemorySeqRm/MemorySeqCp
Test: Multiple concurrent ChatStreaming calls process in parallel

Phase 5: Cleanup & Optimization
Goal: Production readiness

Proper shutdown/drain in Unload
Metrics and observability per-slot
Error isolation (one slot's error doesn't affect others)
System prompt caching across slots (optional optimization)
Dependency Graph
Phase 1 ──► Phase 2 ──► Phase 3 ──► Phase 4 ──► Phase 5
│ │
└────────────┴──── Can be done in parallel
Shall I start with Phase 1? I'll show you the proposed changes before making any edits.

In every phase will the code complile and I can run tests?

The user is asking if the code will compile and tests will pass at every phase. This is a good question about incremental development. Let me think about this:

Phase 1: Adding new fields and structs - yes, this should compile and tests pass since we're just adding things, not changing behavior.

Phase 2: Refactoring processor - this is where it gets tricky. If we add new methods that the existing methods call, we need to be careful. The existing methods need to keep working. This should be doable.

Phase 3: This is where it gets more complex. We're introducing a new code path (engine) for text-only requests. If NParallel >= 1 AND text-only, use engine. Otherwise use existing path. But wait - what if NParallel defaults to 1? Then all text requests would go through the new engine, which might not be ready.

Actually, I need to reconsider the phases. Let me think about how to make each phase truly incremental and safe:

Phase 1: Just add structs/fields, don't use them. Safe.

Phase 2: Refactor processor to have Step methods, but existing methods call them. Safe if done correctly.

Phase 3: This is where I need to be careful. The engine should only be activated when NParallel > 1. When NParallel <= 1 (the default), use existing path. This way existing behavior is unchanged.

Actually, looking back at my phases, I said "when NParallel >= 1 AND text-only" - but if NParallel defaults to 1, that would trigger the new path. I should change this to NParallel > 1 to enable the new path.

Let me reconsider:

NParallel = 0 or 1 (default): use existing sequential path
NParallel > 1: use batch engine
This way Phase 3 can be tested with NParallel=2+ while NParallel=1 keeps the existing behavior.

The safer approach is to build the engine but only activate it when NParallel > 1, keeping existing tests on the old path. Once multi-slot works, I could optionally make NParallel=1 also use the engine with a single slot for consistency, though that's not strictly necessary.

I should check what tests already exist in the model package to understand the current coverage. The key advantage of this phased approach is that at each step the code compiles and existing tests pass—the new engine only engages when NParallel exceeds 1, and since the default is 1, the old behavior remains untouched. New tests would be needed specifically for the parallel execution path.

Yes, with one clarification to my phases:

Phase 3 adjustment: The engine activates only when NParallel > 1. Default NParallel = 0 or 1 uses the existing processChatRequest path unchanged.

Phase Compiles? Existing Tests Pass? Why
1 ✓ ✓ Only adding structs/fields, nothing uses them yet
2 ✓ ✓ New Step\* methods; existing methods call them internally
3 ✓ ✓ Engine only activates when NParallel > 1; default uses old path
4 ✓ ✓ Same—parallel path is opt-in via config
5 ✓ ✓ Cleanup/optimization, no behavior change
Testing strategy:

Existing tests run with default config → use existing sequential path
New tests explicitly set NParallel > 1 → exercise the batch engine
This gives you a safe rollout where you can toggle between paths via config and compare results.

You have coded Phase 1, 2, and 3. We have 3 working with a single request when we use the batch engine (NSeqMax > 2).

These files that have been changed are these:

NEW: sdk/kronk/model/batch.go
MODIFIED: sdk/kronk/model/model.go
MODIFIED: sdk/kronk/model/processor.go

There is a sample working program here with what we are trying to add to the model package.
examples/yzma-parallel/step2/main.go
