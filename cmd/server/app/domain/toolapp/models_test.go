package toolapp

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func TestToPullProgress(t *testing.T) {
	progress := models.DownloadProgress{
		Src:          "model.gguf",
		CurrentBytes: 1_234_567,
		TotalBytes:   9_876_543,
		MBPerSec:     12.34,
		Complete:     true,
	}

	got := toPullProgress(progress)

	wantStatus := "download-model: Downloading model.gguf... 1 MB of 9 MB (12.34 MB/s)"
	if got.Status != wantStatus {
		t.Errorf("Status: got %q, want %q", got.Status, wantStatus)
	}
	if got.Progress == nil {
		t.Fatal("Progress: got nil, want structured progress")
	}
	if got.Progress.Src != progress.Src {
		t.Errorf("Src: got %q, want %q", got.Progress.Src, progress.Src)
	}
	if got.Progress.CurrentBytes != progress.CurrentBytes {
		t.Errorf("CurrentBytes: got %d, want %d", got.Progress.CurrentBytes, progress.CurrentBytes)
	}
	if got.Progress.TotalBytes != progress.TotalBytes {
		t.Errorf("TotalBytes: got %d, want %d", got.Progress.TotalBytes, progress.TotalBytes)
	}
	if got.Progress.MBPerSec != progress.MBPerSec {
		t.Errorf("MBPerSec: got %f, want %f", got.Progress.MBPerSec, progress.MBPerSec)
	}
	if got.Progress.Complete != progress.Complete {
		t.Errorf("Complete: got %t, want %t", got.Progress.Complete, progress.Complete)
	}
}
