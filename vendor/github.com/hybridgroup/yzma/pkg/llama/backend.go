package llama

import (
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/utils"
	"github.com/jupiterrider/ffi"
)

var (
	backendInitFunc ffi.Fun

	backendFreeFunc ffi.Fun

	// LLAMA_API void llama_numa_init(enum ggml_numa_strategy numa);
	numaInitFunc ffi.Fun

	// LLAMA_API size_t llama_max_devices(void);
	maxDevicesFunc ffi.Fun

	// LLAMA_API size_t llama_max_parallel_sequences(void);
	maxParallelSequencesFunc ffi.Fun

	// LLAMA_API size_t llama_max_tensor_buft_overrides(void);
	maxTensorBuftOverridesFunc ffi.Fun

	// LLAMA_API bool llama_supports_mmap(void);
	supportsMmapFunc ffi.Fun

	// LLAMA_API bool llama_supports_mlock(void);
	supportsMlockFunc ffi.Fun

	// LLAMA_API bool llama_supports_gpu_offload(void);
	supportsGpuOffloadFunc ffi.Fun

	// LLAMA_API bool llama_supports_rpc(void);
	supportsRpcFunc ffi.Fun

	// LLAMA_API int64_t llama_time_us(void);
	timeUsFunc ffi.Fun

	// LLAMA_API const char * llama_flash_attn_type_name(enum llama_flash_attn_type flash_attn_type);
	flashAttnTypeNameFunc ffi.Fun

	// LLAMA_API const char * llama_print_system_info(void);
	printSystemInfoFunc ffi.Fun
)

func loadBackendFuncs(lib ffi.Lib) error {
	var err error
	if backendInitFunc, err = lib.Prep("llama_backend_init", &ffi.TypeVoid); err != nil {
		return loadError("llama_backend_init", err)
	}

	if backendFreeFunc, err = lib.Prep("llama_backend_free", &ffi.TypeVoid); err != nil {
		return loadError("llama_backend_free", err)
	}

	if numaInitFunc, err = lib.Prep("llama_numa_init", &ffi.TypeVoid, &ffi.TypeSint32); err != nil {
		return loadError("llama_numa_init", err)
	}

	if maxDevicesFunc, err = lib.Prep("llama_max_devices", &ffiTypeSize); err != nil {
		return loadError("llama_max_devices", err)
	}

	if maxParallelSequencesFunc, err = lib.Prep("llama_max_parallel_sequences", &ffiTypeSize); err != nil {
		return loadError("llama_max_parallel_sequences", err)
	}

	if maxTensorBuftOverridesFunc, err = lib.Prep("llama_max_tensor_buft_overrides", &ffiTypeSize); err != nil {
		return loadError("llama_max_tensor_buft_overrides", err)
	}

	if supportsMmapFunc, err = lib.Prep("llama_supports_mmap", &ffi.TypeUint8); err != nil {
		return loadError("llama_supports_mmap", err)
	}

	if supportsMlockFunc, err = lib.Prep("llama_supports_mlock", &ffi.TypeUint8); err != nil {
		return loadError("llama_supports_mlock", err)
	}

	if supportsGpuOffloadFunc, err = lib.Prep("llama_supports_gpu_offload", &ffi.TypeUint8); err != nil {
		return loadError("llama_supports_gpu_offload", err)
	}

	if supportsRpcFunc, err = lib.Prep("llama_supports_rpc", &ffi.TypeUint8); err != nil {
		return loadError("llama_supports_rpc", err)
	}

	if timeUsFunc, err = lib.Prep("llama_time_us", &ffi.TypeSint64); err != nil {
		return loadError("llama_time_us", err)
	}

	if flashAttnTypeNameFunc, err = lib.Prep("llama_flash_attn_type_name", &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("llama_flash_attn_type_name", err)
	}

	if printSystemInfoFunc, err = lib.Prep("llama_print_system_info", &ffi.TypePointer); err != nil {
		return loadError("llama_print_system_info", err)
	}

	return nil
}

// BackendInit initializes the llama.cpp back-end.
func BackendInit() {
	backendInitFunc.Call(nil)
}

// BackendFree frees the llama.cpp back-end.
func BackendFree() {
	backendFreeFunc.Call(nil)
}

// NumaInit initializes NUMA with the given strategy.
func NumaInit(numaStrategy NumaStrategy) {
	numaInitFunc.Call(nil, unsafe.Pointer(&numaStrategy))
}

// MaxDevices returns the maximum number of devices supported.
func MaxDevices() uint64 {
	var result ffi.Arg
	maxDevicesFunc.Call(unsafe.Pointer(&result))
	return uint64(result)
}

// MaxParallelSequences returns the maximum number of parallel sequences supported.
func MaxParallelSequences() uint64 {
	var result ffi.Arg
	maxParallelSequencesFunc.Call(unsafe.Pointer(&result))
	return uint64(result)
}

// MaxTensorBuftOverrides returns the maximum number of tensor buffer overrides supported.
func MaxTensorBuftOverrides() uint64 {
	var result ffi.Arg
	maxTensorBuftOverridesFunc.Call(unsafe.Pointer(&result))
	return uint64(result)
}

// SupportsMmap checks if memory-mapped files are supported.
func SupportsMmap() bool {
	var result ffi.Arg
	supportsMmapFunc.Call(unsafe.Pointer(&result))
	return result.Bool()
}

// SupportsMlock checks if memory locking is supported.
func SupportsMlock() bool {
	var result ffi.Arg
	supportsMlockFunc.Call(unsafe.Pointer(&result))
	return result.Bool()
}

// SupportsGpuOffload checks if GPU offloading is supported.
func SupportsGpuOffload() bool {
	var result ffi.Arg
	supportsGpuOffloadFunc.Call(unsafe.Pointer(&result))
	return result.Bool()
}

// SupportsRpc checks if RPC is supported.
func SupportsRpc() bool {
	var result ffi.Arg
	supportsRpcFunc.Call(unsafe.Pointer(&result))
	return result.Bool()
}

// TimeUs returns the current time in microseconds.
func TimeUs() int64 {
	var result ffi.Arg
	timeUsFunc.Call(unsafe.Pointer(&result))
	return int64(result)
}

// FlashAttnTypeName returns the name for a given flash attention type.
func FlashAttnTypeName(flashAttnType FlashAttentionType) string {
	var result *byte
	flashAttnTypeNameFunc.Call(unsafe.Pointer(&result), &flashAttnType)

	if result == nil {
		return ""
	}

	return utils.BytePtrToString(result)
}

// PrintSystemInfo returns system information as a string.
func PrintSystemInfo() string {
	var result *byte
	printSystemInfoFunc.Call(unsafe.Pointer(&result))

	if result == nil {
		return ""
	}

	return utils.BytePtrToString(result)
}
