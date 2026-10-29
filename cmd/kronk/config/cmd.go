// Package config provides the config command, an interactive terminal tool
// built with Bubble Tea that creates or edits a model entry in
// model_config.yaml and saves the file where the server reads it.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// ErrNoTerminal is returned when the command is started without a terminal.
var ErrNoTerminal = errors.New("kronk config needs an interactive terminal")

var Cmd = &cobra.Command{
	Use:   "config",
	Short: "Create or edit model and server settings in model_config.yaml (interactive)",
	Long: `Create or edit model and server settings in model_config.yaml (interactive)

Pick the config file, then configure a model or the server settings (kms:).

For a model, say whether the config is for this machine or another one. This
machine's hardware is detected; for another machine, enter its GPUs, GPU
memory and RAM. Pick an installed or catalog model (chat, audio or image),
pick the entry or a new variant, see what AutoTune picks for it on that
hardware, answer the questions, review the entry with a memory check and save
it.

For the server, pick the sections to change and answer their questions. The
questions come from the server's own settings.

Only answered settings are written; everything else in the file, including
comments, is kept. The previous file is kept as .bak. Esc goes back.

The command works on local files and needs no server. The file the server
reads is KRONK_POOL_MODEL_CONFIG_FILE when set, otherwise
<base-path>/models/model_config.yaml. The tool also lists model_config*.yaml
files in the current directory, and can save to any path.

Environment Variables:
      KRONK_BASE_PATH               Base path for kronk data (models, libraries, catalog, model_config)
      KRONK_POOL_MODEL_CONFIG_FILE  Model config file the server reads`,
	Args: cobra.NoArgs,
	RunE: run,
}

func run(cmd *cobra.Command, args []string) error {
	if !isTerminal() {
		return ErrNoTerminal
	}

	src, err := newSource(client.GetBasePath(cmd))
	if err != nil {
		return err
	}

	isDark := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)

	p := tea.NewProgram(newApp(cmd.Context(), src, isDark), tea.WithContext(cmd.Context()))
	if _, err := p.Run(); err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		return fmt.Errorf("config: %w", err)
	}

	return nil
}

func isTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) &&
		term.IsTerminal(int(os.Stdout.Fd())) &&
		os.Getenv("TERM") != "dumb"
}

// shortPath shows a path under the home directory as ~/...
func shortPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.Join("~", rel)
	}
	return path
}

func expandHome(path string) string {
	path = strings.TrimSpace(path)
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	return path
}
