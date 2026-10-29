package config

import (
	"image/color"
	"testing"
)

func TestDarkBackground(t *testing.T) {
	for theme, want := range map[string]bool{"dark": true, "light": false} {
		if got, err := darkBackground(theme); err != nil || got != want {
			t.Errorf("--theme %s: dark = %v, %v", theme, got, err)
		}
	}
	if _, err := darkBackground("blue"); err == nil {
		t.Error("--theme blue: want an error")
	}
}

func TestColorFGBG(t *testing.T) {
	for v, want := range map[string]struct{ dark, ok bool }{
		"15;0":         {true, true},
		"0;15":         {false, true},
		"0;default;7":  {false, true},
		"7;default;0":  {true, true},
		"":             {false, false},
		"0;default":    {false, false},
		"0;nonsense;9": {true, true},
	} {
		if dark, ok := colorFGBG(v); dark != want.dark || ok != want.ok {
			t.Errorf("COLORFGBG=%q: dark = %v, ok = %v; want %v, %v", v, dark, ok, want.dark, want.ok)
		}
	}
}

func TestIsDark(t *testing.T) {
	for c, want := range map[color.RGBA]bool{
		{0xff, 0xff, 0xff, 0xff}: false,
		{0x1e, 0x1e, 0x1e, 0xff}: true,
		{0xfd, 0xf6, 0xe3, 0xff}: false, // Solarized light
		{0x00, 0x2b, 0x36, 0xff}: true,  // Solarized dark
	} {
		if got := isDark(c); got != want {
			t.Errorf("%v: dark = %v, want %v", c, got, want)
		}
	}
}
