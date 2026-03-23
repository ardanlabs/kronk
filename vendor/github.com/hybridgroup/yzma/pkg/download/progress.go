package download

import (
	"fmt"
	"io"
	"time"

	"github.com/hashicorp/go-getter"
)

// ProgressTracker shows download progress in the terminal.
// You can override it with your own implementation if needed.
// By default, it prints progress to stdout.
// See https://pkg.go.dev/github.com/hashicorp/go-getter#ProgressTracker
var ProgressTracker = DefaultProgressTracker()

// DefaultProgressTracker returns the default ProgressTracker that prints download progress to stdout.
func DefaultProgressTracker() getter.ProgressTracker {
	progFunc := func(src string, currentSize int64, totalSize int64, mibPerSec float64, complete bool) {
		fmt.Printf("\r\x1b[Kdownloading %s... %d MiB of %d MiB (%.2f MiB/s)", src, currentSize/(1024*1024), totalSize/(1024*1024), mibPerSec)
		if complete {
			fmt.Println()
		}
	}

	pr := progressReader{
		progress: progFunc,
	}

	return getter.ProgressTracker(&pr)
}

type progressFunc func(src string, currentSize int64, totalSize int64, mibPerSec float64, complete bool)

type progressReader struct {
	src          string
	currentSize  int64
	totalSize    int64
	lastReported int64
	startTime    time.Time
	reader       io.ReadCloser
	progress     progressFunc
}

func (pr *progressReader) TrackProgress(src string, currentSize, totalSize int64, stream io.ReadCloser) io.ReadCloser {
	if currentSize == totalSize {
		return nil
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
