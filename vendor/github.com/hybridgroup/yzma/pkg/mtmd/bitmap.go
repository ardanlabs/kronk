package mtmd

import (
	"os"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/utils"
	"github.com/jupiterrider/ffi"
)

// Opaque types (represented as pointers)
type Bitmap uintptr

var (
	// MTMD_API mtmd_bitmap *         mtmd_bitmap_init           (uint32_t nx, uint32_t ny, const unsigned char * data);
	bitmapInitFunc ffi.Fun

	// MTMD_API void                  mtmd_bitmap_free       (mtmd_bitmap * bitmap);
	bitmapFreeFunc ffi.Fun

	// MTMD_API size_t                mtmd_bitmap_get_n_bytes(const mtmd_bitmap * bitmap);
	bitmapGetNBytesFunc ffi.Fun

	// MTMD_API mtmd_bitmap * mtmd_helper_bitmap_init_from_file(mtmd_context * ctx, const char * fname);
	bitmapInitFromFileFunc ffi.Fun

	// MTMD_API mtmd_bitmap * mtmd_helper_bitmap_init_from_buf(mtmd_context * ctx, const unsigned char * buf, size_t len);
	bitmapInitFromBufFunc ffi.Fun

	// MTMD_API uint32_t mtmd_bitmap_get_nx(const mtmd_bitmap * bitmap);
	bitmapGetNxFunc ffi.Fun

	// MTMD_API uint32_t mtmd_bitmap_get_ny(const mtmd_bitmap * bitmap);
	bitmapGetNyFunc ffi.Fun

	// MTMD_API const unsigned char * mtmd_bitmap_get_data(const mtmd_bitmap * bitmap);
	bitmapGetDataFunc ffi.Fun

	// MTMD_API bool mtmd_bitmap_is_audio(const mtmd_bitmap * bitmap);
	bitmapIsAudioFunc ffi.Fun

	// MTMD_API const char * mtmd_bitmap_get_id(const mtmd_bitmap * bitmap);
	bitmapGetIdFunc ffi.Fun

	// MTMD_API void mtmd_bitmap_set_id(mtmd_bitmap * bitmap, const char * id);
	bitmapSetIdFunc ffi.Fun

	// if bitmap is audio:
	//     length of data must be n_samples * sizeof(float)
	//     the data is in float format (PCM F32)
	// MTMD_API mtmd_bitmap * mtmd_bitmap_init_from_audio(size_t n_samples, const float * data);
	bitmapInitFromAudioFunc ffi.Fun
)

func loadBitmapFuncs(lib ffi.Lib) error {
	var err error

	if bitmapInitFunc, err = lib.Prep("mtmd_bitmap_init", &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32, &ffi.TypePointer); err != nil {
		return loadError("mtmd_bitmap_init", err)
	}

	if bitmapFreeFunc, err = lib.Prep("mtmd_bitmap_free", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("mtmd_bitmap_free", err)
	}

	if bitmapGetNBytesFunc, err = lib.Prep("mtmd_bitmap_get_n_bytes", &ffiTypeSize, &ffi.TypePointer); err != nil {
		return loadError("mtmd_bitmap_get_n_bytes", err)
	}

	if bitmapInitFromFileFunc, err = lib.Prep("mtmd_helper_bitmap_init_from_file", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("mtmd_helper_bitmap_init_from_file", err)
	}

	if bitmapInitFromBufFunc, err = lib.Prep("mtmd_helper_bitmap_init_from_buf", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer, &ffiTypeSize); err != nil {
		return loadError("mtmd_helper_bitmap_init_from_buf", err)
	}

	if bitmapGetNxFunc, err = lib.Prep("mtmd_bitmap_get_nx", &ffi.TypeUint32, &ffi.TypePointer); err != nil {
		return loadError("mtmd_bitmap_get_nx", err)
	}

	if bitmapGetNyFunc, err = lib.Prep("mtmd_bitmap_get_ny", &ffi.TypeUint32, &ffi.TypePointer); err != nil {
		return loadError("mtmd_bitmap_get_ny", err)
	}

	if bitmapGetDataFunc, err = lib.Prep("mtmd_bitmap_get_data", &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("mtmd_bitmap_get_data", err)
	}

	if bitmapIsAudioFunc, err = lib.Prep("mtmd_bitmap_is_audio", &ffi.TypeUint8, &ffi.TypePointer); err != nil {
		return loadError("mtmd_bitmap_is_audio", err)
	}

	if bitmapGetIdFunc, err = lib.Prep("mtmd_bitmap_get_id", &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("mtmd_bitmap_get_id", err)
	}

	if bitmapSetIdFunc, err = lib.Prep("mtmd_bitmap_set_id", &ffi.TypeVoid, &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("mtmd_bitmap_set_id", err)
	}

	if bitmapInitFromAudioFunc, err = lib.Prep("mtmd_bitmap_init_from_audio", &ffi.TypePointer, &ffi.TypeUint64, &ffi.TypePointer); err != nil {
		return loadError("mtmd_bitmap_init_from_audio", err)
	}

	return nil
}

// BitmapInit initializes a Bitmap.
func BitmapInit(nx uint32, ny uint32, data uintptr) Bitmap {
	var bitmap Bitmap
	bitmapInitFunc.Call(unsafe.Pointer(&bitmap), &nx, &ny, unsafe.Pointer(&data))

	return bitmap
}

// BitmapFree frees a previous initialized Bitmap.
func BitmapFree(bitmap Bitmap) {
	if bitmap == 0 {
		return
	}
	bitmapFreeFunc.Call(nil, unsafe.Pointer(&bitmap))
}

// BitmapGetNBytes returns the number of bytes in the Bitmap.
func BitmapGetNBytes(bitmap Bitmap) uint64 {
	if bitmap == 0 {
		return 0
	}
	var result ffi.Arg
	bitmapGetNBytesFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&bitmap))

	return uint64(result)
}

// BitmapInitFromFile initializes a Bitmap from a file.
func BitmapInitFromFile(ctx Context, fname string) Bitmap {
	var bitmap Bitmap
	if ctx == 0 {
		return bitmap
	}
	if _, err := os.Stat(fname); os.IsNotExist(err) {
		// no such file
		return bitmap
	}

	file := &[]byte(fname + "\x00")[0]
	bitmapInitFromFileFunc.Call(unsafe.Pointer(&bitmap), unsafe.Pointer(&ctx), unsafe.Pointer(&file))

	return bitmap
}

// BitmapInitFromBuf initializes a Bitmap from a buffer of bytes.
func BitmapInitFromBuf(ctx Context, buf *byte, len uint64) Bitmap {
	var bitmap Bitmap
	if ctx == 0 {
		return bitmap
	}
	bitmapInitFromBufFunc.Call(unsafe.Pointer(&bitmap), unsafe.Pointer(&ctx), unsafe.Pointer(&buf), &len)

	return bitmap
}

// BitmapGetNx retrieves the width (nx) of the bitmap.
func BitmapGetNx(bitmap Bitmap) uint32 {
	if bitmap == 0 {
		return 0
	}
	var result ffi.Arg
	bitmapGetNxFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&bitmap))
	return uint32(result)
}

// BitmapGetNy retrieves the height (ny) of the bitmap.
func BitmapGetNy(bitmap Bitmap) uint32 {
	if bitmap == 0 {
		return 0
	}
	var result ffi.Arg
	bitmapGetNyFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&bitmap))
	return uint32(result)
}

// BitmapGetData retrieves the raw data of the bitmap.
func BitmapGetData(bitmap Bitmap) []byte {
	if bitmap == 0 {
		return nil
	}
	var dataPtr *byte
	bitmapGetDataFunc.Call(unsafe.Pointer(&dataPtr), unsafe.Pointer(&bitmap))

	if dataPtr == nil {
		return nil
	}

	nx := BitmapGetNx(bitmap)
	ny := BitmapGetNy(bitmap)
	size := nx * ny * 3
	return unsafe.Slice((*byte)(dataPtr), size)
}

// BitmapIsAudio checks if the bitmap represents audio data.
func BitmapIsAudio(bitmap Bitmap) bool {
	if bitmap == 0 {
		return false
	}
	var result ffi.Arg
	bitmapIsAudioFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&bitmap))
	return result.Bool()
}

// BitmapGetId retrieves the ID of the bitmap.
func BitmapGetId(bitmap Bitmap) string {
	if bitmap == 0 {
		return ""
	}
	var idPtr *byte
	bitmapGetIdFunc.Call(unsafe.Pointer(&idPtr), unsafe.Pointer(&bitmap))

	if idPtr == nil {
		return ""
	}

	return utils.BytePtrToString(idPtr)
}

// BitmapSetId sets the ID of the bitmap.
func BitmapSetId(bitmap Bitmap, id string) {
	if bitmap == 0 {
		return
	}
	idPtr, _ := utils.BytePtrFromString(id)
	bitmapSetIdFunc.Call(nil, unsafe.Pointer(&bitmap), unsafe.Pointer(&idPtr))
}

// BitmapInitFromAudio initializes a Bitmap from audio data (PCM F32).
func BitmapInitFromAudio(nSamples uint64, data *float32) Bitmap {
	var bitmap Bitmap
	if data == nil {
		return bitmap
	}
	bitmapInitFromAudioFunc.Call(unsafe.Pointer(&bitmap), &nSamples, unsafe.Pointer(&data))
	return bitmap
}
