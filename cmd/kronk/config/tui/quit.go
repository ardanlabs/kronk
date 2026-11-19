package tui

import (
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/ardanlabs/kronk/cmd/kronk/config/questions"
)

const (
	btnSaveQuit    = "Save and quit"
	btnDiscardQuit = "Discard and quit"
	btnContinue    = "Continue editing"
)

// requestQuit offers to save only when an entry has pending changes. Invalid
// input also needs confirmation: it must not be silently discarded.
func (a *app) requestQuit() (tea.Model, tea.Cmd) {
	if a.ans == nil {
		return a, tea.Quit
	}
	changes, err := questions.Diff(a.qs, a.facts, a.ans, a.existing)
	if err == nil && len(changes) == 0 {
		return a, tea.Quit
	}
	a.quitFrom = a.step
	a.step = stepQuit
	a.quit = buttons{labels: []string{btnSaveQuit, btnDiscardQuit, btnContinue}, at: 2}
	return a, nil
}

func (a *app) quitKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		a.quitAfterSave = false
		return a.reopen(a.quitFrom)
	}
	i, ok := a.quit.key(msg)
	if !ok {
		return a, nil
	}
	switch a.quit.labels[i] {
	case btnDiscardQuit:
		return a, tea.Quit
	case btnContinue:
		a.quitAfterSave = false
		return a.reopen(a.quitFrom)
	case btnSaveQuit:
		a.quitAfterSave = true
		a.list.open = false
		return a.skipToReview()
	}
	return a, nil
}

func (a *app) quitView() string {
	b := a.quit
	b.labels = slices.Clone(b.labels)
	b.labels[b.at] = "> " + b.labels[b.at]
	return a.buttonBox(a.st.box.Padding(1, 2), func(int) (string, string) {
		return "Save your current changes before quitting?\nNothing is discarded unless you choose Discard and quit.", ""
	}, b, "esc continue editing")
}
