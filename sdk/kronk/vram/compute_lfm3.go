package vram

import "strings"

// lfm3ComputeMemory is an uncalibrated, conservative workspace allowance, not
// LFM2's measured policy. b11960 builds separate full/SWA masks and gated Q/K
// normalized attention. Budget both masks at full width even with compact SWA,
// and do not assume that logits and attention scratch can share their peak.
func lfm3ComputeMemory(input Input) ComputeMemory {
	const runtime int64 = 256 * 1024 * 1024
	batch := input.NUBatch
	if batch <= 0 {
		batch = DefaultNUBatch
	}
	slots := max(input.Slots, 1)
	batch = min(batch, max(input.ContextWindow, 1)*slots)
	cells := ((max(input.ContextWindow, 1)*slots + 255) / 256) * 256
	streams := slots
	if input.KVUnified {
		streams = 1
	}
	cells = ((cells/streams + 255) / 256) * 256
	if input.NoPersistentCache {
		// build_attn_inp_no_cache pads the current batch's mask width
		// to 256 even though there is no persistent KV allocation.
		cells, streams = ((batch+255)/256)*256, 1
	}
	maskWidth := int64(2)
	if input.VTransposed {
		maskWidth = 4
	}
	mask := 2 * maskWidth * cells * batch
	attention := mask
	if strings.EqualFold(input.ComputeBackend, "metal") && !input.VTransposed && (!input.KVCacheOnCPU || input.NoPersistentCache) {
		// Full-sized F16 K/V scratch is an upper allowance for either
		// attention class; do not average KV heads over recurrent layers.
		heads := max(input.HeadCountKV, input.SWAHeadCountKV)
		for _, h := range input.HeadCountKVByLayer {
			heads = max(heads, h)
		}
		k := ((2*max(input.KeyLength, input.SWAKeyLength)*heads*cells*streams + 15) / 16) * 16
		v := ((2*max(input.ValueLength, input.SWAValueLength)*heads*cells*streams + 15) / 16) * 16
		attention += k + v
	}
	if input.VTransposed {
		attention += 8 * cells * batch * max(input.HeadCount, 1)
	}
	rows := input.OutputRows
	if rows <= 0 {
		rows = batch
	}
	gpu := genericActivationWorkspace(input) + attention + 4*input.VocabularySize*min(rows, batch)
	cpu := mask + 16*input.EmbeddingLength*batch
	return ComputeMemory{
		WorkspaceGPU:        gpu,
		WorkspaceCPU:        cpu,
		RuntimeAllowanceGPU: runtime,
		MarginGPU:           (gpu + runtime) / 10,
		MarginCPU:           cpu / 10,
	}
}
