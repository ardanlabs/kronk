// Package downloader provide support for downloading files.
package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
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

// Download pulls down a single file from a url to a specified destination.
func Download(ctx context.Context, src string, dest string, progress ProgressFunc, sizeInterval int64) (bool, error) {
	if !hasNetwork() {
		return false, errors.New("download: no network available")
	}

	var pr ProgressReader

	if progress != nil {
		pr = ProgressReader{
			progress:     progress,
			sizeInterval: sizeInterval,
		}
	}

	var getters map[string]getter.Getter

	if os.Getenv("KRONK_HF_TOKEN") != "" {
		httpGetter := &getter.HttpGetter{
			Header: map[string][]string{
				"Authorization": {"Bearer " + os.Getenv("KRONK_HF_TOKEN")},
			},
		}

		getters = map[string]getter.Getter{
			"https": httpGetter,
			"http":  httpGetter,
		}
	}

	client := getter.Client{
		Ctx:              ctx,
		Src:              src,
		Dst:              dest,
		Mode:             getter.ClientModeAny,
		ProgressListener: getter.ProgressTracker(&pr),
		Getters:          getters,
	}

	if err := client.Get(); err != nil {
		return false, fmt.Errorf("download: failed to download model type[%T]: %w", err, err)
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

// Read performs a partial read of the download which gives us the
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

// =============================================================================

func hasNetwork() bool {
	conn, err := net.DialTimeout("tcp", "8.8.8.8:53", 3*time.Second)
	if err != nil {
		return false
	}

	conn.Close()

	return true
}
