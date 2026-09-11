// Package vram provides VRAM requirement calculation for stable-diffusion
// model contexts.
package vram

// RuntimeOverhead is the conservative transient VRAM reserve for one native
// stable-diffusion context. It matches stable-diffusion.cpp's largest pre-load
// auto-fit reserve: 2 GiB for diffusion and text-conditioning execution.
const RuntimeOverhead int64 = 2 << 30

// Config contains the parameters for VRAM calculation that are not derived
// from the installed model bundle.
type Config struct {
	Contexts int64 // Number of independently loaded native contexts. Values below one default to one.
}

// Input contains the parameters needed to calculate VRAM requirements.
type Input struct {
	ModelSizeBytes int64 // Combined size of every model component in bytes.
	Contexts       int64 // Number of independently loaded native contexts. Values below one default to one.
}

// Result contains the calculated VRAM requirements.
type Result struct {
	Input        Input
	ModelBytes   int64
	RuntimeBytes int64
	TotalVRAM    int64
}

// Calculate computes the conservative peak VRAM requirement for a Malina
// handle. Each native context owns a complete copy of the model weights and
// its transient execution buffers, so both are multiplied by the context
// count.
func Calculate(input Input) Result {
	contexts := input.Contexts
	if contexts <= 0 {
		contexts = 1
	}

	modelBytes := input.ModelSizeBytes * contexts
	runtimeBytes := RuntimeOverhead * contexts

	return Result{
		Input:        input,
		ModelBytes:   modelBytes,
		RuntimeBytes: runtimeBytes,
		TotalVRAM:    modelBytes + runtimeBytes,
	}
}
