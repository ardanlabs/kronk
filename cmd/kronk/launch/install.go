package launch

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// agentInstall describes how to locate and, if necessary, install a coding
// agent's binary. It is populated from the embedded agents metadata (see
// metadata.go) so the find/install flow is shared and data-driven instead of
// duplicated per agent.
type agentInstall struct {
	// bin is the executable name to look for (e.g. "opencode", "claude").
	bin string

	// display is the human-facing agent name (e.g. "OpenCode", "Claude Code").
	display string

	// fallbackDirs are directories under the user's home to search when bin is
	// not on PATH (e.g. ".opencode/bin", ".local/bin"), since a fresh
	// installer may not be on PATH in the current shell yet.
	fallbackDirs []string

	// docsURL backs the "see <url> for installation instructions" message shown
	// on platforms with no install recipe.
	docsURL string

	// perOS holds the per-OS install recipe, keyed by runtime.GOOS values
	// (darwin | linux | windows).
	perOS map[string]osInstall
}

// installHint returns the human-readable install command for goos, or a
// docs-link fallback when the platform has no recipe.
func (a agentInstall) installHint(goos string) string {
	if oi, ok := a.perOS[goos]; ok {
		return oi.Hint
	}

	return fmt.Sprintf("see %s for installation instructions", a.docsURL)
}

// installerCommand returns the command and args that install the agent on goos,
// or an error when the platform is unsupported.
func (a agentInstall) installerCommand(goos string) (string, []string, error) {
	oi, ok := a.perOS[goos]
	if !ok {
		return "", nil, fmt.Errorf("unsupported platform for %s install: %s", a.bin, goos)
	}

	return oi.Command.Bin, oi.Command.Args, nil
}

// checkDeps verifies the tools needed to run the installer on goos are present.
// The message is built from a shared, neutral template fed by the per-OS
// metadata: it always surfaces the actual install command (the real fix), notes
// any version/edition specifics (deps_note), and can be fully overridden by a
// bespoke deps_error when the template cannot be made correct.
func (a agentInstall) checkDeps(goos string) error {
	oi, ok := a.perOS[goos]
	if !ok {
		return fmt.Errorf("%s is not installed and automatic install is not supported on %s\n\ninstall it manually: %s", a.display, goos, a.installHint(goos))
	}

	var missing []string
	for _, dep := range oi.Deps {
		if _, err := exec.LookPath(dep); err != nil {
			missing = append(missing, dep)
		}
	}

	if len(missing) == 0 {
		return nil
	}

	// Guardrail: a fully bespoke message wins verbatim when supplied.
	if oi.DepsError != "" {
		return errors.New(oi.DepsError)
	}

	// Shared template. Phrasing is neutral about why a dep is needed ("for
	// setup", not "to install it"), the version nuance is preserved via
	// deps_note, and the actual install command is always shown as the fix.
	note := ""
	if oi.DepsNote != "" {
		note = " (" + oi.DepsNote + ")"
	}

	return fmt.Errorf("%s requires %s for setup%s.\n\ninstall them, then run:\n  %s\n\nthen re-run: kronk launch %s",
		a.display, strings.Join(missing, ", "), note, a.installHint(goos), a.bin)
}

// find returns the agent binary path, checking PATH first and then the
// installer's fallback directories under the user's home (which may not be on
// PATH in the current shell yet).
func (a agentInstall) find() (string, bool) {
	if p, err := exec.LookPath(a.bin); err == nil {
		return p, true
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}

	name := a.bin
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	for _, dir := range a.fallbackDirs {
		candidate := filepath.Join(home, filepath.FromSlash(dir), name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
	}

	return "", false
}

// ensureInstalled returns the agent binary path, installing it first if it is
// not already present. On a non-interactive terminal it never runs a network
// installer and instead returns an error pointing at the install command.
func ensureInstalled(a agentInstall) (string, error) {
	if bin, ok := a.find(); ok {
		return bin, nil
	}

	notInstalledErr := fmt.Errorf("%s is not installed\n\ninstall it and re-run: kronk launch %s\n\ninstall command:\n  %s", a.display, a.bin, a.installHint(runtime.GOOS))

	// On a non-interactive terminal never run a network installer; just point
	// the user at the install command.
	if !isInteractive() {
		return "", notInstalledErr
	}

	if err := a.checkDeps(runtime.GOOS); err != nil {
		return "", err
	}

	if !confirmInstall(a.display, a.installHint(runtime.GOOS)) {
		return "", notInstalledErr
	}

	bin, args, err := a.installerCommand(runtime.GOOS)
	if err != nil {
		return "", err
	}

	fmt.Fprintf(os.Stderr, "Installing %s...\n", a.display)

	cmd := exec.Command(bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("install %s: %w", a.bin, err)
	}

	p, ok := a.find()
	if !ok {
		return "", fmt.Errorf("%s was installed but not found on PATH; restart your shell and re-run: kronk launch %s", a.display, a.bin)
	}

	return p, nil
}

// isInteractive reports whether stdin is a terminal (so we can safely prompt).
func isInteractive() bool {
	stat, err := os.Stdin.Stat()
	return err == nil && stat.Mode()&os.ModeCharDevice != 0
}

// confirmInstall asks the user for permission before running an agent's
// installer. Only "y"/"yes" (case-insensitive) confirms; empty input, EOF, or
// anything else declines.
func confirmInstall(display, hint string) bool {
	fmt.Fprintf(os.Stderr, "%s is not installed. Install it now with:\n  %s\nProceed? (y/N): ", display, hint)

	line, _ := readPromptLine()
	switch strings.ToLower(line) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// readPromptLine reads a single line from stdin for an interactive prompt,
// returning it trimmed of surrounding whitespace. ok is false on EOF or a read
// error before any newline, which callers treat as "no input".
//
// It reads one byte at a time from os.Stdin rather than using a buffered reader:
// these prompts run immediately before the launcher execs an agent that inherits
// the same stdin, so a buffered reader could consume input past the newline and
// swallow the user's first keystrokes to the agent.
func readPromptLine() (string, bool) {
	var b []byte
	buf := make([]byte, 1)

	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				return strings.TrimSpace(string(b)), true
			}
			b = append(b, buf[0])
		}
		if err != nil {
			return strings.TrimSpace(string(b)), len(b) > 0
		}
	}
}
