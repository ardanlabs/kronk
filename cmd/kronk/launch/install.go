package launch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const openCodeInstallHint = "curl -fsSL https://opencode.ai/install | bash"
const openCodeDocsURL = "https://opencode.ai/docs/"

// ensureOpenCode returns the OpenCode executable, offering to install it when
// it is not already available.
func ensureOpenCode() (string, error) {
	if bin, ok := findOpenCode(); ok {
		return bin, nil
	}

	bin, args, hint, deps, err := openCodeInstaller(runtime.GOOS)
	if err != nil {
		return "", err
	}

	missing := make([]string, 0, len(deps))
	for _, dep := range deps {
		if _, err := exec.LookPath(dep); err != nil {
			missing = append(missing, dep)
		}
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("OpenCode installation requires %s\n\ninstall the missing tools, then run:\n  %s", strings.Join(missing, ", "), hint)
	}

	if !isInteractive() {
		return "", fmt.Errorf("OpenCode is not installed\n\nmore information: %s\ninstall it, then re-run the launch command:\n  %s", openCodeDocsURL, hint)
	}

	fmt.Fprintf(os.Stderr, "OpenCode is not installed.\n\nMore information: %s\nKronk can install it with:\n  %s\n\nInstall OpenCode now? (y/N): ", openCodeDocsURL, hint)
	answer := readPromptLine()
	if answer != "y" && answer != "yes" {
		return "", fmt.Errorf("OpenCode was not installed\n\nmore information: %s", openCodeDocsURL)
	}

	fmt.Fprintln(os.Stderr, "Installing OpenCode...")

	cmd := exec.Command(bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("installing OpenCode: %w", err)
	}

	installed, ok := findOpenCode()
	if !ok {
		return "", fmt.Errorf("OpenCode was installed but its executable could not be found")
	}

	return installed, nil
}

// uninstallOpenCode delegates removal to OpenCode so its own installer and
// package-manager-aware cleanup and confirmation flow remain authoritative.
func uninstallOpenCode() error {
	bin, ok := findOpenCode()
	if !ok {
		return fmt.Errorf("OpenCode is not installed\n\nmore information: %s", openCodeDocsURL)
	}

	fmt.Fprintf(os.Stderr, "OpenCode will show what it plans to remove and ask for confirmation.\nMore information: %s\n\n", openCodeDocsURL+"cli/#uninstall")

	cmd := exec.Command(bin, "uninstall")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("uninstalling OpenCode: %w", err)
	}

	return nil
}

// findOpenCode locates OpenCode on PATH or in the official installer's default
// directory.
func findOpenCode() (string, bool) {
	if bin, err := exec.LookPath("opencode"); err == nil {
		return bin, true
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}

	name := "opencode"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	bin := filepath.Join(home, ".opencode", "bin", name)
	if info, err := os.Stat(bin); err == nil && !info.IsDir() {
		return bin, true
	}

	return "", false
}

// openCodeInstaller returns the official OpenCode installation command for the
// specified operating system.
func openCodeInstaller(goos string) (string, []string, string, []string, error) {
	switch goos {
	case "darwin", "linux":
		return "bash", []string{"-c", "set -o pipefail; " + openCodeInstallHint}, openCodeInstallHint, []string{"bash", "curl"}, nil
	case "windows":
		hint := "npm install -g opencode-ai@latest"
		return "npm", []string{"install", "-g", "opencode-ai@latest"}, hint, []string{"npm"}, nil
	default:
		return "", nil, "", nil, fmt.Errorf("automatic OpenCode installation is not supported on %s\n\nsee https://opencode.ai/docs for installation instructions", goos)
	}
}

func isInteractive() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func readPromptLine() string {
	var input []byte
	one := make([]byte, 1)

	for {
		n, err := os.Stdin.Read(one)
		if n > 0 {
			if one[0] == '\n' {
				break
			}
			input = append(input, one[0])
		}
		if err != nil {
			break
		}
	}

	return strings.ToLower(strings.TrimSpace(string(input)))
}
