package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/ardanlabs/kronk/cmd/kronk/config/configfile"
	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func savedApp(c source.Choice) *app {
	a := newApp(context.Background(), nil, true)
	a.step, a.choice, a.path = stepSaving, c, "/tmp/model_config.yaml"
	return a
}

func TestSavedInstalledModelIsDone(t *testing.T) {
	a := savedApp(source.Choice{ID: "a/b", Installed: true})
	a.Update(savedMsg{path: a.path})
	if a.step != stepDone || a.done.labels[a.done.at] != btnExit {
		t.Fatalf("step = %v, focus = %v; want All done on Exit: nothing to install", a.step, a.done)
	}
}

func TestSavedCatalogModelOffersDownload(t *testing.T) {
	a := savedApp(source.Choice{ID: "a/b", Size: 1 << 30})
	a.Update(savedMsg{path: a.path})
	if a.step != stepOffer || a.pull.offer.labels[a.pull.offer.at] != btnInstall {
		t.Fatalf("step = %v, offer = %v; want the install offered first", a.step, a.pull.offer)
	}

	// Esc skips the install: All done says what to run.
	a.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if a.step != stepDone || a.pull.copied || a.pull.pulled {
		t.Fatalf("esc: step = %v; want All done, model not installed", a.step)
	}
}

func TestAnotherMachineOnlyGetsTheCommand(t *testing.T) {
	a := savedApp(source.Choice{ID: "a/b"})
	a.facts.Other = &source.Machine{RAM: 64 << 30}
	a.Update(savedMsg{path: a.path})
	if a.step != stepOffer || len(a.pull.offer.labels) != 1 || a.pull.offer.labels[0] != btnCopy {
		t.Fatalf("step = %v, offer = %v; want only the command, no install here", a.step, a.pull.offer)
	}
}

func TestPullDone(t *testing.T) {
	a := savedApp(source.Choice{ID: "a/b"})
	a.Update(savedMsg{path: a.path})

	a.step = stepDownloading
	a.Update(pullDoneMsg{})
	if a.step != stepDone || !a.pull.pulled {
		t.Fatalf("finished: step = %v, pulled = %v", a.step, a.pull.pulled)
	}

	a.step, a.pull.pulled = stepDownloading, false
	a.Update(pullDoneMsg{err: errors.New("disk full")})
	if a.step != stepOffer || a.pull.err == nil {
		t.Fatalf("failed: step = %v, err = %v; want the choices again with the error", a.step, a.pull.err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.step, a.pull.err, a.pull.ctx = stepDownloading, nil, ctx
	a.Update(pullDoneMsg{err: context.Canceled})
	if a.step != stepOffer || a.pull.err != nil || a.pull.note == "" {
		t.Fatalf("cancelled: step = %v, err = %v, note = %q", a.step, a.pull.err, a.pull.note)
	}
}

func TestDownloadProgressIsShown(t *testing.T) {
	a := savedApp(source.Choice{ID: "a/b"})
	a.step = stepDownloading
	a.pull.events, a.pull.done = make(chan models.DownloadProgress, 1), make(chan error, 1)

	_, cmd := a.Update(pullProgressMsg{Src: "https://host/a/b/model-Q8_0.gguf", CurrentBytes: 300 << 20, TotalBytes: 1200 << 20, MBPerSec: 40})
	if cmd == nil {
		t.Fatal("no wait for the next progress event")
	}

	view := a.downloadView()
	for _, want := range []string{"model-Q8_0.gguf", "25%", "300 MB of 1.2 GB", "40 MB/s"} {
		if !strings.Contains(view, want) {
			t.Errorf("download view lacks %q:\n%s", want, view)
		}
	}
}

func TestCopyThenAllDone(t *testing.T) {
	a := savedApp(source.Choice{ID: "a/b"})
	a.Update(savedMsg{path: a.path})

	if got := a.pull.offer.labels; len(got) != 2 || got[0] != btnCopy || got[1] != btnInstall {
		t.Fatalf("buttons = %v; want copy left, install right", got)
	}
	a.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	_, cmd := a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.step != stepDone || !a.pull.copied || cmd == nil {
		t.Fatalf("copy: step = %v, copied = %v; want All done and the clipboard set", a.step, a.pull.copied)
	}
	if view := a.doneView(); !strings.Contains(view, "All done!") || !strings.Contains(view, source.PullCommand(a.choice)) {
		t.Fatalf("All done lacks the title or the command:\n%s", view)
	}

	// Enter on the default button exits.
	_, cmd = a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Exit did not quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("Exit did not quit")
	}
}

func TestConfigureAnother(t *testing.T) {
	a := savedApp(source.Choice{ID: "a/b", Installed: true})
	a.start = newStart("/srv/model_config.yaml")
	a.start.target = targetAnother
	a.Update(savedMsg{path: a.path})

	a.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if a.done.labels[a.done.at] != "Configure another model" {
		t.Fatalf("left: focus on %q", a.done.labels[a.done.at])
	}
	a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if a.step != stepStart || a.start.target != targetAnother || a.choice.ID != "" || a.savedPath != "" {
		t.Fatalf("again: step = %v, target = %q, choice = %q; want the start with its answers, the rest reset", a.step, a.start.target, a.choice.ID)
	}
}

func TestAllDoneSaysWhatWasWritten(t *testing.T) {
	a := savedApp(source.Choice{ID: "tiny", Kind: source.KindAudioModel, Installed: true})
	a.entryID, a.section = "tiny", "bucky-models"
	a.Update(savedMsg{path: a.path})
	if view := a.doneView(); !strings.Contains(view, "Kronk's defaults apply") {
		t.Fatalf("nothing written, but All done says otherwise:\n%s", view)
	}

	a.changes = []configfile.Change{{Key: "nseq-max", New: 2}}
	if view := a.doneView(); !strings.Contains(view, "tiny under bucky-models:") {
		t.Fatalf("entry written, but All done doesn't name it:\n%s", view)
	}
}

func TestOfferWrapsOnNarrowScreens(t *testing.T) {
	a := savedApp(source.Choice{ID: "unsloth/Qwen3-1.7B-Q4_K_M", Size: 1 << 30})
	a.Update(savedMsg{path: a.path})
	a.width, a.height = 40, 40

	a.Update(tea.KeyPressMsg{Code: tea.KeyEscape}) // skip to All done, its widest case
	views := map[string]string{"offer": "", "All done": a.doneView()}
	a.step = stepOffer
	views["offer"] = a.offerView()

	for name, view := range views {
		for line := range strings.SplitSeq(view, "\n") {
			if w := lipgloss.Width(line); w > 40 {
				t.Errorf("%s: line is %d wide on a 40 column screen: %q", name, w, line)
			}
		}
	}
}
