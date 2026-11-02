package service

import (
	"bytes"
	_ "embed"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"text/template"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/spf13/cobra"
)

const launchdLabel = "com.ardanlabs.kronk"

//go:embed templates/com.ardanlabs.kronk.plist.tmpl
var launchdPlistTemplate string

// NewLaunchdCommand constructs macOS launchd user-agent commands. Help is
// available on every platform; operations require a macOS desktop login session.
func NewLaunchdCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "launchd",
		Short: "Install and manage the macOS launchd user agent",
		Long: `Install and manage Kronk as a macOS per-user LaunchAgent. Run as your
normal user, not with sudo. A macOS desktop login session must be available.

Unlike "kronk server start --detach", launchd owns this process and its logs.
Use "kronk launchd stop" and "kronk launchd logs" to manage it.

Install from a permanent location (go install, Homebrew, or a release binary),
not go run: the agent records the absolute path used to invoke this executable.`,
		Example: `
  kronk launchd install     # Install and enable startup at desktop login
  kronk launchd start       # Load the agent and start the server
  kronk launchd restart     # Reload the plist and restart the server
  kronk launchd logs        # Follow ~/Library/Logs/kronk/kronk.log
  kronk launchd uninstall   # Remove the agent, preserving server data and logs`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "darwin" {
				return errors.New("launchd user agents are supported only on macOS")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install and enable the user agent without starting it",
		Long: `Install the embedded LaunchAgent and enable startup at desktop login,
without starting the server. Stop any manual server before "kronk launchd start".

--base-path or KRONK_BASE_PATH at installation selects the data directory.
Other server settings can be set in the existing model_config.yaml or in the
plist's EnvironmentVariables dictionary. Shell startup files are not sourced.

Use --force to replace an existing plist after moving the binary. This resets
manual plist edits, but preserves models, keys, server YAML configuration, and
logs. Run "kronk launchd restart" afterwards if the agent is already loaded.`,
		Args: cobra.NoArgs,
		RunE: installLaunchd,
	}
	installCmd.Flags().Bool("force", false, "Replace the existing plist, resetting manual plist edits")
	cmd.AddCommand(installCmd)

	for _, action := range []struct {
		name  string
		short string
		run   func(*cobra.Command, []string) error
	}{
		{"start", "Load and start the installed LaunchAgent", startLaunchd},
		{"stop", "Unload the LaunchAgent until started again or the next desktop login", stopLaunchd},
		{"restart", "Gracefully unload and reload the LaunchAgent from its plist", restartLaunchd},
		{"uninstall", "Stop, disable, and remove the agent, preserving server data and logs", uninstallLaunchd},
		{"status", "Show launchd agent status (not HTTP readiness)", func(cmd *cobra.Command, args []string) error {
			return runLaunchdCommand(cmd, "launchctl", "print", launchdServiceTarget())
		}},
		{"logs", "Follow the agent's combined output log (Ctrl-C to stop following)", func(cmd *cobra.Command, args []string) error {
			_, logPath, err := launchdPaths()
			if err != nil {
				return err
			}
			return runLaunchdCommand(cmd, "tail", "-n", "50", "-F", logPath)
		}},
	} {
		cmd.AddCommand(&cobra.Command{Use: action.name, Short: action.short, Args: cobra.NoArgs, RunE: action.run})
	}

	return cmd
}

// =============================================================================

func installLaunchd(cmd *cobra.Command, args []string) error {
	plistPath, logPath, err := launchdPaths()
	if err != nil {
		return err
	}
	force, _ := cmd.Flags().GetBool("force")
	if _, err := os.Lstat(plistPath); err == nil {
		if !force {
			return fmt.Errorf("agent plist already exists at %s; use --force to replace it", plistPath)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking agent plist: %w", err)
	}
	// Preserve stable symlinks such as Homebrew's bin/kronk across upgrades.
	executable, err := exec.LookPath(os.Args[0])
	if err != nil {
		return fmt.Errorf("finding kronk executable: %w", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return fmt.Errorf("resolving kronk executable: %w", err)
	}
	basePath := client.GetBasePath(cmd)
	if basePath != "" {
		basePath, err = filepath.Abs(basePath)
		if err != nil {
			return fmt.Errorf("resolving data directory: %w", err)
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("finding home directory: %w", err)
	}
	tmpl, err := template.New("agent").Funcs(template.FuncMap{"string": func(value string) (string, error) {
		data, err := xml.Marshal(value)
		return string(data), err
	}}).Parse(launchdPlistTemplate)
	if err != nil {
		return fmt.Errorf("parsing embedded agent template: %w", err)
	}
	var plist bytes.Buffer
	if err := tmpl.Execute(&plist, struct {
		Executable string
		Home       string
		BasePath   string
		LogPath    string
	}{executable, home, basePath, logPath}); err != nil {
		return fmt.Errorf("rendering agent plist: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		return fmt.Errorf("creating LaunchAgents directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return fmt.Errorf("creating agent log directory: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("creating agent log: %w", err)
	}
	if err := logFile.Close(); err != nil {
		return fmt.Errorf("closing agent log: %w", err)
	}
	// Replace the selected plist, not the target of a possible symlink.
	if force {
		if err := os.Remove(plistPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("replacing agent plist: %w", err)
		}
	}
	file, err := os.OpenFile(plistPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("creating agent plist: %w", err)
	}
	_, writeErr := file.Write(plist.Bytes())
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return fmt.Errorf("writing agent plist: %w", err)
	}
	if err := runLaunchdCommand(cmd, "launchctl", "enable", launchdServiceTarget()); err != nil {
		return err
	}
	cmd.Printf("Installed %s\nLogs: %s\n", plistPath, logPath)
	cmd.Println("Run kronk launchd start, or kronk launchd restart if already loaded. Stop any manual server first.")
	return nil
}

func startLaunchd(cmd *cobra.Command, args []string) error {
	plistPath, _, err := launchdPaths()
	if err != nil {
		return err
	}
	if _, err := os.Stat(plistPath); err != nil {
		return fmt.Errorf("finding installed agent (run kronk launchd install first): %w", err)
	}
	if err := runLaunchdCommand(cmd, "launchctl", "enable", launchdServiceTarget()); err != nil {
		return err
	}
	active, err := launchdLoaded(cmd)
	if err != nil {
		return err
	}
	if active {
		return runLaunchdCommand(cmd, "launchctl", "kickstart", launchdServiceTarget())
	}
	return runLaunchdCommand(cmd, "launchctl", "bootstrap", launchdDomainTarget(), plistPath)
}

func stopLaunchd(cmd *cobra.Command, args []string) error {
	active, err := launchdLoaded(cmd)
	if err != nil {
		return err
	}
	if !active {
		cmd.Println("LaunchAgent is not loaded.")
		return nil
	}
	if err := runLaunchdCommand(cmd, "launchctl", "bootout", launchdServiceTarget()); err != nil {
		return err
	}
	// bootout can return while the job is still shutting down. Wait for its
	// removal so a following start cannot kickstart the disappearing old job.
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		active, err := launchdLoaded(cmd)
		if err != nil {
			return err
		}
		if !active {
			return nil
		}
		select {
		case <-cmd.Context().Done():
			return cmd.Context().Err()
		case <-ticker.C:
		}
	}
}

func restartLaunchd(cmd *cobra.Command, args []string) error {
	if err := stopLaunchd(cmd, args); err != nil {
		return err
	}
	return startLaunchd(cmd, args)
}

func uninstallLaunchd(cmd *cobra.Command, args []string) error {
	plistPath, _, err := launchdPaths()
	if err != nil {
		return err
	}
	if _, err := os.Lstat(plistPath); err != nil {
		return fmt.Errorf("finding installed agent plist: %w", err)
	}
	if err := stopLaunchd(cmd, args); err != nil {
		return err
	}
	if err := runLaunchdCommand(cmd, "launchctl", "disable", launchdServiceTarget()); err != nil {
		return err
	}
	if err := os.Remove(plistPath); err != nil {
		return fmt.Errorf("removing agent plist: %w", err)
	}
	cmd.Println("LaunchAgent removed. Server data, YAML configuration, and logs were preserved.")
	return nil
}

func launchdPaths() (string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", fmt.Errorf("finding home directory: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist"),
		filepath.Join(home, "Library", "Logs", "kronk", "kronk.log"), nil
}

func launchdDomainTarget() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func launchdServiceTarget() string {
	return launchdDomainTarget() + "/" + launchdLabel
}

func launchdLoaded(cmd *cobra.Command) (bool, error) {
	output, err := exec.CommandContext(cmd.Context(), "launchctl", "print", launchdServiceTarget()).CombinedOutput()
	if err == nil {
		return true, nil
	}
	// launchctl error 113 is "Could not find specified service". Missing GUI
	// domains have a different error and must not be mistaken for a stopped job.
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && exitErr.ExitCode() == 113 {
		return false, nil
	}
	return false, fmt.Errorf("checking LaunchAgent (requires a macOS desktop login session): %s: %w", output, err)
}

func runLaunchdCommand(cmd *cobra.Command, name string, args ...string) error {
	process := exec.CommandContext(cmd.Context(), name, args...)
	process.Stdin = cmd.InOrStdin()
	process.Stdout = cmd.OutOrStdout()
	process.Stderr = cmd.ErrOrStderr()
	if err := process.Run(); err != nil {
		return fmt.Errorf("running %s: %w", name, err)
	}
	return nil
}
