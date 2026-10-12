package vram

import "github.com/ardanlabs/kronk/sdk/kronk/modelprofile"

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
	switch {
	case input.ComputeKind == modelprofile.ComputeGemma4:
		return gemma4ComputeMemory(input)
	case input.ComputeKind == modelprofile.ComputeQwen35:
		return qwen35ComputeMemory(input)
	case input.ComputeKind == modelprofile.ComputeGPTOSS:
		return gptossComputeMemory(input)
	case input.ComputeKind == modelprofile.ComputeMistral3:
		return mistral3ComputeMemory(input)
	case input.ComputeKind == modelprofile.ComputeLFM2:
		return lfm2ComputeMemory(input)
	case input.ComputeKind == modelprofile.ComputeGLM4MoE:
		return glm4moeComputeMemory(input)
	case input.ComputeKind == modelprofile.ComputeGemma3:
		return gemma3ComputeMemory(input)
	case input.ComputeKind == modelprofile.ComputeLlama:
		return llamaComputeMemory(input)
	case input.ComputeKind == modelprofile.ComputeClef:
		return clefComputeMemory(input)
	case input.ComputeKind == modelprofile.ComputeModernBERT:
		return modernbertComputeMemory(input)
	case input.ComputeKind == modelprofile.ComputeIndexed || input.IndexerKeyLength > 0:
		return qwen4expComputeMemory(input)
	default:
		return genericComputeMemory(input)
	}
}
