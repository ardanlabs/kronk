package model

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"

	"github.com/icza/mjpeg"
)

// SaveAVI writes standard Go images as a Motion-JPEG AVI file.
func SaveAVI(filename string, frames []image.Image, fps int, quality int) (err error) {
	if len(frames) == 0 {
		return errors.New("save-avi: no frames")
	}
	if fps <= 0 || fps > 1_000_000 {
		return errors.New("save-avi: fps must be between 1 and 1000000")
	}
	if quality < 1 || quality > 100 {
		return errors.New("save-avi: quality must be between 1 and 100")
	}
	if frames[0] == nil || frames[0].Bounds().Dx() <= 0 || frames[0].Bounds().Dy() <= 0 {
		return errors.New("save-avi: frame 0 has invalid dimensions")
	}

	width := frames[0].Bounds().Dx()
	height := frames[0].Bounds().Dy()
	if width >= 1<<16 || height >= 1<<16 {
		return errors.New("save-avi: frame dimensions exceed JPEG limits")
	}
	const maxInt32 = int64(1<<31 - 1)
	width64 := int64(width)
	height64 := int64(height)
	if width64 > maxInt32 || height64 > maxInt32 || width64 > maxInt32/(height64*3) {
		return errors.New("save-avi: frame dimensions exceed AVI limits")
	}
	for i, frame := range frames {
		if frame == nil || frame.Bounds().Dx() != width || frame.Bounds().Dy() != height {
			return fmt.Errorf("save-avi: frame %d dimensions do not match %dx%d", i, width, height)
		}
	}

	writer, err := mjpeg.New(filename, int32(width), int32(height), int32(fps))
	if err != nil {
		return fmt.Errorf("save-avi: creating writer: %w", err)
	}
	defer func() {
		if closeErr := writer.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("save-avi: closing writer: %w", closeErr))
		}
	}()

	for i, frame := range frames {
		var encoded bytes.Buffer
		if err := jpeg.Encode(&encoded, frame, &jpeg.Options{Quality: quality}); err != nil {
			return fmt.Errorf("save-avi: encoding frame %d: %w", i, err)
		}
		if err := writer.AddFrame(encoded.Bytes()); err != nil {
			return fmt.Errorf("save-avi: adding frame %d: %w", i, err)
		}
	}

	return nil
}
