// DO NOT DELETE THIS FILE when InitYzmaWorkarounds is empty.
// Pending upstream macOS MTP row symbol aliases:
// https://github.com/hybridgroup/yzma/issues/492

package model

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/loader"
	"github.com/jupiterrider/ffi"
)

// InitYzmaWorkarounds initializes Kronk-specific Yzma compatibility code.
// Call it during native initialization, before using these bindings.
func InitYzmaWorkarounds(path string) error {
	return initMTPDeviceBindings(path)
}

// The extension header permits C++ exports. Match yzma's optional NextN
// loading, including both int64_t manglings (long on Linux, long long on macOS).
// Yzma's device-row loader currently lacks the macOS xx aliases below.
var mtpDeviceBindings struct {
	available bool
	enable    ffi.Fun
	getRows   ffi.Fun
	setRows   ffi.Fun
	setSource ffi.Fun
	setRow    ffi.Fun
}

func initMTPDeviceBindings(path string) error {
	mtpDeviceBindings.available = false
	lib, err := loader.LoadLibrary(path, "llama")
	if err != nil {
		return fmt.Errorf("loading MTP compatibility library: %w", err)
	}
	functions := []struct {
		names  []string
		target *ffi.Fun
		args   []*ffi.Type
	}{
		{[]string{"llama_set_embeddings_nextn_dev", "_Z30llama_set_embeddings_nextn_devP13llama_contextb", "?llama_set_embeddings_nextn_dev@@YA_NPEAUllama_context@@_N@Z"}, &mtpDeviceBindings.enable, []*ffi.Type{&ffi.TypePointer, &ffi.TypeUint8}},
		{[]string{"llama_get_embeddings_nextn_rows", "_Z31llama_get_embeddings_nextn_rowsP13llama_contextllPf", "_Z31llama_get_embeddings_nextn_rowsP13llama_contextxxPf", "?llama_get_embeddings_nextn_rows@@YA_NPEAUllama_context@@_J1PEAM@Z"}, &mtpDeviceBindings.getRows, []*ffi.Type{&ffi.TypePointer, &ffi.TypeSint64, &ffi.TypeSint64, &ffi.TypePointer}},
		{[]string{"llama_set_embeddings_nextn_rows", "_Z31llama_set_embeddings_nextn_rowsP13llama_contextllPKf", "_Z31llama_set_embeddings_nextn_rowsP13llama_contextxxPKf", "?llama_set_embeddings_nextn_rows@@YA_NPEAUllama_context@@_J1PEBM@Z"}, &mtpDeviceBindings.setRows, []*ffi.Type{&ffi.TypePointer, &ffi.TypeSint64, &ffi.TypeSint64, &ffi.TypePointer}},
		{[]string{"llama_set_embd_state_src", "_Z24llama_set_embd_state_srcP13llama_contextS0_", "?llama_set_embd_state_src@@YA_NPEAUllama_context@@0@Z"}, &mtpDeviceBindings.setSource, []*ffi.Type{&ffi.TypePointer, &ffi.TypePointer}},
		{[]string{"llama_batch_ext_set_embd_state_row", "_Z34llama_batch_ext_set_embd_state_rowP15llama_batch_extii", "?llama_batch_ext_set_embd_state_row@@YA_NPEAUllama_batch_ext@@HH@Z"}, &mtpDeviceBindings.setRow, []*ffi.Type{&ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32}},
	}
	for _, function := range functions {
		*function.target = ffi.Fun{}
		for _, name := range function.names {
			fn, err := lib.Prep(name, &ffi.TypeUint8, function.args...)
			if err == nil {
				*function.target = fn
				break
			}
		}
		if function.target.Cif == nil {
			return nil // Older bundles retain host-based MTP.
		}
	}
	mtpDeviceBindings.available = true
	return nil
}

func setEmbeddingsNextNDevice(ctx llama.Context, enabled bool) bool {
	if !mtpDeviceBindings.available || ctx == 0 {
		return false
	}
	var result ffi.Arg
	mtpDeviceBindings.enable.Call(unsafe.Pointer(&result), &ctx, &enabled)
	return uint8(result) != 0
}

// Copies are synchronous. Never expose or retain a pointer into native storage.
func getEmbeddingsNextNRows(ctx llama.Context, row int64, count int64, out []float32, width int) bool {
	if !mtpDeviceBindings.available || ctx == 0 || row < 0 || count <= 0 || width <= 0 || count > int64(len(out)/width) {
		return false
	}
	p := unsafe.SliceData(out)
	var result ffi.Arg
	mtpDeviceBindings.getRows.Call(unsafe.Pointer(&result), &ctx, &row, &count, unsafe.Pointer(&p))
	runtime.KeepAlive(out)
	return uint8(result) != 0
}

func setEmbeddingsNextNRows(ctx llama.Context, row int64, count int64, data []float32, width int) bool {
	if !mtpDeviceBindings.available || ctx == 0 || row < 0 || count <= 0 || width <= 0 || count > int64(len(data)/width) {
		return false
	}
	p := unsafe.SliceData(data)
	var result ffi.Arg
	mtpDeviceBindings.setRows.Call(unsafe.Pointer(&result), &ctx, &row, &count, unsafe.Pointer(&p))
	runtime.KeepAlive(data)
	return uint8(result) != 0
}

// A zero source detaches successfully even though native returns false (there
// is no longer a supported source). Callers ignore that result on teardown.
func setMTPStateSource(ctx, source llama.Context) bool {
	if !mtpDeviceBindings.available || ctx == 0 {
		return false
	}
	var result ffi.Arg
	mtpDeviceBindings.setSource.Call(unsafe.Pointer(&result), &ctx, &source)
	return uint8(result) != 0
}

func setMTPStateRow(batch llama.BatchExt, index, row int32) bool {
	if !mtpDeviceBindings.available || batch == 0 || index < 0 || row < 0 {
		return false
	}
	var result ffi.Arg
	mtpDeviceBindings.setRow.Call(unsafe.Pointer(&result), &batch, &index, &row)
	return uint8(result) != 0
}
