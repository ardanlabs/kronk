package llama

import (
	"errors"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/utils"
	"github.com/jupiterrider/ffi"
)

type (
	GGMLBackendDeviceType int32
	GGMLBackendDevice     uintptr
	GGMLBackendReg        uintptr
	GGMLType              int32
)

const (
	// CPU device using system memory
	GGMLBackendDeviceTypeCPU GGMLBackendDeviceType = iota
	// GPU device using dedicated memory
	GGMLBackendDeviceTypeGPU
	// integrated GPU device using host memory
	GGMLBackendDeviceTypeIGPU
	// accelerator devices intended to be used together with the CPU backend (e.g. BLAS or AMX)
	GGMLBackendDeviceTypeACCEL
)

const (
	// GGML_TYPE_F32
	GGMLTypeF32 GGMLType = 0
	// GGML_TYPE_F16
	GGMLTypeF16 GGMLType = 1
	// GGML_TYPE_Q4_0
	GGMLTypeQ4_0 GGMLType = 2
	// GGML_TYPE_Q4_1
	GGMLTypeQ4_1 GGMLType = 3
	// GGML_TYPE_Q4_2 = 4, support has been removed
	// GGML_TYPE_Q4_3 = 5, support has been removed
	// GGML_TYPE_Q5_0
	GGMLTypeQ5_0 GGMLType = 6
	// GGML_TYPE_Q5_1
	GGMLTypeQ5_1 GGMLType = 7
	// GGML_TYPE_Q8_0
	GGMLTypeQ8_0 GGMLType = 8
	// GGML_TYPE_Q8_1
	GGMLTypeQ8_1 GGMLType = 9
	// GGML_TYPE_Q2_K
	GGMLTypeQ2_K GGMLType = 10
	// GGML_TYPE_Q3_K
	GGMLTypeQ3_K GGMLType = 11
	// GGML_TYPE_Q4_K
	GGMLTypeQ4_K GGMLType = 12
	// GGML_TYPE_Q5_K
	GGMLTypeQ5_K GGMLType = 13
	// GGML_TYPE_Q6_K
	GGMLTypeQ6_K GGMLType = 14
	// GGML_TYPE_Q8_K
	GGMLTypeQ8_K GGMLType = 15
	// GGML_TYPE_IQ2_XXS
	GGMLTypeIQ2_XXS GGMLType = 16
	// GGML_TYPE_IQ2_XS
	GGMLTypeIQ2_XS GGMLType = 17
	// GGML_TYPE_IQ3_XXS
	GGMLTypeIQ3_XXS GGMLType = 18
	// GGML_TYPE_IQ1_S
	GGMLTypeIQ1_S GGMLType = 19
	// GGML_TYPE_IQ4_NL
	GGMLTypeIQ4_NL GGMLType = 20
	// GGML_TYPE_IQ3_S
	GGMLTypeIQ3_S GGMLType = 21
	// GGML_TYPE_IQ2_S
	GGMLTypeIQ2_S GGMLType = 22
	// GGML_TYPE_IQ4_XS
	GGMLTypeIQ4_XS GGMLType = 23
	// GGML_TYPE_I8
	GGMLTypeI8 GGMLType = 24
	// GGML_TYPE_I16
	GGMLTypeI16 GGMLType = 25
	// GGML_TYPE_I32
	GGMLTypeI32 GGMLType = 26
	// GGML_TYPE_I64
	GGMLTypeI64 GGMLType = 27
	// GGML_TYPE_F64
	GGMLTypeF64 GGMLType = 28
	// GGML_TYPE_IQ1_M
	GGMLTypeIQ1_M GGMLType = 29
	// GGML_TYPE_BF16
	GGMLTypeBF16 GGMLType = 30
	// GGML_TYPE_Q4_0_4_4 = 31, support has been removed from gguf files
	// GGML_TYPE_Q4_0_4_8 = 32,
	// GGML_TYPE_Q4_0_8_8 = 33,
	// GGML_TYPE_TQ1_0
	GGMLTypeTQ1_0 GGMLType = 34
	// GGML_TYPE_TQ2_0
	GGMLTypeTQ2_0 GGMLType = 35
	// GGML_TYPE_IQ4_NL_4_4 = 36,
	// GGML_TYPE_IQ4_NL_4_8 = 37,
	// GGML_TYPE_IQ4_NL_8_8 = 38,
	// GGML_TYPE_MXFP4
	GGMLTypeMXFP4 GGMLType = 39
	// GGML_TYPE_COUNT
	GGMLTypeCOUNT GGMLType = 40
)

var (
	// GGML_API void ggml_backend_load_all(void);
	ggmlBackendLoadAllFunc ffi.Fun

	// GGML_API void ggml_backend_load_all(void);
	ggmlBackendLoadAllFromPath ffi.Fun

	// Unload a backend if loaded dynamically and unregister it
	// GGML_API void               ggml_backend_unload(ggml_backend_reg_t reg);
	ggmlBackendUnloadFunc ffi.Fun

	// Device enumeration
	// GGML_API size_t             ggml_backend_dev_count(void);
	ggmlBackendDevCountFunc ffi.Fun

	// GGML_API ggml_backend_dev_t ggml_backend_dev_get(size_t index);
	ggmlBackendDevGetFunc ffi.Fun

	// GGML_API ggml_backend_dev_t ggml_backend_dev_by_name(const char * name);
	ggmlBackendDevByNameFunc ffi.Fun

	// GGML_API ggml_backend_dev_t ggml_backend_dev_by_type(enum ggml_backend_dev_type type);
	ggmlBackendDevByTypeFunc ffi.Fun

	// GGML_API size_t             ggml_backend_reg_count(void);
	ggmlBackendRegCountFunc ffi.Fun

	// GGML_API ggml_backend_reg_t ggml_backend_reg_get(size_t index);
	ggmlBackendRegGetFunc ffi.Fun

	// GGML_API ggml_backend_reg_t ggml_backend_reg_by_name(const char * name);
	ggmlBackendRegByNameFunc ffi.Fun
)

func loadGGML(lib ffi.Lib) error {
	var err error

	if ggmlBackendLoadAllFunc, err = lib.Prep("ggml_backend_load_all", &ffi.TypeVoid); err != nil {
		return loadError("ggml_backend_load_all", err)
	}

	if ggmlBackendLoadAllFromPath, err = lib.Prep("ggml_backend_load_all_from_path", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("ggml_backend_load_all_from_path", err)
	}

	if ggmlBackendUnloadFunc, err = lib.Prep("ggml_backend_unload", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("ggml_backend_unload", err)
	}

	if ggmlBackendDevCountFunc, err = lib.Prep("ggml_backend_dev_count", &ffi.TypeUint64); err != nil {
		return loadError("ggml_backend_dev_count", err)
	}

	if ggmlBackendDevGetFunc, err = lib.Prep("ggml_backend_dev_get", &ffi.TypePointer, &ffi.TypeUint64); err != nil {
		return loadError("ggml_backend_dev_get", err)
	}

	if ggmlBackendDevByNameFunc, err = lib.Prep("ggml_backend_dev_by_name", &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("ggml_backend_dev_by_name", err)
	}

	if ggmlBackendDevByTypeFunc, err = lib.Prep("ggml_backend_dev_by_type", &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("ggml_backend_dev_by_type", err)
	}

	if ggmlBackendRegCountFunc, err = lib.Prep("ggml_backend_reg_count", &ffi.TypeUint64); err != nil {
		return loadError("ggml_backend_reg_count", err)
	}

	if ggmlBackendRegGetFunc, err = lib.Prep("ggml_backend_reg_get", &ffi.TypePointer, &ffi.TypeUint64); err != nil {
		return loadError("ggml_backend_reg_get", err)
	}

	if ggmlBackendRegByNameFunc, err = lib.Prep("ggml_backend_reg_by_name", &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("ggml_backend_reg_by_name", err)
	}

	return nil
}

// GGMLBackendLoadAll loads all backends using the default search paths.
func GGMLBackendLoadAll() {
	ggmlBackendLoadAllFunc.Call(nil)
}

// GGMLBackendLoadAllFromPath loads all backends from a specific path.
func GGMLBackendLoadAllFromPath(path string) error {
	if path == "" {
		return errors.New("invalid path")
	}

	p := &[]byte(path + "\x00")[0]
	ggmlBackendLoadAllFromPath.Call(nil, unsafe.Pointer(&p))

	return nil
}

// GGMLBackendUnload unloads a backend if loaded dynamically and unregisters it.
func GGMLBackendUnload(reg GGMLBackendReg) {
	if reg == 0 {
		return
	}

	ggmlBackendUnloadFunc.Call(nil, unsafe.Pointer(&reg))
}

// GGMLBackendDeviceCount returns the number of backend devices.
func GGMLBackendDeviceCount() uint64 {
	var ret ffi.Arg
	ggmlBackendDevCountFunc.Call(unsafe.Pointer(&ret))
	return uint64(ret)
}

// GGMLBackendDeviceGet returns the backend device at the given index.
func GGMLBackendDeviceGet(index uint64) GGMLBackendDevice {
	var ret GGMLBackendDevice
	ggmlBackendDevGetFunc.Call(unsafe.Pointer(&ret), &index)
	return ret
}

// GGMLBackendDeviceByName returns the backend device by its name.
func GGMLBackendDeviceByName(name string) GGMLBackendDevice {
	namePtr, _ := utils.BytePtrFromString(name)
	var ret GGMLBackendDevice
	ggmlBackendDevByNameFunc.Call(unsafe.Pointer(&ret), unsafe.Pointer(&namePtr))
	return ret
}

// GGMLBackendDeviceByType returns the backend device by its type.
func GGMLBackendDeviceByType(devType GGMLBackendDeviceType) GGMLBackendDevice {
	var ret GGMLBackendDevice
	ggmlBackendDevByTypeFunc.Call(unsafe.Pointer(&ret), unsafe.Pointer(&devType))
	return ret
}

// GGMLBackendRegCount returns the number of backend registrations.
func GGMLBackendRegCount() uint64 {
	var ret ffi.Arg
	ggmlBackendRegCountFunc.Call(unsafe.Pointer(&ret))
	return uint64(ret)
}

// GGMLBackendRegGet returns the backend registration at the given index.
func GGMLBackendRegGet(index uint64) GGMLBackendReg {
	var ret GGMLBackendReg
	ggmlBackendRegGetFunc.Call(unsafe.Pointer(&ret), &index)
	return ret
}

// GGMLBackendRegByName returns the backend registration by its name.
func GGMLBackendRegByName(name string) GGMLBackendReg {
	namePtr, _ := utils.BytePtrFromString(name)
	var ret GGMLBackendReg
	ggmlBackendRegByNameFunc.Call(unsafe.Pointer(&ret), unsafe.Pointer(&namePtr))
	return ret
}

// GGMLBackendDeviceMemory returns the free and total memory (in bytes) for the given device.
func GGMLBackendDeviceMemory(device GGMLBackendDevice) (free uint64, total uint64) {
	if device == 0 {
		return 0, 0
	}

	freePtr := unsafe.Pointer(&free)
	totalPtr := unsafe.Pointer(&total)
	ggmlBackendDevMemoryFunc.Call(nil, unsafe.Pointer(&device), unsafe.Pointer(&freePtr), unsafe.Pointer(&totalPtr))
	return free, total
}
