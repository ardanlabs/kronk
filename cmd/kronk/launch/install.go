package launch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// agentInstall describes how to locate and, if necessary, install a coding
// agent's binary. Each supported agent supplies one of these so the install
// flow (find on PATH, prompt, check deps, run installer, re-find) is shared
// instead of duplicated per agent.
type agentInstall struct {
	// bin is the executable name to look for (e.g. "opencode", "claude").
	bin string

	// display is the human-facing agent name (e.g. "OpenCode", "Claude Code").
	display string

	// fallbackDirs are directories under the user's home to search when bin is
	// not on PATH (e.g. ".opencode/bin", ".local/bin"), since a fresh
	// installer may not be on PATH in the current shell yet.
	fallbackDirs []string

	// installHint returns the human-readable install command for goos.
	installHint func(goos string) string

	// installerCommand returns the command and args that install the agent on
	// goos, or an error when the platform is unsupported.
	installerCommand func(goos string) (string, []string, error)

	// checkDeps verifies the tools needed to run the installer on goos are
	// present, returning a helpful error when they are not.
	checkDeps func(goos string) error
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
// installer.
func confirmInstall(display, hint string) bool {
	fmt.Fprintf(os.Stderr, "%s is not installed. Install it now with:\n  %s\nProceed? (y/N): ", display, hint)

	var response string
	fmt.Scanln(&response)

	return response == "y" || response == "Y"
}
