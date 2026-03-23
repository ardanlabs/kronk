package llama

import (
	"fmt"
	"unsafe"

	"github.com/jupiterrider/ffi"
)

// PerfContextData represents the C struct llama_perf_context_data
type PerfContextData struct {
	TStartMs      float64 // absolute start time
	TLoadMs       float64 // time needed for loading the model
	TPromptEvalMs float64 // time needed for processing the prompt
	TEvalMs       float64 // time needed for generating tokens

	NPEval  int32 // number of prompt tokens
	NEval   int32 // number of generated tokens
	NReused int32 // number of times a ggml compute graph had been reused
}

// String returns a formatted string representation of PerfContextData
func (p PerfContextData) String() string {
	return fmt.Sprintf("PerfContextData{Start: %.2fms, Load: %.2fms, Prompt Eval: %.2fms, Eval: %.2fms, Prompt Tokens: %d, Gen Tokens: %d, Reused: %d}",
		p.TStartMs, p.TLoadMs, p.TPromptEvalMs, p.TEvalMs, p.NPEval, p.NEval, p.NReused)
}

// PerfSamplerData represents the C struct llama_perf_sampler_data
type PerfSamplerData struct {
	TSampleMs float64 // time needed for sampling in ms

	NSample int32 // number of sampled tokens
}

// String returns a formatted string representation of PerfSamplerData
func (p PerfSamplerData) String() string {
	return fmt.Sprintf("PerfSamplerData{Sample Time: %.2fms, Samples: %d}", p.TSampleMs, p.NSample)
}

var (
	// LLAMA_API struct llama_perf_context_data llama_perf_context(const struct llama_context * ctx);
	perfContextFunc ffi.Fun

	// LLAMA_API void llama_perf_context_print(const struct llama_context * ctx);
	perfContextPrintFunc ffi.Fun

	// LLAMA_API struct llama_perf_sampler_data llama_perf_sampler(const struct llama_sampler * chain);
	perfSamplerFunc ffi.Fun

	// LLAMA_API void llama_perf_sampler_print(const struct llama_sampler * chain);
	perfSamplerPrintFunc ffi.Fun

	// LLAMA_API void llama_perf_sampler_reset(struct llama_sampler * chain);
	perfSamplerResetFunc ffi.Fun

	// LLAMA_API void llama_memory_breakdown_print(const struct llama_context * ctx);
	memoryBreakdownPrintFunc ffi.Fun
)

func loadPerfFuncs(lib ffi.Lib) error {
	var err error

	if perfContextFunc, err = lib.Prep("llama_perf_context", &ffiPerfContextData, &ffi.TypePointer); err != nil {
		return loadError("llama_perf_context", err)
	}

	if perfContextPrintFunc, err = lib.Prep("llama_perf_context_print", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("llama_perf_context_print", err)
	}

	if perfSamplerFunc, err = lib.Prep("llama_perf_sampler", &ffiPerfSamplerData, &ffi.TypePointer); err != nil {
		return loadError("llama_perf_sampler", err)
	}

	if perfSamplerPrintFunc, err = lib.Prep("llama_perf_sampler_print", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("llama_perf_sampler_print", err)
	}

	if perfSamplerResetFunc, err = lib.Prep("llama_perf_sampler_reset", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("llama_perf_sampler_reset", err)
	}

	if memoryBreakdownPrintFunc, err = lib.Prep("llama_memory_breakdown_print", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("llama_memory_breakdown_print", err)
	}

	return nil
}

// ffiPerfContextData represents the C struct llama_perf_context_data
var ffiPerfContextData = ffi.NewType(&ffi.TypeDouble, &ffi.TypeDouble, &ffi.TypeDouble, &ffi.TypeDouble, &ffi.TypeSint32, &ffi.TypeSint32, &ffi.TypeSint32)

// ffiPerfSamplerData represents the C struct llama_perf_sampler_data
var ffiPerfSamplerData = ffi.NewType(&ffi.TypeDouble, &ffi.TypeSint32)

// PerfContext returns performance data for the model context.
func PerfContext(ctx Context) PerfContextData {
	var data PerfContextData
	if ctx == 0 {
		return data
	}
	perfContextFunc.Call(unsafe.Pointer(&data), unsafe.Pointer(&ctx))
	return data
}

// PerfSampler returns performance data for the sampler.
func PerfSampler(chain Sampler) PerfSamplerData {
	var data PerfSamplerData
	if chain == 0 {
		return data
	}
	perfSamplerFunc.Call(unsafe.Pointer(&data), unsafe.Pointer(&chain))
	return data
}

// PerfSamplerReset resets sampler performance metrics.
func PerfSamplerReset(chain Sampler) {
	if chain == 0 {
		return
	}
	perfSamplerResetFunc.Call(nil, unsafe.Pointer(&chain))
}

// PerfContextPrint prints performance data for the model context.
func PerfContextPrint(ctx Context) {
	if ctx == 0 {
		return
	}
	perfContextPrintFunc.Call(nil, unsafe.Pointer(&ctx))
}

// PerfSamplerPrint prints performance data for the sampler.
func PerfSamplerPrint(chain Sampler) {
	if chain == 0 {
		return
	}
	perfSamplerPrintFunc.Call(nil, unsafe.Pointer(&chain))
}

// MemoryBreakdownPrint prints a breakdown of per-device memory use.
func MemoryBreakdownPrint(ctx Context) {
	if ctx == 0 {
		return
	}
	memoryBreakdownPrintFunc.Call(nil, unsafe.Pointer(&ctx))
}
