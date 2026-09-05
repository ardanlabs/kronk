package pull

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
)

func TestProgressPrinterRewritesSameFile(t *testing.T) {
	var buf bytes.Buffer
	prt := newProgressPrinter(&buf, true)

	prt.print(toolapp.PullResponse{Status: "download-model: model-url[x] file[1/2]"})
	prt.print(toolapp.PullResponse{Progress: &toolapp.PullProgress{Src: "model.gguf", CurrentBytes: 100_000_000, TotalBytes: 400_000_000, MBPerSec: 25}})
	prt.print(toolapp.PullResponse{Progress: &toolapp.PullProgress{Src: "model.gguf", CurrentBytes: 200_000_000, TotalBytes: 400_000_000, MBPerSec: 25}})
	prt.print(toolapp.PullResponse{Progress: &toolapp.PullProgress{Src: "mmproj.gguf", CurrentBytes: 10_000_000, TotalBytes: 20_000_000, MBPerSec: 10}})
	prt.close()

	got := buf.String()

	if strings.Count(got, "\r\x1b[K") != 3 {
		t.Errorf("carriage returns: got %d, want 3\n%q", strings.Count(got, "\r\x1b[K"), got)
	}

	// One line per file: the model line closed when mmproj started, and the
	// mmproj line closed by the printer. The status message adds its own.
	if lines := strings.Count(got, "\n"); lines != 3 {
		t.Errorf("lines: got %d, want 3\n%q", lines, got)
	}

	if !strings.Contains(got, " 50.0%") {
		t.Errorf("percentage missing from %q", got)
	}
}

func TestProgressPrinterCompleteClosesLine(t *testing.T) {
	var buf bytes.Buffer
	prt := newProgressPrinter(&buf, true)

	prt.print(toolapp.PullResponse{Progress: &toolapp.PullProgress{Src: "model.gguf", CurrentBytes: 400_000_000, TotalBytes: 400_000_000, MBPerSec: 25, Complete: true}})

	if !strings.HasSuffix(buf.String(), "\n") {
		t.Errorf("completed file must close its line: %q", buf.String())
	}

	// The line is already closed, so the printer must not add a second break.
	prt.close()

	if strings.Count(buf.String(), "\n") != 1 {
		t.Errorf("lines: got %d, want 1\n%q", strings.Count(buf.String(), "\n"), buf.String())
	}
}

func TestProgressPrinterWithoutRewrite(t *testing.T) {
	var buf bytes.Buffer
	prt := newProgressPrinter(&buf, false)

	prt.print(toolapp.PullResponse{Progress: &toolapp.PullProgress{Src: "model.gguf", CurrentBytes: 100_000_000, TotalBytes: 400_000_000, MBPerSec: 25}})
	prt.print(toolapp.PullResponse{Progress: &toolapp.PullProgress{Src: "model.gguf", CurrentBytes: 200_000_000, TotalBytes: 400_000_000, MBPerSec: 25}})
	prt.close()

	got := buf.String()

	if strings.Contains(got, "\r") {
		t.Errorf("redirected output must not rewrite lines: %q", got)
	}

	if lines := strings.Count(got, "\n"); lines != 2 {
		t.Errorf("lines: got %d, want 2\n%q", lines, got)
	}
}

func TestProgressLine(t *testing.T) {
	tests := []struct {
		name     string
		progress toolapp.PullProgress
		contains []string
		excludes []string
	}{
		{
			name:     "percent and eta",
			progress: toolapp.PullProgress{Src: "model.gguf", CurrentBytes: 1_000_000_000, TotalBytes: 16_464_000_000, MBPerSec: 30},
			contains: []string{"Downloading model.gguf", "6.1%", "1.0/16.5 GB", "30.00 MB/s", "ETA 8m35s"},
		},
		{
			name:     "hours left",
			progress: toolapp.PullProgress{Src: "model.gguf", CurrentBytes: 1_000_000_000, TotalBytes: 16_464_000_000, MBPerSec: 1},
			contains: []string{"ETA 4h17m"},
		},
		{
			name:     "complete has no eta",
			progress: toolapp.PullProgress{Src: "model.gguf", CurrentBytes: 400_000_000, TotalBytes: 400_000_000, MBPerSec: 30, Complete: true},
			contains: []string{"100.0%", "400/400 MB"},
			excludes: []string{"ETA"},
		},
		{
			name:     "unknown total",
			progress: toolapp.PullProgress{Src: "model.gguf", CurrentBytes: 100_000_000, MBPerSec: 30},
			contains: []string{"100 MB", "30.00 MB/s"},
			excludes: []string{"%", "ETA"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := progressLine(test.progress)

			for _, want := range test.contains {
				if !strings.Contains(got, want) {
					t.Errorf("got %q, want it to contain %q", got, want)
				}
			}

			for _, unwanted := range test.excludes {
				if strings.Contains(got, unwanted) {
					t.Errorf("got %q, want it to exclude %q", got, unwanted)
				}
			}
		})
	}
}

func TestProgressLineKeepsColumnsStable(t *testing.T) {
	src := "Qwen3.8-27B-UD-Q4_K_M-00001-of-00009.gguf"

	updates := []toolapp.PullProgress{
		{Src: src, CurrentBytes: 100_000_000, TotalBytes: 16_464_000_000, MBPerSec: 9.5},
		{Src: src, CurrentBytes: 7_440_000_000, TotalBytes: 16_464_000_000, MBPerSec: 129.85},
		{Src: src, CurrentBytes: 16_464_000_000, TotalBytes: 16_464_000_000, MBPerSec: 29.85, Complete: true},
	}

	var pct, rate int
	for i, pg := range updates {
		got := progressLine(pg)

		// The columns must land on the same offset across updates or the
		// rewritten line jitters on screen.
		gotPct := strings.Index(got, "%")
		gotRate := strings.Index(got, " MB/s")

		if i == 0 {
			pct, rate = gotPct, gotRate
			continue
		}

		if gotPct != pct || gotRate != rate {
			t.Errorf("columns: got %d/%d, want %d/%d\n%q", gotPct, gotRate, pct, rate, got)
		}
	}
}

func TestProgressLineFitsTerminal(t *testing.T) {
	pg := toolapp.PullProgress{
		Src:          "Qwen3.8-27B-UD-Q4_K_M-with-a-really-long-file-name-00001-of-00009.gguf",
		CurrentBytes: 7_440_000_000,
		TotalBytes:   16_464_000_000,
		MBPerSec:     29.85,
	}

	got := progressLine(pg)

	if width := len([]rune(got)); width > maxLineWidth {
		t.Errorf("width: got %d, want at most %d\n%q", width, maxLineWidth, got)
	}

	if !strings.Contains(got, "00001-of-00009.gguf") {
		t.Errorf("the tail of the file name must survive truncation: %q", got)
	}
}
