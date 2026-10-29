package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// The colors follow the terminal's background while the tool runs: switching
// the terminal or the system between light and dark restyles the screens.
// The terminal is asked for its background now and then, the same query as at
// startup, since few terminals report the switch on their own.

// backgroundEvery is how often the background is asked for.
const backgroundEvery = 2 * time.Second

// colors is the palette in use, for a dark or a light background.
type colors struct {
	dark   bool
	follow bool           // ask for the background again: --theme auto
	forms  [2]*huh.Styles // the form themes, light and dark
}

func newColors(dark bool, follow bool) *colors {
	return &colors{
		dark:   dark,
		follow: follow,
		forms:  [2]*huh.Styles{formTheme(false), formTheme(true)},
	}
}

// form is the form theme for the current background. huh reads it each time
// it draws, so a switch shows at once.
func (c *colors) form() *huh.Styles {
	if c.dark {
		return c.forms[1]
	}
	return c.forms[0]
}

// backgroundTickMsg is the time to ask for the background again.
type backgroundTickMsg struct{}

// watchBackground asks for the background after a while.
func (a *app) watchBackground() tea.Cmd {
	if !a.colors.follow {
		return nil
	}
	return tea.Tick(backgroundEvery, func(time.Time) tea.Msg { return backgroundTickMsg{} })
}

// setBackground restyles the screens when the background turned from dark to
// light, or back. The form on screen is built again: its option labels and
// descriptions carry the old colors.
func (a *app) setBackground(dark bool) (tea.Model, tea.Cmd) {
	if !a.colors.follow || dark == a.colors.dark {
		return a, nil
	}

	a.colors.dark = dark
	a.st = newStyles(dark)
	a.spin.Style = a.st.heading

	if a.currentForm() == nil {
		return a, nil
	}
	return a.reopen(a.step)
}
