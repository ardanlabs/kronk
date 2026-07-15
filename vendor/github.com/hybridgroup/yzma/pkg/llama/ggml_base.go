package llama

import (
	"fmt"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/utils"
	"github.com/jupiterrider/ffi"
)

// Opaque types (represented as pointers)
type GGMLBackendBufferType uintptr

var (
	// GGML_API ggml_backend_buffer_type_t ggml_backend_cpu_buffer_type(void);
	ggmlBackendCpuBufferType ffi.Fun

	// GGML_API const char * ggml_backend_dev_name(ggml_backend_dev_t device);
	ggmlBackendDevNameFunc ffi.Fun

	// GGML_API void ggml_backend_dev_memory(ggml_backend_dev_t device, size_t * free, size_t * total);
	ggmlBackendDevMemoryFunc ffi.Fun
)

func loadGGMLBase(lib ffi.Lib) error {
	var err error

	if ggmlBackendCpuBufferType, err = lib.Prep("ggml_backend_cpu_buffer_type", &ffi.TypeVoid); err != nil {
		return loadError("ggml_backend_cpu_buffer_type", err)
	}

	if ggmlBackendDevNameFunc, err = lib.Prep("ggml_backend_dev_name", &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("ggml_backend_dev_name", err)
	}

	if ggmlBackendDevMemoryFunc, err = lib.Prep("ggml_backend_dev_memory", &ffi.TypeVoid, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("ggml_backend_dev_memory", err)
	}

	return nil
}

// GGMLBackendCpuBufferType returns the buffer type used for CPU backends.
func GGMLBackendCpuBufferType() GGMLBackendBufferType {
	var ret uintptr
	ggmlBackendCpuBufferType.Call(&ret)
	return GGMLBackendBufferType(ret)
}

const ffnExprsRegex = `\\.ffn_(up|down|gate)_(ch|)exps`

// MoEExpertTensorPattern is the canonical regex matching routed expert tensors.
// It matches ffn_(up|down|gate)_exps and ffn_(up|down|gate)_chexps tensor names.
const MoEExpertTensorPattern = ffnExprsRegex

func ffnExprBlockRegex(index int) string {
	return fmt.Sprintf("blk\\.%d%s", index, ffnExprsRegex)
}

// NewTensorBuftBlockOverride creates a TensorBuftOverride for a specific FFN block index to execute in the CPU.
func NewTensorBuftBlockOverride(index int) TensorBuftOverride {
	return NewTensorBuftOverride(ffnExprBlockRegex(index))
}

// NewTensorBuftAllFFNExprsOverride creates a TensorBuftOverride for all FFN expression tensors to execute in the CPU.
func NewTensorBuftAllFFNExprsOverride() TensorBuftOverride {
	return NewTensorBuftOverride(ffnExprsRegex)
}

// NewTensorBuftOverride creates a TensorBuftOverride for a custom pattern to execute in the CPU.
func NewTensorBuftOverride(pattern string) TensorBuftOverride {
	data, err := utils.BytePtrFromString(pattern)
	if err != nil {
		return TensorBuftOverride{}
	}
	return TensorBuftOverride{
		Pattern: data,
		Type:    GGMLBackendCpuBufferType(),
	}
}

// GGMLBackendDeviceName returns the name of the given backend device.
func GGMLBackendDeviceName(device GGMLBackendDevice) string {
	var ret *byte
	ggmlBackendDevNameFunc.Call(unsafe.Pointer(&ret), unsafe.Pointer(&device))

	name := utils.BytePtrToString(ret)
	return name
}
