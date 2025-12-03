package install

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/hashicorp/go-getter"
)

type ProgressFunc func(src string, currentSize int64, totalSize int64, mibPerSec float64)

func pullFile(ctx context.Context, url string, dest string, progress ProgressFunc) error {
	var pl getter.ProgressTracker
	if progress != nil {
		pl = &progressReader{
			dst:      dest,
			progress: progress,
		}
	}

	client := getter.Client{
		Ctx:              ctx,
		Src:              url,
		Dst:              dest,
		Mode:             getter.ClientModeAny,
		ProgressListener: pl,
	}

	if err := client.Get(); err != nil {
		return fmt.Errorf("failed to download model: %w", err)
	}

	return nil
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
	pr.src = src
	pr.currentSize = currentSize
	pr.totalSize = totalSize
	pr.startTime = time.Now()
	pr.reader = stream

	if pr.currentSize == pr.totalSize {
		return nil
	}

	if pr.currentSize != pr.totalSize {
		os.Remove(pr.dst)
	}

	return pr
}

const mib = 1024 * 1024 * 100

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.currentSize += int64(n)

	if pr.progress != nil && pr.currentSize-pr.lastReported >= mib {
		pr.lastReported = pr.currentSize
		pr.progress(pr.src, pr.currentSize, pr.totalSize, pr.mibPerSec())
	}

	return n, err
}

func (pr *progressReader) Close() error {
	if pr.progress != nil {
		pr.progress(pr.src, pr.currentSize, pr.totalSize, pr.mibPerSec())
	}

	return pr.reader.Close()
}

func (pr *progressReader) mibPerSec() float64 {
	elapsed := time.Since(pr.startTime).Seconds()
	if elapsed == 0 {
		return 0
	}

	return float64(pr.currentSize) / (1024 * 1024) / elapsed
}
