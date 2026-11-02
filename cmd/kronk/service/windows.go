package service

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// NewWindowsCommand constructs Windows service management commands. The supplied
// server command validates installation flags. Help is available on every platform;
// construction never contacts SCM.
func NewWindowsCommand(server *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Install and manage the native Windows service",
		Long: `Install and manage Kronk as a native Windows service. Administrative
operations require an elevated terminal. Linux users should use "kronk systemd";
macOS users should use "kronk launchd" instead.

Install from a permanent administrator-controlled binary location, not go run.
The service runs independently of your login, using LocalService by default.
Unlike --detach, Windows owns this process. Use service stop and service logs.`,
		Example: `  kronk service install
  kronk service start
  kronk service status
  kronk service logs
  kronk service stop
  kronk service uninstall`,
		RunE: func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
	install := &cobra.Command{
		Use:   "install [flags] [-- server flags]",
		Short: "Register automatic startup without starting the service",
		Long: `Register the Windows service without starting it. The default data directory
is %ProgramData%\Kronk, overridden by --base-path or KRONK_BASE_PATH.
Pass additional server flags after --. Use absolute configuration/library paths.
Store secrets in protected configuration, not service command-line arguments.

--force updates a stopped service and preserves its account unless --account is
explicitly supplied. A named account password is read securely from the terminal.
The account must already have the Windows "Log on as a service" right.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && cmd.ArgsLenAtDash() != 0 {
				return errors.New("pass server flags after --")
			}
			if err := validateServerArgs(server, args); err != nil {
				return err
			}
			return installService(cmd, args, server)
		},
	}
	install.Flags().Bool("force", false, "Update an existing stopped service registration")
	install.Flags().String("start-type", "auto", "Startup type: auto or manual")
	install.Flags().String("account", `NT AUTHORITY\LocalService`, "Service account (named accounts prompt for a password)")
	cmd.AddCommand(install)
	for _, action := range []struct{ name, short string }{
		{"uninstall", "Stop and remove the service, preserving data, settings, and logs"},
		{"start", "Start the installed service"},
		{"stop", "Stop the service and wait for model unload"},
		{"restart", "Stop gracefully and start the service again"},
		{"status", "Show Windows service state and exit codes (not HTTP readiness)"},
		{"logs", "Follow the installed service's output log (Ctrl-C to stop)"},
	} {
		cmd.AddCommand(&cobra.Command{
			Use: action.name, Short: action.short, Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error { return manageService(cmd, action.name) },
		})
	}
	return cmd
}

func validateServerArgs(cmd *cobra.Command, args []string) error {
	cmd.InitDefaultHelpFlag()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.ParseFlags(args); err != nil {
		return fmt.Errorf("server arguments: %w", err)
	}
	if cmd.Flags().NArg() != 0 {
		return errors.New("server arguments must be flags, not positional arguments")
	}
	if cmd.Flags().Changed("base-path") {
		return errors.New("server flag --base-path is not allowed in service arguments")
	}
	return validateServerFlags(cmd)
}

func validateServerFlags(cmd *cobra.Command) error {
	for _, name := range []string{"detach", "hf-token", "mcp-brave-api-key", "web-admin-password-sha-256", "help", "version"} {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("server flag --%s is not allowed in service arguments", name)
		}
	}
	return nil
}
