package vram

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// unifiedAttentionWorkspace adds b12125's row-list inputs and Metal gathers to
// the existing dense-fallback workspace. Reservation uses full cache extents
// for every sequence slice, even when runtime lists can later compact.
func unifiedAttentionWorkspace(input Input) (gpu, cpu int64) {
	if !input.KVUnified || input.VTransposed || input.NoPersistentCache {
		return 0, 0
	}
	// Indexed/MLA graphs own their attention inputs and K/V aliasing. Do not
	// apply the ordinary GQA inventory to those independent policies.
	switch input.ComputeKind {
	case modelprofile.ComputeDeepSeek41, modelprofile.ComputeIndexed, modelprofile.ComputeClef, modelprofile.ComputeModernBERT:
		return 0, 0
	}
	if input.IndexerKeyLength > 0 {
		return 0, 0
	}
	if input.ComputeKind == modelprofile.ComputeGeneric &&
		!strings.HasPrefix(strings.ToLower(input.Architecture), "qwen") && !strings.EqualFold(input.Architecture, "lfm3") {
		return 0, 0
	}
	slots := max(input.Slots, 1)
	batch := input.NUBatch
	if batch <= 0 {
		batch = DefaultNUBatch
	}
	batch = min(batch, max(input.ContextWindow, 1)*slots)
	// graph_reserve rounds query capacity up to a multiple of n_seq_max.
	queries := ((batch + slots - 1) / slots) * slots
	cells := ((max(input.ContextWindow, 1)*slots + 255) / 256) * 256
	swaCells := cells
	if input.SlidingWindow > 0 && !input.SWAFull {
		swaCells = ((min(cells, input.SlidingWindow*slots+batch) + 255) / 256) * 256
	}
	trunk := max(input.BlockCount-max(input.NextNPredictLayers, 0), 0)
	start, end := int64(0), trunk
	if input.MTPContext {
		start, end = trunk, input.BlockCount
	}
	gpuStart := input.BlockCount - max(clampGPULayers(input.GPULayers, input.BlockCount)-1, 0)
	var full, swa, onGPU bool
	var scratch int64
	for layer := start; layer < end; layer++ {
		if layer < int64(len(input.RecurrentPattern)) && input.RecurrentPattern[layer] {
			continue
		}
		isSWA := layer < input.SlidingWindowLayers
		if int64(len(input.SWAPattern)) == input.BlockCount {
			isSWA = input.SWAPattern[layer]
		}
		heads, key, value, extent := input.HeadCountKV, input.KeyLength, input.ValueLength, cells
		if isSWA && input.SlidingWindow > 0 {
			heads, key, value, extent = input.SWAHeadCountKV, input.SWAKeyLength, input.SWAValueLength, swaCells
			if heads <= 0 {
				heads = input.HeadCountKV
			}
			if key <= 0 {
				key = input.KeyLength
			}
			if value <= 0 {
				value = input.ValueLength
			}
		}
		if layer < int64(len(input.HeadCountKVByLayer)) {
			heads = input.HeadCountKVByLayer[layer]
		}
		if heads <= 0 || key <= 0 || value <= 0 {
			continue
		}
		if isSWA {
			swa = true
		} else {
			full = true
		}
		if layer < gpuStart || input.KVCacheOnCPU {
			continue
		}
		onGPU = true
		if !strings.EqualFold(input.ComputeBackend, "metal") {
			continue
		}
		// extra_rows coexists with the original F16 K/V scratch. Each
		// region is aligned separately; layers reuse the same arena.
		k := ((2*key*heads*extent*slots + 15) / 16) * 16
		v := ((2*value*heads*extent*slots + 15) / 16) * 16
		// extra_tmp predates rows, but splitting queries enlarges it.
		tempRows := max(min(queries/slots, 32)*slots-min(batch, 32), 0)
		temp := 4 * tempRows * input.HeadCount * 32 * (value + 2)
		scratch = max(scratch, k+v+temp)
	}
	var maskCells int64
	if full {
		maskCells += cells
	}
	if swa {
		maskCells += swaCells
	}
	// I32 lists [cells, slices] and the extra rounded-query F16 mask
	// have host storage and a device copy. The mask is not multiplied
	// by slices again: its query dimension already includes all slices.
	cpu = 4*maskCells*slots + 2*maskCells*(queries-batch)
	if onGPU {
		gpu = cpu + scratch
	}
	return gpu, cpu
}
