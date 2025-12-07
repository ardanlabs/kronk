package tools

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/hashicorp/go-getter"
)

// SizeInterval are pre-calculated size interval values.
const (
	SizeIntervalMIB    = 1024 * 1024
	SizeIntervalMIB10  = SizeIntervalMIB * 10
	SizeIntervalMIB100 = SizeIntervalMIB * 100
)

// ProgressFunc provides feedback on the progress of a file download.
type ProgressFunc func(src string, currentSize int64, totalSize int64, mibPerSec float64, complete bool)

// DownloadFile pulls down a single file from a url to a specified destination.
func DownloadFile(ctx context.Context, url string, dest string, progress ProgressFunc, sizeInterval int64) (bool, error) {
	var pr ProgressReader

	if progress != nil {
		pr = ProgressReader{
			progress:     progress,
			sizeInterval: sizeInterval,
		}
	}

	client := getter.Client{
		Ctx:              ctx,
		Src:              url,
		Dst:              dest,
		Mode:             getter.ClientModeAny,
		ProgressListener: getter.ProgressTracker(&pr),
	}

	if err := client.Get(); err != nil {
		return false, fmt.Errorf("failed to download model: %w", err)
	}

	if pr.currentSize == 0 {
		return false, nil
	}

	return true, nil
}

// =============================================================================

// ProgressReader returns details about the download.
type ProgressReader struct {
	src          string
	currentSize  int64
	totalSize    int64
	lastReported int64
	startTime    time.Time
	reader       io.ReadCloser
	progress     ProgressFunc
	sizeInterval int64
}

// NewProgressReader constructs a progress reader for use.
func NewProgressReader(progress ProgressFunc, sizeInterval int64) *ProgressReader {
	return &ProgressReader{
		progress:     progress,
		sizeInterval: sizeInterval,
	}
}

// TrackProgress is called once at the beginning to setup the download.
func (pr *ProgressReader) TrackProgress(src string, currentSize, totalSize int64, stream io.ReadCloser) io.ReadCloser {
	pr.src = src
	pr.currentSize = currentSize
	pr.totalSize = totalSize
	pr.startTime = time.Now()
	pr.reader = stream

	return pr
}

// Read performs a partical read of the download which gives us the
// ability to get stats.
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.currentSize += int64(n)

	if pr.progress != nil && pr.currentSize-pr.lastReported >= pr.sizeInterval {
		pr.lastReported = pr.currentSize
		pr.progress(pr.src, pr.currentSize, pr.totalSize, pr.mibPerSec(), false)
	}

	return n, err
}

// Close closes the reader once the download is complete.
func (pr *ProgressReader) Close() error {
	if pr.progress != nil {
		pr.progress(pr.src, pr.currentSize, pr.totalSize, pr.mibPerSec(), true)
	}

	return pr.reader.Close()
}

// =============================================================================

func (pr *ProgressReader) mibPerSec() float64 {
	elapsed := time.Since(pr.startTime).Seconds()
	if elapsed == 0 {
		return 0
	}

	return float64(pr.currentSize) / SizeIntervalMIB / elapsed
}
