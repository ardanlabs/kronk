package launch

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// longTemplate is the "kronk launch" long help. The two %s placeholders are
// filled at init time from the embedded curated-models metadata (see
// curatedAliasReference and curatedAliasExamples) so the alias help stays in
// sync with yaml/models.yaml instead of being hardcoded here.
const longTemplate = `Launch a coding agent pre-configured to use your local Kronk server and the
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

  # Swap models with a short curated alias (works for ANY agent):
%s
%s

  # Or pass any installed model id shown by "kronk model ls"
  kronk launch codex --model Qwen3-8B-Q8_0

  # Pass extra arguments through to the agent
  kronk launch opencode -- --help`

// Cmd is the "kronk launch" command.
var Cmd = &cobra.Command{
	Use:   "launch [agent] [-- extra args]",
	Short: "Launch a coding agent wired to your local Kronk server",
	Args:  cobra.ArbitraryArgs,
	Run:   main,
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
	Cmd.Long = fmt.Sprintf(longTemplate, curatedAliasReference(), curatedAliasExamples())

	usage := "Model for the agent: any installed model id. Defaults to the preferred curated model."
	if list := curatedAliasList(); list != "" {
		usage = fmt.Sprintf("Model for the agent: a curated alias (%s) or any installed model id. Defaults to the preferred curated model.", list)
	}
	Cmd.Flags().String("model", "", usage)
}

// curatedAliasList returns the curated aliases in metadata order joined by
// ", " (e.g. "qwen, qwen-mtp, gemma"), for the --model flag usage. It is empty
// when metadata is unavailable.
func curatedAliasList() string {
	var names []string
	for _, m := range orderedCurated() {
		if m.Alias != "" {
			names = append(names, m.Alias)
		}
	}

	return strings.Join(names, ", ")
}

// curatedAliasReference returns the indented "  #   <alias>  <Display> (<Quant>)"
// lines (aliases aligned) in metadata order for the command's long help. It is
// empty when metadata is unavailable.
func curatedAliasReference() string {
	cur := orderedCurated()

	width := 0
	for _, m := range cur {
		if len(m.Alias) > width {
			width = len(m.Alias)
		}
	}

	var b strings.Builder
	for _, m := range cur {
		if m.Alias == "" {
			continue
		}
		fmt.Fprintf(&b, "  #   %-*s  %s (%s)\n", width, m.Alias, m.Display, m.Quant)
	}

	return strings.TrimRight(b.String(), "\n")
}

// curatedAliasExamples returns two example "--model <alias>" launch lines built
// from the first (and second, when present) curated alias, so the examples in
// the long help track yaml/models.yaml. It is empty when no alias exists.
func curatedAliasExamples() string {
	var aliases []string
	for _, m := range orderedCurated() {
		if m.Alias != "" {
			aliases = append(aliases, m.Alias)
		}
	}

	if len(aliases) == 0 {
		return ""
	}

	first := aliases[0]
	second := first
	if len(aliases) > 1 {
		second = aliases[1]
	}

	return fmt.Sprintf("  kronk launch claude   --model %s\n  kronk launch opencode --model %s", first, second)
}
