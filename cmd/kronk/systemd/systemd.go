// Package systemd provides Linux systemd user-service management for Kronk.
package systemd

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/spf13/cobra"
)

//go:embed kronk.service.tmpl
var unitTemplate string

// Cmd manages Kronk through the Linux systemd user service manager.
var Cmd = newCmd()

func newCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "systemd",
		Short: "Install and manage the Linux systemd user service",
		Long: `Install and manage Kronk as a Linux systemd user service. Run as your
normal user, not with sudo. systemd must be installed and its user manager available.

Unlike "kronk server start --detach", systemd owns this process and its logs.
Use "kronk systemd stop" and "kronk systemd logs" to manage it.

Install from a permanent location (go install, Homebrew, or a release binary),
not go run: the service records the absolute path of this executable.

EXAMPLES
  kronk systemd install          Register and enable startup at login
  kronk systemd install --boot   Also enable startup before login and after logout
  kronk systemd start            Start the installed service
  kronk systemd logs             Follow the service journal
  kronk systemd uninstall        Remove the service, preserving data and settings`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "linux" {
				return errors.New("systemd user services are supported only on Linux")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install and enable the user service without starting it",
		Long: `Install the embedded unit and enable startup at login, without starting
the server. Stop any manually started server before using "kronk systemd start".

Use --force to replace an existing unit after moving the binary. Configuration
in kronk.env and systemctl drop-ins is preserved. --base-path or KRONK_BASE_PATH
at installation selects the data directory; kronk.env can override it later.

--boot enables lingering for your account using loginctl. This may require
authorization and affects all your user services, not only Kronk. Uninstall
does not disable lingering because other services may depend on it.`,
		Args: cobra.NoArgs,
		RunE: install,
	}
	installCmd.Flags().Bool("force", false, "Replace an existing service unit")
	installCmd.Flags().Bool("boot", false, "Enable user lingering for startup at boot and operation after logout")
	cmd.AddCommand(installCmd)
	cmd.AddCommand(&cobra.Command{
		Use:   "uninstall",
		Short: "Stop, disable, and remove the service, preserving data and settings",
		Args:  cobra.NoArgs,
		RunE:  uninstall,
	})

	for _, action := range []struct {
		name  string
		short string
	}{
		{"start", "Start the installed systemd service"},
		{"stop", "Stop the systemd service"},
		{"restart", "Restart the systemd service after configuration or binary changes"},
		{"status", "Show systemd service status (not HTTP readiness)"},
	} {
		cmd.AddCommand(&cobra.Command{
			Use:   action.name,
			Short: action.short,
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runCommand(cmd, "systemctl", "--user", action.name, "kronk.service")
			},
		})
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "logs",
		Short: "Follow the systemd service journal (Ctrl-C to stop following)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommand(cmd, "journalctl", "--user", "--unit=kronk.service", "--follow")
		},
	})

	return cmd
}

// =============================================================================

func install(cmd *cobra.Command, args []string) error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("finding user configuration directory: %w", err)
	}
	unitPath := filepath.Join(configDir, "systemd", "user", "kronk.service")
	force, _ := cmd.Flags().GetBool("force")
	if _, err := os.Lstat(unitPath); err == nil {
		if !force {
			return fmt.Errorf("service unit already exists at %s; use --force to replace it", unitPath)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking service unit: %w", err)
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("finding systemctl (systemd is required): %w", err)
	}
	// Preserve the invoked symlink (for example Homebrew's bin/kronk), rather
	// than os.Executable's versioned target that an upgrade can remove.
	executable, err := exec.LookPath(os.Args[0])
	if err != nil {
		return fmt.Errorf("finding kronk executable: %w", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return fmt.Errorf("resolving kronk executable: %w", err)
	}
	envPath := filepath.Join(configDir, "kronk", "kronk.env")
	// EnvironmentFile takes an unquoted literal path, not a command argument.
	if strings.ContainsAny(envPath, "\r\n") {
		return errors.New("configuration directory must not contain line breaks")
	}
	baseSetting := ""
	if basePath := client.GetBasePath(cmd); basePath != "" {
		basePath, err = filepath.Abs(basePath)
		if err != nil {
			return fmt.Errorf("resolving data directory: %w", err)
		}
		baseSetting = "Environment=" + unitQuote("KRONK_BASE_PATH="+basePath)
	}
	unit := strings.NewReplacer(
		"@EXECUTABLE@", unitQuote(executable),
		"@BASE_PATH@", baseSetting,
		"@ENVIRONMENT_FILE@", "-"+strings.ReplaceAll(envPath, "%", "%%"),
	).Replace(unitTemplate)

	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		return fmt.Errorf("creating user service directory: %w", err)
	}
	// Remove only the explicitly selected unit, so --force replaces a symlink
	// rather than overwriting the file it points to.
	if force {
		if err := os.Remove(unitPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("replacing service unit: %w", err)
		}
	}
	file, err := os.OpenFile(unitPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("creating service unit: %w", err)
	}
	_, writeErr := file.WriteString(unit)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return fmt.Errorf("writing service unit: %w", err)
	}
	if err := runCommand(cmd, "systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	if err := runCommand(cmd, "systemctl", "--user", "enable", "kronk.service"); err != nil {
		return err
	}
	boot, _ := cmd.Flags().GetBool("boot")
	if boot {
		if err := runCommand(cmd, "loginctl", "enable-linger"); err != nil {
			return fmt.Errorf("service installed, but enabling startup at boot failed (authorization may be required): %w", err)
		}
	}
	cmd.Printf("Installed %s\nOptional settings: %s\n", unitPath, envPath)
	cmd.Println("Service enabled but not started. Stop any manual server, then run: kronk systemd start")
	return nil
}

func uninstall(cmd *cobra.Command, args []string) error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("finding user configuration directory: %w", err)
	}
	unitPath := filepath.Join(configDir, "systemd", "user", "kronk.service")
	if _, err := os.Lstat(unitPath); err != nil {
		return fmt.Errorf("finding installed service unit: %w", err)
	}
	if err := runCommand(cmd, "systemctl", "--user", "disable", "--now", "kronk.service"); err != nil {
		return err
	}
	if err := os.Remove(unitPath); err != nil {
		return fmt.Errorf("removing service unit: %w", err)
	}
	if err := runCommand(cmd, "systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	cmd.Println("Service removed. Models, configuration, systemctl drop-ins, and user lingering were preserved.")
	return nil
}

func runCommand(cmd *cobra.Command, name string, args ...string) error {
	process := exec.CommandContext(cmd.Context(), name, args...)
	process.Stdin = cmd.InOrStdin()
	process.Stdout = cmd.OutOrStdout()
	process.Stderr = cmd.ErrOrStderr()
	if err := process.Run(); err != nil {
		return fmt.Errorf("running %s (requires an available systemd user session): %w", name, err)
	}
	return nil
}

// unitQuote escapes a literal value for systemd's quoted unit-file syntax.
// Percent specifiers are escaped; ExecStart uses ':' to disable dollar expansion.
func unitQuote(value string) string {
	return `"` + strings.NewReplacer(
		`\`, `\\`, `"`, `\"`, "%", "%%",
		"\n", `\n`, "\r", `\r`, "\t", `\t`,
	).Replace(value) + `"`
}
