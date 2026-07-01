package launch

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// Cmd is the "kronk launch" command.
var Cmd = &cobra.Command{
	Use:   "launch [agent] [-- extra args]",
	Short: "Launch a coding agent wired to your local Kronk server",
	Long: `Launch a coding agent pre-configured to use your local Kronk server and the
chat models installed on it.

The launcher talks to a running Kronk server, discovers the installed
chat-capable models, and starts the agent with a generated configuration.
The Kronk server must already be running; start it first with "kronk server
start" if it is not.

Supported agents:
  opencode   OpenCode (https://opencode.ai)
  claude     Claude Code (https://claude.com/claude-code)
  codex      Codex CLI (https://developers.openai.com/codex)
  copilot    GitHub Copilot CLI (https://github.com/features/copilot/cli)
  pi         Pi (https://pi.dev)
  openclaw   OpenClaw (https://openclaw.ai)
  hermes     Hermes Agent (https://hermes-agent.nousresearch.com)
  vscode     VS Code + GitHub Copilot Chat BYOK (https://code.visualstudio.com)

EXAMPLES

  # Launch OpenCode using the first installed chat model as the default
  kronk launch opencode

  # Launch Claude Code wired to the local Kronk server
  kronk launch claude

  # Launch Codex CLI wired to the local Kronk server
  kronk launch codex

  # Launch GitHub Copilot CLI wired to the local Kronk server
  kronk launch copilot

  # Launch Pi wired to the local Kronk server
  kronk launch pi

  # Launch OpenClaw's local TUI wired to the local Kronk server
  kronk launch openclaw

  # Launch Hermes Agent wired to the local Kronk server
  kronk launch hermes

  # Configure VS Code (Copilot Chat BYOK) for the local Kronk server and open it
  kronk launch vscode

  # Launch with a specific installed model as the default
  # (use the model id shown by "kronk model ls", e.g. Qwen3-8B-Q8_0)
  kronk launch opencode --model Qwen3-8B-Q8_0

  # Pass extra arguments through to the agent
  kronk launch opencode -- --help`,
	Args: cobra.ArbitraryArgs,
	Run:  main,
}

func main(cmd *cobra.Command, args []string) {
	if err := run(cmd, args); err != nil {
		// When the launched agent exits non-zero, propagate its exit code
		// instead of collapsing to 1 (and don't print "exit status N" on top
		// of the agent's own output, e.g. after Ctrl-C).
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if code := exitErr.ExitCode(); code >= 0 {
				os.Exit(code)
			}
			os.Exit(1)
		}

		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	Cmd.Flags().String("model", "", "Default model id for the agent (defaults to the first installed chat model)")
}
