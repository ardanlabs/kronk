package vram

import (
	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// ComputeMemory separates shared execution workspace from the runtime allowance
// and safety margin. These are per native context, not fixed per-slot costs;
// Calculate reports the combined breakdown for all configured compute contexts.
type ComputeMemory struct {
	WorkspaceGPU        int64 // Architecture-specific scratch plus activation storage.
	WorkspaceCPU        int64 // Host scratch that coexists with GPU workspace.
	RuntimeAllowanceGPU int64 // Fixed backend/bookkeeping allowance per context.
	MarginGPU           int64 // Architecture's safety margin for GPU compute memory.
	MarginCPU           int64 // Architecture's safety margin for host workspace.
}

// GPUBytes returns the GPU compute reservation, including allowance and margin.
func (m ComputeMemory) GPUBytes() int64 {
	return m.WorkspaceGPU + m.RuntimeAllowanceGPU + m.MarginGPU
}

// CPUBytes returns the host compute reservation, including margin.
func (m ComputeMemory) CPUBytes() int64 {
	return m.WorkspaceCPU + m.MarginCPU
}

// EstimateComputeBuffer estimates GPU compute storage, including the selected
// architecture's margin. Architecture-specific paths also estimate host storage;
// Calculate includes that storage in system and unified-memory totals.
func EstimateComputeBuffer(input Input) int64 {
	return estimateComputeMemory(input).GPUBytes()
}

func estimateComputeMemory(input Input) ComputeMemory {
	return selectMemoryPolicy(input).compute(input)
}

// memoryPolicy keeps architecture selection consistent across compute, persistent
// state, and per-device cache sizing. A nil deviceCache uses proportional sizing.
type memoryPolicy struct {
	compute     func(Input) ComputeMemory
	cache       func(Input) gguf.KVCache
	deviceCache func(Input, []float64) []int64
}

func selectMemoryPolicy(input Input) memoryPolicy {
	policy := memoryPolicy{
		compute: genericComputeMemory,
		cache:   genericCache,
	}

	switch {
	case input.ComputeKind == modelprofile.ComputeDeepSeek41:
		policy.compute = deepseek41ComputeMemory
		policy.cache = deepseek41Cache
		policy.deviceCache = deepseek41DeviceCache
	case input.ComputeKind == modelprofile.ComputeGemma4:
		policy.compute = gemma4ComputeMemory
	case input.ComputeKind == modelprofile.ComputeQwen35:
		policy.compute = qwen35ComputeMemory
	case input.ComputeKind == modelprofile.ComputeGPTOSS:
		policy.compute = gptossComputeMemory
	case input.ComputeKind == modelprofile.ComputeMistral3:
		policy.compute = mistral3ComputeMemory
	case input.ComputeKind == modelprofile.ComputeLFM2:
		policy.compute = lfm2ComputeMemory
	case input.ComputeKind == modelprofile.ComputeGLM4MoE:
		policy.compute = glm4moeComputeMemory
	case input.ComputeKind == modelprofile.ComputeGemma3:
		policy.compute = gemma3ComputeMemory
	case input.ComputeKind == modelprofile.ComputeLlama:
		policy.compute = llamaComputeMemory
	case input.ComputeKind == modelprofile.ComputeClef:
		policy.compute = clefComputeMemory
		policy.cache = noPersistentCache
	case input.ComputeKind == modelprofile.ComputeModernBERT:
		policy.compute = modernbertComputeMemory
		policy.cache = noPersistentCache
	case input.ComputeKind == modelprofile.ComputeIndexed || input.IndexerKeyLength > 0:
		policy.compute = qwen4expComputeMemory
	}

	return policy
}
