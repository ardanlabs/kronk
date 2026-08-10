package model

import (
	"image"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAVI(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 2, 2))
	filename := filepath.Join(t.TempDir(), "test.avi")

	if err := SaveAVI(filename, []image.Image{frame, frame}, 24, 90); err != nil {
		t.Fatalf("SaveAVI() error = %v", err)
	}

	info, err := os.Stat(filename)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}
	if info.Size() == 0 {
		t.Error("AVI size = 0, want data")
	}
}

func TestSaveAVIValidation(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "test.avi")
	tests := []struct {
		name    string
		frames  []image.Image
		fps     int
		quality int
	}{
		{name: "no frames", fps: 24, quality: 90},
		{name: "invalid fps", frames: []image.Image{image.NewRGBA(image.Rect(0, 0, 2, 2))}, quality: 90},
		{name: "overflow fps", frames: []image.Image{image.NewRGBA(image.Rect(0, 0, 2, 2))}, fps: 1_000_001, quality: 90},
		{name: "invalid quality", frames: []image.Image{image.NewRGBA(image.Rect(0, 0, 2, 2))}, fps: 24},
		{name: "JPEG dimensions", frames: []image.Image{image.NewRGBA(image.Rect(0, 0, 1<<16, 1))}, fps: 24, quality: 90},
		{name: "mismatched frames", frames: []image.Image{image.NewRGBA(image.Rect(0, 0, 2, 2)), image.NewRGBA(image.Rect(0, 0, 3, 2))}, fps: 24, quality: 90},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := SaveAVI(filename, tt.frames, tt.fps, tt.quality); err == nil {
				t.Fatal("SaveAVI() error = nil, want error")
			}
			if _, err := os.Stat(filename); !os.IsNotExist(err) {
				t.Errorf("validation created output: os.Stat() error = %v, want not exist", err)
				if removeErr := os.Remove(filename); removeErr != nil {
					t.Fatalf("os.Remove() error = %v", removeErr)
				}
			}
		})
	}
}
