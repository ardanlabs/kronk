package tui

import (
	"image/color"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// Go brand colors (go.dev/s/brandbook).
var (
	goLightBlue = lipgloss.Color("#5DC9E2")
	goAqua      = lipgloss.Color("#00A29C")
	goFuchsia   = lipgloss.Color("#CE3262")
	goYellow    = lipgloss.Color("#FDDD00")
	white       = lipgloss.Color("#FFFFFF")
)

// Darker or lighter shades of the brand colors, the nearest ones that read
// well: at least 4.5:1 contrast (WCAG AA) for text, 3:1 for borders. The
// brand colors themselves are too light on white (Go blue 2.6:1, aqua 3.2:1)
// and Go fuchsia too dark on black (3.4:1).
var (
	blueOnWhite    = lipgloss.Color("#00728E") // 5.5:1 on white
	aquaOnWhite    = lipgloss.Color("#00837E") // 4.6:1 on white
	fuchsiaOnBlack = lipgloss.Color("#D85B81") // 4.6:1 on #1E1E1E
	buttonBlue     = lipgloss.Color("#0080A0") // white text on it: 4.6:1
)

// palette holds the colors used by the screens and the form theme for the
// current terminal background.
type palette struct {
	accent color.Color
	text   color.Color
	muted  color.Color
	faint  color.Color
	ok     color.Color
	warn   color.Color
	bad    color.Color
}

func newPalette(isDark bool) palette {
	ld := lipgloss.LightDark(isDark)

	return palette{
		accent: ld(blueOnWhite, goLightBlue),                     // 5.5:1, 8.7:1
		text:   ld(lipgloss.Color("235"), lipgloss.Color("252")), // 15:1, 11:1
		muted:  ld(lipgloss.Color("241"), lipgloss.Color("246")), // 6.1:1, 5.5:1
		faint:  ld(lipgloss.Color("246"), lipgloss.Color("242")), // borders: 3.0:1, 3.2:1
		ok:     ld(aquaOnWhite, goAqua),                          // 4.6:1, 5.3:1
		warn:   ld(lipgloss.Color("#9A6700"), goYellow),          // 4.9:1, 12:1
		bad:    ld(goFuchsia, fuchsiaOnBlack),                    // 4.9:1, 4.6:1
	}
}

// styles are the lipgloss styles for everything outside the forms.
type styles struct {
	brand    lipgloss.Style
	subtitle lipgloss.Style
	heading  lipgloss.Style
	label    lipgloss.Style
	value    lipgloss.Style
	muted    lipgloss.Style
	selected lipgloss.Style
	added    lipgloss.Style
	removed  lipgloss.Style
	ok       lipgloss.Style
	warn     lipgloss.Style
	bad      lipgloss.Style
	box      lipgloss.Style
	help     lipgloss.Style
}

func newStyles(isDark bool) styles {
	p := newPalette(isDark)

	return styles{
		brand:    lipgloss.NewStyle().Bold(true).Foreground(white).Background(buttonBlue).Padding(0, 1),
		subtitle: lipgloss.NewStyle().Foreground(p.muted),
		heading:  lipgloss.NewStyle().Bold(true).Foreground(p.accent),
		label:    lipgloss.NewStyle().Foreground(p.muted),
		value:    lipgloss.NewStyle().Foreground(p.text),
		muted:    lipgloss.NewStyle().Foreground(p.muted),
		selected: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.LightDark(isDark)(lipgloss.Color("0"), white)),
		added:    lipgloss.NewStyle().Foreground(p.ok),
		removed:  lipgloss.NewStyle().Foreground(p.bad),
		ok:       lipgloss.NewStyle().Foreground(p.ok),
		warn:     lipgloss.NewStyle().Foreground(p.warn),
		bad:      lipgloss.NewStyle().Foreground(p.bad),
		box:      lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(p.faint).Padding(0, 1),
		help:     lipgloss.NewStyle().Foreground(p.muted),
	}
}

// formTheme is the huh theme in Go blue.
func formTheme(isDark bool) *huh.Styles {
	t := huh.ThemeBase(isDark)
	p := newPalette(isDark)
	ld := lipgloss.LightDark(isDark)

	t.Focused.Base = t.Focused.Base.BorderForeground(p.muted)
	t.Focused.Card = t.Focused.Base
	t.Focused.Title = t.Focused.Title.Foreground(p.accent).Bold(true)
	t.Focused.NoteTitle = t.Focused.NoteTitle.Foreground(p.accent).Bold(true).MarginBottom(1)
	// A blank line after the description sets the answers apart, on every
	// screen.
	t.Focused.Description = t.Focused.Description.Foreground(p.text).MarginBottom(1)
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(p.bad)
	t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(p.bad)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(p.accent)
	t.Focused.NextIndicator = t.Focused.NextIndicator.Foreground(p.accent)
	t.Focused.PrevIndicator = t.Focused.PrevIndicator.Foreground(p.accent)
	t.Focused.Option = t.Focused.Option.Foreground(p.text)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(p.accent)
	t.Focused.SelectedPrefix = lipgloss.NewStyle().Foreground(p.accent).SetString("✓ ")
	t.Focused.UnselectedPrefix = lipgloss.NewStyle().Foreground(p.muted).SetString("• ")
	t.Focused.UnselectedOption = t.Focused.UnselectedOption.Foreground(p.text)
	t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(white).Background(buttonBlue)
	t.Focused.Next = t.Focused.FocusedButton
	t.Focused.BlurredButton = t.Focused.BlurredButton.Foreground(p.text).Background(ld(lipgloss.Color("252"), lipgloss.Color("237")))
	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(p.accent)
	t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(p.muted)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(p.accent)

	t.Blurred = t.Focused
	t.Blurred.Base = t.Focused.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.Card = t.Blurred.Base
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()

	t.Group.Title = t.Focused.Title
	t.Group.Description = t.Focused.Description

	// huh's own key help is too faint on a dark background.
	t.Help.ShortKey = t.Help.ShortKey.Foreground(p.text)
	t.Help.FullKey = t.Help.FullKey.Foreground(p.text)
	t.Help.ShortDesc = t.Help.ShortDesc.Foreground(p.muted)
	t.Help.FullDesc = t.Help.FullDesc.Foreground(p.muted)
	t.Help.ShortSeparator = t.Help.ShortSeparator.Foreground(p.muted)
	t.Help.FullSeparator = t.Help.FullSeparator.Foreground(p.muted)

	return t
}
