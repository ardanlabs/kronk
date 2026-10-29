package tui

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// A catalog model is configured from its GGUF header alone, so the download
// waits until the entry is saved: a cancelled or failed download never costs
// the answers. The install offer (finish.go) starts it.

// pullProgressMsg is a progress event of the download.
type pullProgressMsg models.DownloadProgress

// pullDoneMsg ends the download; err is nil when it finished.
type pullDoneMsg struct {
	err error
}

// pull is the install offer's state and the download's.
type pull struct {
	offer  buttons
	note   string // shown above the buttons, e.g. cancelled
	err    error  // the last download failed
	copied bool   // the pull command was copied
	dir    string // where the model is installed, for All done

	ctx    context.Context
	cancel context.CancelFunc
	events chan models.DownloadProgress
	done   chan error
	last   models.DownloadProgress
	start  time.Time
	pulled bool // the model was downloaded
}

// needsPull reports whether the saved entry is for a model that is not
// downloaded. Audio and image models are only offered when installed.
func (a *app) needsPull() bool {
	return !a.server && !a.choice.Installed
}

// startPull starts the download in the background. Progress events come
// back one at a time through waitPull.
func (a *app) startPull() (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(a.ctx)
	events := make(chan models.DownloadProgress, 1)
	done := make(chan error, 1)

	a.pull.ctx, a.pull.cancel = ctx, cancel
	a.pull.events, a.pull.done = events, done
	a.pull.last = models.DownloadProgress{}
	a.pull.start = time.Now()
	a.pull.note, a.pull.err = "", nil
	a.step = stepDownloading

	src, c := a.src, a.choice
	go func() {
		done <- src.Pull(ctx, c, func(p models.DownloadProgress) {
			// Keep the download going when the screen is behind; a later
			// event carries the bytes so far.
			select {
			case events <- p:
			default:
			}
		})
	}()

	// The SDK reports a model file every 100 MB; the spinner and the time
	// show it is working in between.
	return a, tea.Batch(a.spin.Tick, a.waitPull())
}

// waitPull waits for the next progress event or the end of the download.
func (a *app) waitPull() tea.Cmd {
	events, done := a.pull.events, a.pull.done
	return func() tea.Msg {
		select {
		case p := <-events:
			return pullProgressMsg(p)
		case err := <-done:
			return pullDoneMsg{err: err}
		}
	}
}

// pullDone ends the download: All done when it finished, otherwise the
// install offer again with what happened.
func (a *app) pullDone(err error) (tea.Model, tea.Cmd) {
	cancelled := a.pull.ctx != nil && a.pull.ctx.Err() != nil
	if a.pull.cancel != nil {
		a.pull.cancel()
	}

	switch {
	case err == nil:
		a.pull.pulled = true
		return a.finish()
	case cancelled:
		a.pull.note = "Download cancelled."
	default:
		a.pull.err = err
	}

	a.step = stepOffer
	return a, nil
}

// pullKey handles a key during the download: esc cancels it, ctrl+c also
// quits.
func (a *app) pullKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.pull.cancel()
	case "ctrl+c":
		a.pull.cancel()
		return a, tea.Quit
	}
	return a, nil
}

// downloadView shows the file being downloaded and its progress.
func (a *app) downloadView() string {
	p := a.pull.last

	width := 50
	if w := a.contentWidth(); w > 0 {
		width = min(w, 70)
	}
	var frac float64
	if p.TotalBytes > 0 {
		frac = min(max(float64(p.CurrentBytes)/float64(p.TotalBytes), 0), 1)
	}
	filled := int(frac * float64(width))
	bar := a.st.heading.Render(strings.Repeat("█", filled)) + a.st.muted.Render(strings.Repeat("░", width-filled)) +
		fmt.Sprintf(" %3.0f%%", frac*100)

	file := "Downloading " + a.choice.ID
	if p.Src != "" {
		file = "Downloading " + path.Base(p.Src)
	}

	stats := time.Since(a.pull.start).Truncate(time.Second).String()

	// The whisper download reports no progress: the size and the time
	// show it is working.
	if !source.PullReportsProgress(a.choice) {
		if a.choice.Size > 0 {
			stats = source.FormatBytes(uint64(a.choice.Size)) + " · " + stats
		}
		return lipgloss.JoinVertical(lipgloss.Left,
			a.spin.View()+" "+a.st.heading.Render(file),
			a.st.muted.Render(stats),
			"",
			a.st.help.Render("esc cancels the download"),
		)
	}

	if p.TotalBytes > 0 {
		stats = fmt.Sprintf("%s of %s · %.0f MB/s · %s",
			source.FormatBytes(uint64(max(p.CurrentBytes, 0))), source.FormatBytes(uint64(p.TotalBytes)), p.MBPerSec, stats)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		a.spin.View()+" "+a.st.heading.Render(file),
		bar,
		a.st.muted.Render(stats),
		"",
		a.st.help.Render("esc cancels the download"),
	)
}
