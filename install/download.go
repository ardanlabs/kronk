package install

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/hashicorp/go-getter"
)

type ProgressFunc func(src string, currentSize int64, totalSize int64, mibPerSec float64, complete bool)

func pullFile(ctx context.Context, url string, dest string, progress ProgressFunc) (bool, error) {
	var pr progressReader

	if progress != nil {
		pr = progressReader{
			dst:      dest,
			progress: progress,
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

type progressReader struct {
	src          string
	dst          string
	currentSize  int64
	totalSize    int64
	lastReported int64
	startTime    time.Time
	reader       io.ReadCloser
	progress     ProgressFunc
}

func (pr *progressReader) TrackProgress(src string, currentSize, totalSize int64, stream io.ReadCloser) io.ReadCloser {
	if currentSize == totalSize {
		return nil
	}

	if currentSize != totalSize {
		os.Remove(pr.dst)
	}

	pr.src = src
	pr.currentSize = currentSize
	pr.totalSize = totalSize
	pr.startTime = time.Now()
	pr.reader = stream

	return pr
}

const (
	mib    = 1024 * 1024
	mib100 = mib * 100
)

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.currentSize += int64(n)

	if pr.progress != nil && pr.currentSize-pr.lastReported >= mib100 {
		pr.lastReported = pr.currentSize
		pr.progress(pr.src, pr.currentSize, pr.totalSize, pr.mibPerSec(), false)
	}

	return n, err
}

func (pr *progressReader) Close() error {
	if pr.progress != nil {
		pr.progress(pr.src, pr.currentSize, pr.totalSize, pr.mibPerSec(), true)
	}

	return pr.reader.Close()
}

func (pr *progressReader) mibPerSec() float64 {
	elapsed := time.Since(pr.startTime).Seconds()
	if elapsed == 0 {
		return 0
	}

	return float64(pr.currentSize) / mib / elapsed
}
