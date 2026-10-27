package vram

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
)

// deepseek41ComputeMemory estimates an uncalibrated, leading-order b11901
// workspace. No full V4.1 model was available for measuring allocator peaks.
// Weights, raw/compressed K caches, compressor state, and Engram token history
// belong to residency/cache accounting, not this per-context compute arena.
func deepseek41ComputeMemory(input Input) ComputeMemory {
	runtime := int64(256 * 1024 * 1024)
	if input.ModelSizeBytes > 50*1024*1024*1024 {
		runtime = 512 * 1024 * 1024
	}
	gpu, cpu := deepseek41Workspace(input)
	return ComputeMemory{
		WorkspaceGPU:        gpu,
		WorkspaceCPU:        cpu,
		RuntimeAllowanceGPU: runtime,
		MarginGPU:           (gpu + runtime) / 10,
		MarginCPU:           cpu / 10,
	}
}

// deepseek41Workspace follows src/models/deepseek41.cpp build_attention41 and
// graph, deepseek4.cpp build_raw_attention/build_hc_*, and llama-graph.cpp
// build_inp_dsv4 at https://github.com/ggml-org/llama.cpp/tree/b11901/src.
// Layer-local work uses a peak, not a layer multiplier. Source rows/top-k and
// upfront host Engram gathers can survive across layers and are summed instead.
// This is an inventory envelope, not a simulation of graph tensor lifetimes,
// backend fusion, offload splits, or allocator reuse. FFN scratch remains an
// activation heuristic. Persistent allocations are sized separately.
func deepseek41Workspace(input Input) (gpu, cpu int64) {
	batch := input.NUBatch
	if batch <= 0 {
		batch = DefaultNUBatch
	}
	slots := max(input.Slots, 1)
	cells := ((max(input.ContextWindow, 1)*slots + 255) / 256) * 256
	batch = min(batch, cells)
	if !input.KVUnified {
		cells = ((cells/slots + 255) / 256) * 256
	}
	// llama-context.cpp normalizes n_ctx_seq before DSV4 ignores unified and
	// forces per-sequence streams. Public unified mode therefore still changes
	// per-stream capacity; it does NOT reduce the stream count to one.
	raw := cells
	if input.SlidingWindow > 0 && !input.SWAFull {
		raw = ((min(cells, input.SlidingWindow+batch) + 255) / 256) * 256
	}
	maskWidth := int64(2)
	if input.VTransposed {
		maskWidth = 4
	}
	heads := max(input.HeadCount, 1)
	key := max(input.KeyLength, 0)
	hc := max(input.HyperConnections, 1)
	// F32 residual/mix/FFN slabs, hyper-connection coefficients, Q/QR, KV,
	// attention output and grouped low-rank output projection intermediates.
	activation := 32*input.EmbeddingLength*hc*batch +
		16*hc*(hc+2)*batch +
		4*batch*(2*key*heads+key+max(input.QueryRank, 0)+max(input.OutputRank, 0)*max(input.OutputGroups, 0))
	mask := maskWidth * raw * batch
	cpu = mask + 4*input.EmbeddingLength*hc*batch
	var retained, peak int64
	stagedRows := raw * slots
	ratios := make(map[int64]bool)
	for layer, ratio := range input.CompressRatios {
		if ratio <= 0 {
			continue
		}
		comp := (((cells+ratio-1)/ratio + 255) / 256) * 256
		selected := min(comp, max(input.IndexerTopK, 0))
		if !ratios[ratio] {
			ratios[ratio] = true
			// One input mask and compression plan per ratio, not per layer.
			mask += maskWidth * comp * batch
			blocks := batch/ratio + min(slots, batch)
			plan := 4*batch + 12*blocks + 4*ratio*blocks + 8*min(batch, ratio*slots)
			cpu += maskWidth*comp*batch + plan
		}
		if input.KVSources[layer] == int64(layer) {
			blocks := batch/ratio + min(slots, batch)
			// New [KV | index K] rows can feed later consumers. This is F32
			// graph output, not another copy of the persistent K-only cache.
			retained += 4 * (key + max(input.IndexerKeyLength, 0)) * blocks
		}
		// Scores plus full-width I32 argsort (top_k is a view of argsort),
		// selection/scatter masks, and the concatenated raw/compressed keys.
		work := 8*comp*batch + maskWidth*(2*comp+raw+selected)*batch
		keyRows := (raw + 2*comp) * slots
		stage := (raw + comp) * slots
		if !input.VTransposed && ((slots == 1 && batch <= 32) || batch == slots) {
			// Small-query FA gathers/repeats the window per query, rather
			// than concatenating one dense K tensor per sequence stream.
			keyRows = max(keyRows, (2*raw+2*selected)*batch)
			stage = max(stage, (raw+selected)*batch)
		}
		stagedRows = max(stagedRows, stage)
		work += 4 * key * keyRows
		work += 4 * max(input.IndexerKeyLength, 0) * comp * slots
		if input.KVSources[layer] == int64(layer) {
			// Current KV/gate scores, restored-state concatenations, gathered
			// ratio groups and softmax/multiply/reduction temporaries.
			blocks := batch/ratio + min(slots, batch)
			work += 8*key*(batch+ratio*slots) + 16*key*ratio*blocks
		}
		if input.IndexerSources[layer] == int64(layer) {
			retained += 4 * selected * batch
			// Unfused indexer chunks near 2^26 head-score elements, with
			// at most 64 chunks. Fused lightning-indexer needs less scratch.
			streams := min(slots, batch)
			tokens := (batch + streams - 1) / streams
			indexHeads := max(input.IndexerHeads, 1)
			chunk := min(tokens, max((tokens+63)/64, (1<<26)/(indexHeads*comp*streams), 1))
			work += 8 * indexHeads * comp * chunk * streams
		}
		if input.VTransposed {
			work += 8 * (raw + comp) * batch * heads
		}
		peak = max(peak, work)
	}
	// Candidate selection does not remove full compressed scores. Its source
	// retains block ids; later index sources expand a full-width candidate mask.
	if source := input.CandidateSourceLayer; source >= 0 && input.CandidateBlockSize > 0 {
		ratio := input.CompressRatios[source]
		comp := (((cells+ratio-1)/ratio + 255) / 256) * 256
		blocks := (comp + input.CandidateBlockSize - 1) / input.CandidateBlockSize
		pin := 4 * blocks * batch
		cpu += pin
		retained += pin + 4*min(blocks, input.CandidateTopKBlocks)*batch
		peak += 4 * (blocks*input.CandidateBlockSize + blocks) * batch
	}
	// Engram rows are gathered on the host up front, then transferred for
	// their layer's projection. Reserve the retained rows on both sides;
	// projection/gating scratch uses a single layer's peak, not all modules.
	engram := 4 * input.LayerEmbeddingRows * input.LayerEmbeddingLength * batch
	cpu += engram + 4*input.LayerEmbeddingRows*batch
	retained += engram
	if input.LayerEmbeddingRows > 0 {
		activation += 8 * input.EmbeddingLength * (hc + 1) * batch
	}
	peak = max(peak, maskWidth*raw*batch)
	if input.VTransposed {
		peak = max(peak, 8*raw*batch*heads)
	}
	if strings.EqualFold(input.ComputeBackend, "metal") && !input.VTransposed && !input.KVCacheOnCPU {
		// Metal stages K and V independently as F16 even though both refer
		// to the same K-only tensor. Include dense and per-query gathers.
		peak += 2 * (((2*key*stagedRows + 15) / 16) * 16)
	}
	rows := input.OutputRows
	if rows <= 0 {
		rows = batch
	}
	logits := 4 * input.VocabularySize * min(rows, batch)
	// Input masks coexist, whereas attention and vocabulary projection reuse
	// the arena. Retained-source sums deliberately err on the cautious side.
	gpu = activation + mask + retained + max(peak, logits)
	return gpu, cpu
}

// deepseek41Cache follows b11901's llama_kv_cache_dsv4 constructor. Its
// architecture policy stays here: raw K-only SWA at every block, compressed
// [KV | index K] rows at sources/device mirrors, and two F32 state tensors at
// each source. Shared inputs contain only normalized dimensions and topology.
func deepseek41Cache(input Input) gguf.KVCache {
	streams := max(input.Slots, 1)
	cells := ((input.ContextWindow*streams + 255) / 256) * 256
	if !input.KVUnified {
		cells = ((cells/streams + 255) / 256) * 256
	}
	batch := input.NUBatch
	if batch <= 0 {
		batch = DefaultNUBatch
	}
	rawCells := cells
	if !input.SWAFull {
		rawCells = ((min(cells, input.SlidingWindow+batch) + 255) / 256) * 256
	}
	keyRow := input.KeyLength * input.BytesPerElement
	compressedRow := (input.KeyLength + input.IndexerKeyLength) * input.BytesPerElement
	if input.TypeK != 0 {
		typeK := uint32(input.TypeK)
		if typeK == 50 {
			typeK = 0 // Kronk's F32 ID differs from ggml's.
		}
		keyRow = gguf.GGMLRowSize(typeK, input.KeyLength)
		compressedRow = gguf.GGMLRowSize(typeK, input.KeyLength+input.IndexerKeyLength)
	}
	raw := keyRow * rawCells * streams
	layers := make([]int64, input.BlockCount)
	owners := make(map[[2]int64]bool)
	start := input.BlockCount - max(clampGPULayers(input.GPULayers, input.BlockCount)-1, 0)
	var total int64
	for layer := range input.BlockCount {
		bytes := raw
		source := input.KVSources[layer]
		if source >= 0 {
			device := int64(0)
			switch {
			case input.KVCacheOnCPU || layer < start:
				device = -1
			case len(input.cacheLayerDevices) > 0:
				device = int64(input.cacheLayerDevices[layer])
			}
			owner := [2]int64{source, device}
			if !owners[owner] {
				ratio := input.CompressRatios[layer]
				compressedCells := ((max(1, (cells+ratio-1)/ratio) + 255) / 256) * 256
				bytes += compressedRow * compressedCells * streams
				owners[owner] = true
			}
			if source == layer {
				bytes += 2 * 4 * input.KeyLength * input.CompressRatios[layer] * streams * max(input.RecurrentStateCopies, 1)
			}
		}
		layers[layer] = bytes
		total += bytes
	}
	return gguf.KVCache{
		KVPerTokenPerLayer: keyRow,
		KVPerSlot:          total / streams,
		SlotMemory:         total,
		LayerMemory:        layers,
	}
}

// deepseek41DeviceCache follows native layer-split boundaries, including the
// output in the offload count. Each source/device pair owns one compressed
// cache; only source blocks own compressor state. Weight splitting remains the
// calculator's existing proportional approximation.
func deepseek41DeviceCache(input Input, fractions []float64) []int64 {
	bytes := make([]int64, len(fractions))
	if input.KVCacheOnCPU {
		return bytes
	}
	offload := clampGPULayers(input.GPULayers, input.BlockCount)
	start := input.BlockCount - max(offload-1, 0)
	input.cacheLayerDevices = make([]int, input.BlockCount)
	for layer := range input.BlockCount {
		if layer < start {
			input.cacheLayerDevices[layer] = -1
			continue
		}
		position := float64(layer-start) / float64(offload)
		var cumulative float64
		for device, fraction := range fractions {
			cumulative += fraction
			if position < cumulative || device == len(fractions)-1 {
				input.cacheLayerDevices[layer] = device
				break
			}
		}
	}
	kv := deepseek41Cache(input)
	for layer, device := range input.cacheLayerDevices {
		if device >= 0 {
			bytes[device] += kv.LayerMemory[layer]
		}
	}
	return bytes
}
