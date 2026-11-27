package vram

import (
	"errors"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
)

// configureDSpark recognizes the host-synchronized, own-KV Markov sidecar.
// Borrowed token/output tables are not part of this model's weight residency.
func configureDSpark(input *Input, metadata map[string]string, tensors []gguf.TensorInfo) error {
	if input.Architecture != "dflash" {
		return nil
	}
	var vocabulary int64
	for _, tensor := range tensors {
		if tensor.Name == "markov_w1.weight" && len(tensor.Dims) == 2 {
			vocabulary = tensor.Dims[1]
		}
	}
	if vocabulary <= 0 || metadata["dflash.has_markov_context"] != "true" ||
		metadata["dflash.has_confidence_head"] != "false" || metadata["dflash.sample_from_anchor"] != "false" {
		return nil
	}
	block, err := gguf.ParseInt64(metadata, "dflash.block_size")
	if err != nil || block <= 0 {
		return errors.New("dspark requires a positive block_size")
	}
	layers, err := gguf.ParseInt64OrArray(metadata, "dflash.target_layers")
	if err != nil || len(layers) == 0 || input.EmbeddingLength <= 0 {
		return errors.New("dspark requires extraction and embedding dimensions")
	}
	input.VocabularySize = max(input.VocabularySize, vocabulary)
	input.dsparkBlockSize = block
	input.dsparkFeatureWidth = int64(len(layers)) * input.EmbeddingLength
	input.OutputRows = min(block*max(input.Slots, 1), dsparkBatch(*input))
	// No target rollback state or appended MTP layers live in this context.
	input.RecurrentStateCopies = 1
	input.ComputeContexts = 1
	input.MTPContext = false
	input.NextNPredictLayers = 0
	input.NoPersistentCache = false
	return nil
}

func dsparkBatch(input Input) int64 {
	if input.NBatch > 0 {
		return input.NBatch
	}
	if input.NUBatch > 0 {
		return input.NUBatch
	}
	return DefaultNUBatch
}

// dsparkComputeMemory separates allocation-shaped host storage from unmeasured
// graph allowances. KV uses the ordinary independent per-layer cache policy.
func dsparkComputeMemory(input Input) ComputeMemory {
	batch := dsparkBatch(input)
	physical := input.NUBatch
	if physical <= 0 {
		physical = DefaultNUBatch
	}
	physical = min(physical, batch)
	graph := input
	graph.NUBatch = physical
	memory := llamaComputeMemory(graph)
	width, features := input.EmbeddingLength, input.dsparkFeatureWidth
	rows := min(input.OutputRows, batch)
	// Target per-layer extraction slabs, the copied concatenated tray, and
	// BatchExt feature storage each cover ALL logical batch rows, not slots.
	// Dense NextN readback is a separate hidden-width logical-batch slab.
	// Markov ring entries retain a position tag (conservatively eight bytes).
	host := 12*batch*features + 4*batch*width
	// One reusable anchor/prediction state block, independent of sequence count.
	host += 4 * input.dsparkBlockSize * width
	// Shared target sampling may retain a float32 law, native-int indices and
	// a padded index/logit heap. Sparse proposal copies are bounded by their
	// packed native row width; one residual law is needed during verification.
	host += 28*input.VocabularySize + 4*width
	host += 4 * max(input.Slots, 1) * (input.dsparkBlockSize - 1) * width
	host += max(input.Slots, 1) * (input.dsparkBlockSize + 1) * (4*width + 8)
	// Native output buffers coexist with graph logits and dense NextN rows.
	host += 4 * rows * input.VocabularySize
	memory.WorkspaceCPU += host
	// Feature inputs plus Markov intermediates and borrowed-head projection
	// scratch. Two public-vocab logits slabs are a HEURISTIC allowance, not
	// an exact padded-row formula (the qualified pair has 201024 vs 200058
	// rows). This covers its padded head without charging the borrowed weights.
	memory.WorkspaceGPU += 4*physical*features + 8*physical*width + 8*physical*input.VocabularySize
	if input.GPULayers == -1 {
		memory.WorkspaceCPU += memory.WorkspaceGPU + memory.RuntimeAllowanceGPU
		memory.WorkspaceGPU, memory.RuntimeAllowanceGPU = 0, 0
	}
	memory.MarginGPU = (memory.WorkspaceGPU + memory.RuntimeAllowanceGPU) / 10
	memory.MarginCPU = memory.WorkspaceCPU / 10
	return memory
}
