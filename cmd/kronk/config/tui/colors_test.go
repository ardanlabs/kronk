package tui

import (
	"context"
	"image/color"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
)

func TestColorsFollowTheBackground(t *testing.T) {
	a := newApp(context.Background(), nil, true)
	a.colors.follow = true
	a.choices = []source.Choice{{ID: "a/b", Installed: true}, {ID: "tiny", Kind: source.KindAudioModel}}
	a.openGroups()
	old := a.currentForm()

	white := tea.BackgroundColorMsg{Color: color.RGBA{0xff, 0xff, 0xff, 0xff}}
	a.Update(white)

	if a.colors.dark || a.theme.Theme(true) != a.colors.forms[0] {
		t.Fatal("white background: the light theme is not in use")
	}
	if a.st.heading.GetForeground() != newStyles(false).heading.GetForeground() {
		t.Fatal("white background: the screens keep the dark styles")
	}
	if a.currentForm() == old || a.step != stepGroup {
		t.Fatal("the form on screen is not built again in the new colors")
	}

	// The same background again changes nothing.
	same := a.currentForm()
	a.Update(white)
	if a.currentForm() != same {
		t.Fatal("an unchanged background rebuilt the form")
	}
}

func TestFixedThemeIgnoresTheBackground(t *testing.T) {
	a := newApp(context.Background(), nil, true) // --theme dark: no follow
	a.Update(tea.BackgroundColorMsg{Color: color.RGBA{0xff, 0xff, 0xff, 0xff}})
	if !a.colors.dark {
		t.Fatal("--theme dark switched to light")
	}
}
