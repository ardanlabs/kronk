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

// downloadView shows the download in a box like the install offer's: what
// is downloading, the file and its progress, with the key help under it.
func (a *app) downloadView() string {
	p := a.pull.last
	box := a.st.box.BorderForeground(a.st.heading.GetForeground()).Padding(1, 2)

	// The bar fills the box, leaving room for the percentage.
	width := 50
	if w := a.contentWidth(); w > 0 {
		width = min(max(w-box.GetHorizontalFrameSize()-5, 10), 60)
	}

	lines := []string{
		a.spin.View() + " " + a.st.heading.Render("Downloading "+a.choice.ID),
		a.st.muted.Render("The configuration is saved. The server can load the model once it is here."),
		"",
	}

	stats := time.Since(a.pull.start).Truncate(time.Second).String()
	switch {
	case !source.PullReportsProgress(a.choice):
		// The whisper download reports no progress: the size and the time
		// show it is working.
		if a.choice.Size > 0 {
			stats = source.FormatBytes(uint64(a.choice.Size)) + " · " + stats
		}
		lines = append(lines, a.st.value.Render(stats))

	default:
		var frac float64
		if p.TotalBytes > 0 {
			frac = min(max(float64(p.CurrentBytes)/float64(p.TotalBytes), 0), 1)
			stats = fmt.Sprintf("%s of %s · %.0f MB/s · %s",
				source.FormatBytes(uint64(max(p.CurrentBytes, 0))), source.FormatBytes(uint64(p.TotalBytes)), p.MBPerSec, stats)
		}
		filled := int(frac * float64(width))
		bar := a.st.heading.Render(strings.Repeat("█", filled)) + a.st.muted.Render(strings.Repeat("░", width-filled)) +
			a.st.value.Render(fmt.Sprintf(" %3.0f%%", frac*100))

		if p.Src != "" {
			lines = append(lines, a.st.label.Render("File  ")+a.st.value.Render(path.Base(p.Src)), "")
		}
		lines = append(lines, bar, a.st.muted.Render(stats))
	}

	content := strings.Join(lines, "\n")
	if w := a.contentWidth(); w > 0 && lipgloss.Width(box.Render(content)) > w {
		box = box.Width(w)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		box.Render(content),
		"",
		a.wrap(a.st.help.Render("esc cancels the download")),
	)
}
