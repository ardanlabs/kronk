package pull

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
)

// A progress line is rewritten in place, so it has to fit the terminal or the
// wrapped remainder is left behind on screen. 80 columns is the width every
// terminal provides.
const (
	maxLineWidth = 80
	minSrcWidth  = 16
	linePrefix   = "Downloading "
)

const (
	mb = 1000 * 1000
	gb = 1000 * mb
)

// widestETA is the longest estimate that can be rendered. The line is laid out
// against it so the file name keeps its width for the whole download.
const widestETA = "99h59m"

// progressPrinter renders a model pull stream. Consecutive progress updates
// for the same file rewrite a single line; a different file starts a new one.
// Any other status message is written on its own line.
type progressPrinter struct {
	w       io.Writer
	rewrite bool
	src     string
	open    bool
}

// newProgressPrinter constructs a printer that writes to w. With rewrite set
// to false every update lands on its own line, since a carriage return means
// nothing once the output is redirected to a file or a pipe.
func newProgressPrinter(w io.Writer, rewrite bool) *progressPrinter {
	return &progressPrinter{
		w:       w,
		rewrite: rewrite,
	}
}

// print writes a single stream update.
func (p *progressPrinter) print(pr toolapp.PullResponse) {
	if pr.Progress == nil {
		p.close()

		if status := strings.TrimSpace(strings.TrimPrefix(pr.Status, "\r\x1b[K")); status != "" {
			fmt.Fprintln(p.w, status)
		}

		return
	}

	line := progressLine(*pr.Progress)

	if !p.rewrite {
		fmt.Fprintln(p.w, line)
		return
	}

	if pr.Progress.Src != p.src {
		p.close()
		p.src = pr.Progress.Src
	}

	fmt.Fprintf(p.w, "\r\x1b[K%s", line)
	p.open = true

	// The file is done, so keep the last line and move on to the next one.
	if pr.Progress.Complete {
		p.close()
	}
}

// close terminates a progress line still held open on screen.
func (p *progressPrinter) close() {
	if !p.open {
		return
	}

	fmt.Fprintln(p.w)
	p.open = false
}

// progressLine formats one progress update:
//
//	Downloading …8-27B-UD-Q4_K_M.gguf   45.2%   7.4/16.5 GB  29.85 MB/s  ETA 5m02s
func progressLine(pg toolapp.PullProgress) string {
	unit := unitFor(pg.TotalBytes)

	// The total stays unknown until the response header lands.
	if pg.TotalBytes <= 0 {
		unit = unitFor(pg.CurrentBytes)
		stats := fmt.Sprintf("%s %s  %6.2f MB/s", sizeText(pg.CurrentBytes, unit), unit, pg.MBPerSec)

		return layout(pg.Src, stats, stats)
	}

	total := sizeText(pg.TotalBytes, unit)

	// The size is padded to the width of the total and the rate to its widest
	// value, so the columns hold still while the numbers move.
	stats := fmt.Sprintf("%5.1f%%  %*s/%s %s  %6.2f MB/s",
		float64(pg.CurrentBytes)/float64(pg.TotalBytes)*100,
		len(total), sizeText(pg.CurrentBytes, unit), total, unit,
		pg.MBPerSec)

	// The estimate is the only variable width field and it sits last, where
	// the erase-to-end-of-line takes care of the leftovers.
	widest := fmt.Sprintf("%s  ETA %s", stats, widestETA)

	if left := eta(pg); left != "" {
		stats = fmt.Sprintf("%s  ETA %s", stats, left)
	}

	return layout(pg.Src, stats, widest)
}

// layout joins the file name and the stats into a line sized for the terminal.
// The name is measured against the widest form the stats can take so it does
// not change width from one update to the next.
func layout(src string, stats string, widest string) string {
	return fmt.Sprintf("%s%s  %s", linePrefix, srcName(src, maxLineWidth-len(linePrefix)-2-len(widest)), stats)
}

// eta estimates the time left from the transfer rate reported for this update.
func eta(pg toolapp.PullProgress) string {
	if pg.Complete || pg.MBPerSec <= 0 || pg.CurrentBytes >= pg.TotalBytes {
		return ""
	}

	secs := float64(pg.TotalBytes-pg.CurrentBytes) / (pg.MBPerSec * mb)

	d := time.Duration(secs * float64(time.Second))

	switch {
	case d >= 100*time.Hour:
		return widestETA
	case d >= time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d/time.Hour), int(d/time.Minute)%60)
	case d >= time.Minute:
		return fmt.Sprintf("%dm%02ds", int(d/time.Minute), int(d/time.Second)%60)
	default:
		return fmt.Sprintf("%ds", int(d/time.Second))
	}
}

// unitFor picks the unit that keeps a size readable.
func unitFor(bytes int64) string {
	if bytes >= gb {
		return "GB"
	}

	return "MB"
}

// sizeText renders bytes in the given unit, leaving the unit itself out so a
// pair of sizes can share one.
func sizeText(bytes int64, unit string) string {
	if unit == "GB" {
		return fmt.Sprintf("%.1f", float64(bytes)/gb)
	}

	return fmt.Sprintf("%d", bytes/mb)
}

// srcName shortens a file name from the front so the tail — where the
// quantization and the extension live — stays visible.
func srcName(src string, width int) string {
	if width < minSrcWidth {
		width = minSrcWidth
	}

	name := []rune(src)
	if len(name) <= width {
		return src
	}

	return "…" + string(name[len(name)-width+1:])
}

// isTerminal reports whether f is attached to a terminal, which is what makes
// an in-place line rewrite meaningful.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
