package model

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"runtime"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/mtmd"

	// Image decoders registered with image.Decode. Images are decoded in
	// Go (rather than via the unstable mtmd-helper) so we only depend on
	// the stable mtmd_bitmap_init core API.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"
)

// newMediaBitmap converts a single raw media payload (encoded image or audio
// bytes) into an mtmd bitmap. The caller owns the returned bitmap and must
// free it with mtmd.BitmapFree.
//
// Images are decoded in Go to packed RGB24 and handed to the stable
// mtmd_bitmap_init(nx, ny, data) core API, avoiding the mtmd-helper bitmap
// functions whose signature is explicitly unstable upstream (see
// mtmd-helper.h: "these helpers are not guaranteed to be stable").
//
// Audio still flows through the corrected mtmd-helper binding
// (BitmapInitFromBuf) because decoding compressed audio (WAV/MP3/OGG/FLAC)
// to PCM F32 in Go is not yet wired up.
func newMediaBitmap(ctx mtmd.Context, med []byte) (mtmd.Bitmap, error) {
	switch mediaTypeFromMagicBytes(med) {
	case MediaTypeVision:
		return newImageBitmap(med)

	case MediaTypeAudio:
		bmp := BitmapInitFromBuf(ctx, &med[0], uint64(len(med)))
		if bmp == 0 {
			return 0, fmt.Errorf("mtmd could not decode audio payload")
		}
		return bmp, nil

	default:
		return 0, fmt.Errorf("media payload does not match any supported image or audio format")
	}
}

// newImageBitmap decodes an encoded image (JPEG/PNG/GIF/WEBP) into packed
// RGB24 pixels (RGBRGB..., no alpha, no row padding) and builds an mtmd
// bitmap via the stable mtmd_bitmap_init core API.
//
// mtmd_bitmap_init copies the pixel buffer, so the Go slice can be released
// after the call returns.
func newImageBitmap(med []byte) (mtmd.Bitmap, error) {
	img, _, err := image.Decode(bytes.NewReader(med))
	if err != nil {
		return 0, fmt.Errorf("decode image: %w", err)
	}

	b := img.Bounds()
	nx := b.Dx()
	ny := b.Dy()
	if nx <= 0 || ny <= 0 {
		return 0, fmt.Errorf("invalid image dimensions: %dx%d", nx, ny)
	}

	// Draw into an NRGBA canvas to get straight-alpha pixels in a known
	// layout, then strip the alpha channel down to RGB24.
	canvas := image.NewNRGBA(image.Rect(0, 0, nx, ny))
	draw.Draw(canvas, canvas.Bounds(), img, b.Min, draw.Src)

	rgb := make([]byte, nx*ny*3)
	for y := range ny {
		src := canvas.Pix[y*canvas.Stride:]
		dst := rgb[y*nx*3:]
		for x := range nx {
			dst[x*3+0] = src[x*4+0]
			dst[x*3+1] = src[x*4+1]
			dst[x*3+2] = src[x*4+2]
		}
	}

	bmp := mtmd.BitmapInit(uint32(nx), uint32(ny), uintptr(unsafe.Pointer(&rgb[0])))
	runtime.KeepAlive(rgb)
	if bmp == 0 {
		return 0, fmt.Errorf("mtmd_bitmap_init returned 0")
	}

	return bmp, nil
}
