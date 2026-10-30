// Package config provides the config command code: it starts the terminal
// UI that writes model_config.yaml.
//
// The work is split by concern, each package only importing the ones before
// it:
//
//	configfile  edits model_config.yaml, keeping the rest of the file
//	source      reads the local models, catalog, devices and templates
//	questions   what is asked, the answers, and the changes they make
//	tui         the Bubble Tea program, one huh form per step
package config

import (
	"errors"
	"fmt"
	"image/color"
	"os"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
	"github.com/ardanlabs/kronk/cmd/kronk/config/tui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// ErrNoTerminal is returned when the command is started without a terminal.
var ErrNoTerminal = errors.New("kronk config needs an interactive terminal")

func run(cmd *cobra.Command) error {
	if !isTerminal() {
		return ErrNoTerminal
	}

	src, err := source.New(client.GetBasePath(cmd))
	if err != nil {
		return err
	}

	theme, _ := cmd.Flags().GetString("theme")
	isDark, err := darkBackground(theme)
	if err != nil {
		return err
	}

	// With --theme auto the colors follow the background when it changes.
	follow := theme == "" || theme == "auto"

	p := tea.NewProgram(tui.New(cmd.Context(), src, isDark, follow), tea.WithContext(cmd.Context()))
	m, err := p.Run()
	if err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		return fmt.Errorf("config: %w", err)
	}

	// The screen is gone by now: the error is printed again and sets the
	// exit code.
	return tui.Err(m)
}

func isTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) &&
		term.IsTerminal(int(os.Stdout.Fd())) &&
		os.Getenv("TERM") != "dumb"
}

// darkBackground reports whether to use the colors for a dark terminal. The
// terminal is asked for its background; when it doesn't answer (some
// terminals, or through tmux or ssh), COLORFGBG says, which many terminals
// set. Without either, dark is assumed, so --theme light fixes a white
// terminal whose colors are hard to read.
func darkBackground(theme string) (bool, error) {
	switch theme {
	case "dark":
		return true, nil
	case "light":
		return false, nil
	case "", "auto":
	default:
		return false, fmt.Errorf("--theme %q: use auto, light or dark", theme)
	}

	if bg, err := lipgloss.BackgroundColor(os.Stdin, os.Stdout); err == nil && bg != nil {
		return isDark(bg), nil
	}

	if dark, ok := colorFGBG(os.Getenv("COLORFGBG")); ok {
		return dark, nil
	}

	return true, nil
}

// isDark reports whether c is dark: HSL lightness below one half, as
// lipgloss.HasDarkBackground decides.
func isDark(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return max(r, g, b)+min(r, g, b) < 0xffff
}

// colorFGBG reads COLORFGBG ("fg;bg" or "fg;default;bg"): its last field is
// the background's ANSI color, where 7 (white) and 15 (bright white) are
// light.
func colorFGBG(v string) (dark bool, ok bool) {
	if v == "" {
		return false, false
	}
	parts := strings.Split(v, ";")
	n, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil || n < 0 || n > 15 {
		return false, false
	}
	return n != 7 && n != 15, true
}
