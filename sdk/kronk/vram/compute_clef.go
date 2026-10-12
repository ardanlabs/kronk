package vram

import "strings"

// clefComputeMemory owns Clef's batch-local decision graph allowances. Unlike
// autoregressive Qwen graphs, it retains neither KV nor recurrent state and
// does not produce vocabulary-sized logits. Keep its policy independent.
func clefComputeMemory(input Input) ComputeMemory {
	const runtime int64 = 256 * 1024 * 1024
	gpu, cpu := clefWorkspace(input)
	return ComputeMemory{
		WorkspaceGPU:        gpu,
		WorkspaceCPU:        cpu,
		RuntimeAllowanceGPU: runtime,
		MarginGPU:           (gpu + runtime) / 10,
		MarginCPU:           cpu / 10,
	}
}

// clefWorkspace sizes attention against the evaluated batch, not the configured
// context. Temporary delta-net states and the decision head are covered by the
// activation/runtime heuristics; layers share the compute arena. Larger physical
// batches grow both activations and the quadratic no-cache attention mask.
func clefWorkspace(input Input) (gpu, cpu int64) {
	batch := input.NUBatch
	if batch <= 0 {
		batch = DefaultNUBatch
	}
	batch = min(batch, max(input.ContextWindow, 1)*max(input.Slots, 1))
	maskWidth := int64(2)
	if input.VTransposed {
		maskWidth = 4
	}
	mask := maskWidth * batch * batch
	attention := mask
	if strings.EqualFold(input.ComputeBackend, "metal") && !input.VTransposed {
		// Metal's F16 K/V scratch covers the current batch only. Clef has
		// no persistent cache to move to CPU or multiply by sequence streams.
		kBytes := ((2*input.KeyLength*input.HeadCountKV*batch + 15) / 16) * 16
		vBytes := ((2*input.ValueLength*input.HeadCountKV*batch + 15) / 16) * 16
		attention += kBytes + vBytes
	}
	if input.VTransposed {
		attention += 8 * batch * batch * max(input.HeadCount, 1)
	}
	slotMultiplier := 1.0 + 0.25*float64(max(input.Slots, 1)-1)
	activation := int64(float64(32*batch*input.EmbeddingLength) * slotMultiplier)
	gpu = activation + attention
	// Four F32 hidden-width slabs approximate host graph inputs. This is
	// a workspace allowance, not a persistent state allocation per layer.
	cpu = mask + 16*input.EmbeddingLength*batch
	return gpu, cpu
}
