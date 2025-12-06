package tools

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/hashicorp/go-getter"
)

// ProgressFunc provides feedback on the progress of a file download.
type ProgressFunc func(src string, currentSize int64, totalSize int64, mibPerSec float64, complete bool)

// DownloadFile pulls down a single file from a url to a specified destination.
func DownloadFile(ctx context.Context, url string, dest string, progress ProgressFunc) (bool, error) {
	var pr ProgressReader

	if progress != nil {
		pr = ProgressReader{
			Dst:      dest,
			Progress: progress,
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

	if pr.CurrentSize == 0 {
		return false, nil
	}

	return true, nil
}

// =============================================================================

// ProgressReader returns details about the download.
type ProgressReader struct {
	Src          string
	Dst          string
	CurrentSize  int64
	TotalSize    int64
	LastReported int64
	StartTime    time.Time
	Reader       io.ReadCloser
	Progress     ProgressFunc
}

// TrackProgress is called once at the beginning to setup the download.
func (pr *ProgressReader) TrackProgress(src string, currentSize, totalSize int64, stream io.ReadCloser) io.ReadCloser {
	if currentSize == totalSize {
		return nil
	}

	if currentSize != totalSize {
		os.Remove(pr.Dst)
	}

	pr.Src = src
	pr.CurrentSize = currentSize
	pr.TotalSize = totalSize
	pr.StartTime = time.Now()
	pr.Reader = stream

	return pr
}

const (
	mib    = 1024 * 1024
	mib100 = mib * 100
)

// Read performs a partical read of the download which gives us the
// ability to get stats.
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	pr.CurrentSize += int64(n)

	if pr.Progress != nil && pr.CurrentSize-pr.LastReported >= mib100 {
		pr.LastReported = pr.CurrentSize
		pr.Progress(pr.Src, pr.CurrentSize, pr.TotalSize, pr.mibPerSec(), false)
	}

	return n, err
}

// Close closes the reader once the download is complete.
func (pr *ProgressReader) Close() error {
	if pr.Progress != nil {
		pr.Progress(pr.Src, pr.CurrentSize, pr.TotalSize, pr.mibPerSec(), true)
	}

	return pr.Reader.Close()
}

// =============================================================================

func (pr *ProgressReader) mibPerSec() float64 {
	elapsed := time.Since(pr.StartTime).Seconds()
	if elapsed == 0 {
		return 0
	}

	return float64(pr.CurrentSize) / mib / elapsed
}
