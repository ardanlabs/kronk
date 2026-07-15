package whisper

import (
	"unsafe"

	"github.com/ardanlabs/bucky/pkg/utils"
	"github.com/jupiterrider/ffi"
)

var (
	// WHISPER_API int          whisper_bench_memcpy          (int n_threads);
	benchMemcpyFunc ffi.Fun

	// WHISPER_API const char * whisper_bench_memcpy_str      (int n_threads);
	benchMemcpyStrFunc ffi.Fun

	// WHISPER_API int          whisper_bench_ggml_mul_mat    (int n_threads);
	benchGGMLMulMatFunc ffi.Fun

	// WHISPER_API const char * whisper_bench_ggml_mul_mat_str(int n_threads);
	benchGGMLMulMatStrFunc ffi.Fun
)

func loadBenchFuncs(lib ffi.Lib) error {
	var err error

	if benchMemcpyFunc, err = lib.Prep("whisper_bench_memcpy", &ffi.TypeSint32, &ffi.TypeSint32); err != nil {
		return loadError("whisper_bench_memcpy", err)
	}

	if benchMemcpyStrFunc, err = lib.Prep("whisper_bench_memcpy_str", &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("whisper_bench_memcpy_str", err)
	}

	if benchGGMLMulMatFunc, err = lib.Prep("whisper_bench_ggml_mul_mat", &ffi.TypeSint32, &ffi.TypeSint32); err != nil {
		return loadError("whisper_bench_ggml_mul_mat", err)
	}

	if benchGGMLMulMatStrFunc, err = lib.Prep("whisper_bench_ggml_mul_mat_str", &ffi.TypePointer, &ffi.TypeSint32); err != nil {
		return loadError("whisper_bench_ggml_mul_mat_str", err)
	}

	return nil
}

// BenchMemcpy runs the upstream memcpy micro-benchmark with the given thread
// count. Returns the C return code (0 on success).
func BenchMemcpy(nThreads int32) int32 {
	var result ffi.Arg
	benchMemcpyFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&nThreads))
	return int32(result)
}

// BenchMemcpyStr runs the memcpy benchmark and returns a human-readable
// summary string produced by whisper.cpp.
func BenchMemcpyStr(nThreads int32) string {
	var ptr *byte
	benchMemcpyStrFunc.Call(unsafe.Pointer(&ptr), unsafe.Pointer(&nThreads))
	if ptr == nil {
		return ""
	}
	return utils.BytePtrToString(ptr)
}

// BenchGGMLMulMat runs the upstream ggml_mul_mat micro-benchmark with the
// given thread count. Returns the C return code (0 on success).
func BenchGGMLMulMat(nThreads int32) int32 {
	var result ffi.Arg
	benchGGMLMulMatFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&nThreads))
	return int32(result)
}

// BenchGGMLMulMatStr runs the ggml_mul_mat benchmark and returns a
// human-readable summary string produced by whisper.cpp.
func BenchGGMLMulMatStr(nThreads int32) string {
	var ptr *byte
	benchGGMLMulMatStrFunc.Call(unsafe.Pointer(&ptr), unsafe.Pointer(&nThreads))
	if ptr == nil {
		return ""
	}
	return utils.BytePtrToString(ptr)
}
