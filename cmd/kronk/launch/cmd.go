// Package launch provides the "kronk launch opencode" command.
package launch

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/spf13/cobra"
)

// Cmd is the "kronk launch" command.
var Cmd = newCommand()

func newCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "launch opencode <model> [--host <IP:PORT>] [-- OpenCode arguments]",
		Short: "Install and launch OpenCode in an isolated temporary environment",
		Long: `Install OpenCode when necessary and launch it with Kronk's default
configuration in an isolated temporary environment.

OpenCode uses the selected model ID exactly as provided. Kronk resolves that ID
to an available downloaded model and optional configuration profile. Only that
model is exposed to the launched session.

Launch requires an already-running Kronk server. The default address is
localhost:11435; use --host when it runs elsewhere. Launch does not start,
configure, or stop the server.

The temporary workspace, configuration, credentials, cache, and session data
are removed when OpenCode exits. Existing user and project OpenCode
configuration is not loaded or modified.

Run 'kronk launch opencode uninstall' to invoke OpenCode's confirmed uninstall
flow.`,
		Args: cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if err := run(cmd, args); err != nil {
				fmt.Fprintln(os.Stderr, err)

				if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
					os.Exit(exitErr.ExitCode())
				}
				os.Exit(1)
			}
		},
	}
	cmd.Flags().String("host", "localhost:11435", "Address of the running Kronk server (IP:PORT)")

	return cmd
}

func run(cmd *cobra.Command, args []string) error {
	requested, opencodeArgs, err := parseArgs(args, cmd.ArgsLenAtDash())
	if err != nil {
		return err
	}
	if strings.EqualFold(requested, "uninstall") {
		if len(opencodeArgs) > 0 {
			return fmt.Errorf("OpenCode uninstall does not accept pass-through arguments")
		}
		return uninstallOpenCode()
	}
	host, _ := cmd.Flags().GetString("host")
	serverURL, err := normalizeServerURL(host)
	if err != nil {
		return err
	}

	selected := strings.TrimSpace(requested)
	fmt.Fprintf(os.Stderr, "Launching OpenCode\n  Kronk server: %s\n  Model: %s\n\n", serverURL, selected)

	fmt.Fprintln(os.Stderr, "[1/4] Checking the Kronk server and downloaded model...")
	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()
	if err := verifyServer(ctx, serverURL, selected); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "      Kronk is ready and the model is available.")

	fmt.Fprintln(os.Stderr, "[2/4] Locating OpenCode...")
	bin, err := ensureOpenCode()
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "      OpenCode: %s\n", bin)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	return launchOpenCode(bin, opencodeArgs, []string{selected}, serverURL, signals)
}

func normalizeServerURL(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "localhost:11435"
	}
	if !strings.Contains(host, "://") {
		host = "http://" + host
	}

	u, err := url.Parse(host)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", fmt.Errorf("invalid Kronk server address %q (expected IP:PORT)", host)
	}
	if _, _, err := net.SplitHostPort(u.Host); err != nil {
		return "", fmt.Errorf("invalid Kronk server address %q (expected IP:PORT)", host)
	}

	return strings.TrimSuffix(u.String(), "/"), nil
}

func verifyServer(ctx context.Context, serverURL, modelID string) error {
	modelsURL, err := url.JoinPath(serverURL, "/v1/models")
	if err != nil {
		return fmt.Errorf("building Kronk models URL: %w", err)
	}

	cln := client.New(client.NoopLogger, client.WithBearer(os.Getenv("KRONK_TOKEN")))
	var response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := cln.Do(ctx, http.MethodGet, modelsURL, nil, &response); err != nil {
		return fmt.Errorf("connecting to Kronk at %s: %w\n\nverify the server is running and --host is correct", serverURL, err)
	}

	ids := make([]string, len(response.Data))
	for i, model := range response.Data {
		ids[i] = model.ID
	}

	if !slices.Contains(ids, modelID) {
		return fmt.Errorf("model %q is not available from Kronk at %s", modelID, serverURL)
	}

	return nil
}

// parseArgs validates the supported agent and returns arguments following the
// "--" separator for OpenCode.
func parseArgs(args []string, dash int) (string, []string, error) {
	if len(args) == 0 {
		return "", nil, fmt.Errorf("an agent name and model are required\n\nexample: kronk launch opencode mradermacher/Qwopus3.5-4B-Coder.Q8_0/AGENT")
	}

	if !strings.EqualFold(args[0], "opencode") {
		return "", nil, fmt.Errorf("unsupported agent %q (supported: opencode)", args[0])
	}

	if dash == -1 {
		if len(args) < 2 {
			return "", nil, fmt.Errorf("a model is required\n\nexample: kronk launch opencode mradermacher/Qwopus3.5-4B-Coder.Q8_0/AGENT")
		}
		if len(args) > 2 {
			return "", nil, fmt.Errorf("unexpected arguments: %v\nuse '--' to pass arguments to OpenCode", args[2:])
		}
		return args[1], nil, nil
	}

	if dash != 2 {
		return "", nil, fmt.Errorf("expected 'opencode <model>' before '--'")
	}

	return args[1], args[2:], nil
}
