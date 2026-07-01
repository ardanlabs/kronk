package launch

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
)

const claudeInstallScript = "curl -fsSL https://claude.ai/install.sh | bash"

// claudePlaceholderToken is sent as ANTHROPIC_AUTH_TOKEN when the Kronk
// server needs no auth. Claude Code requires a token to skip its login flow
// but the value is ignored by a token-less Kronk server (this mirrors
// Ollama, which sets ANTHROPIC_AUTH_TOKEN=ollama).
const claudePlaceholderToken = "kronk"

// claudeInstall describes how to locate and install the Claude Code binary.
// The native installer (used on all platforms) drops the binary in
// ~/.local/bin, which may not be on PATH in the current shell yet.
var claudeInstall = agentInstall{
	bin:              "claude",
	display:          "Claude Code",
	fallbackDirs:     []string{".local/bin"},
	installHint:      claudeInstallHint,
	installerCommand: claudeInstallerCommand,
	checkDeps:        checkClaudeInstallDeps,
}

// claudeCode implements Runner for the Claude Code agent. Unlike OpenCode,
// Claude Code has no provider/model config file; it is configured entirely
// through environment variables and talks to Kronk's Anthropic-compatible
// Messages API at /v1/messages.
type claudeCode struct{}

// Run implements Runner. It ensures Claude Code is installed, builds the
// Anthropic environment pointing at the local Kronk server, and execs
// Claude Code with that environment.
func (claudeCode) Run(defaultModel string, chatModels []Model, args []string) error {
	bin, err := ensureInstalled(claudeInstall)
	if err != nil {
		return err
	}

	env, err := buildClaudeEnv(defaultModel, chatModels)
	if err != nil {
		return fmt.Errorf("build claude env: %w", err)
	}

	cmd := exec.Command(bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), env...)

	return cmd.Run()
}

// buildClaudeEnv returns the environment variables that point Claude Code at
// the local Kronk server:
//
//   - ANTHROPIC_BASE_URL: the Kronk server root. Claude Code appends
//     "/v1/messages" itself, which Kronk serves.
//   - ANTHROPIC_AUTH_TOKEN: forwarded as "Authorization: Bearer". Uses
//     KRONK_TOKEN when set, otherwise a placeholder so Claude Code skips its
//     login flow (the value is ignored by a token-less server).
//   - Model tiers: the chosen local model is used for every tier Claude Code
//     might request. ANTHROPIC_MODEL is the main model; the Haiku/Sonnet/Opus
//     defaults are pinned too because, against a non-first-party provider,
//     Claude Code otherwise falls back to built-in names (e.g. "haiku-4.5")
//     for background/compaction tasks, which the Kronk server does not have.
//     ANTHROPIC_SMALL_FAST_MODEL (deprecated but still honored by older Claude
//     Code versions) is set for the same reason.
//   - CLAUDE_CODE_MAX_CONTEXT_TOKENS: the model's resolved context window, so
//     Claude Code (which would otherwise assume a Claude-sized window for an
//     unrecognized model name) compacts to fit the server's window.
func buildClaudeEnv(defaultModel string, chatModels []Model) ([]string, error) {
	if defaultModel == "" || len(chatModels) == 0 {
		return nil, fmt.Errorf("a default model and at least one model are required")
	}

	baseURL, err := client.DefaultURL("")
	if err != nil {
		return nil, fmt.Errorf("default-url: %w", err)
	}
	baseURL = strings.TrimRight(baseURL, "/")

	token := os.Getenv("KRONK_TOKEN")
	if token == "" {
		token = claudePlaceholderToken
	}

	env := []string{
		"ANTHROPIC_BASE_URL=" + baseURL,
		"ANTHROPIC_AUTH_TOKEN=" + token,
		"ANTHROPIC_MODEL=" + defaultModel,
		"ANTHROPIC_DEFAULT_HAIKU_MODEL=" + defaultModel,
		"ANTHROPIC_DEFAULT_SONNET_MODEL=" + defaultModel,
		"ANTHROPIC_DEFAULT_OPUS_MODEL=" + defaultModel,
		"ANTHROPIC_SMALL_FAST_MODEL=" + defaultModel,
	}

	// Neutralize any inherited cloud-provider routing flags so a user's
	// existing CLAUDE_CODE_USE_BEDROCK=1 (or Vertex/Foundry/Mantle/AWS)
	// cannot divert requests away from the local Kronk server pointed to by
	// ANTHROPIC_BASE_URL. These flags are read as truthy, so an empty value
	// disables them; "0" would not (a non-empty string is truthy). Go's exec
	// keeps the last value for duplicate keys, so these override the inherited
	// ones.
	for _, flag := range []string{
		"CLAUDE_CODE_USE_BEDROCK",
		"CLAUDE_CODE_USE_VERTEX",
		"CLAUDE_CODE_USE_FOUNDRY",
		"CLAUDE_CODE_USE_MANTLE",
		"CLAUDE_CODE_USE_ANTHROPIC_AWS",
	} {
		env = append(env, flag+"=")
	}

	if cw := contextFor(defaultModel, chatModels); cw > 0 {
		env = append(env, "CLAUDE_CODE_MAX_CONTEXT_TOKENS="+strconv.Itoa(cw))
	}

	return env, nil
}

// contextFor returns the resolved context window of the model with the given
// id, or 0 when it is unknown.
func contextFor(id string, chatModels []Model) int {
	for _, m := range chatModels {
		if m.ID == id {
			return m.Context
		}
	}
	return 0
}

// checkClaudeInstallDeps verifies the tools needed to install Claude Code on
// the given OS are available.
func checkClaudeInstallDeps(goos string) error {
	switch goos {
	case "windows":
		if _, err := exec.LookPath("powershell"); err != nil {
			return fmt.Errorf("claude is not installed and PowerShell is required to install it\n\ninstall it manually: %s\n\nthen re-run: kronk launch claude", claudeInstallHint(goos))
		}

	case "darwin", "linux":
		var missing []string
		if _, err := exec.LookPath("curl"); err != nil {
			missing = append(missing, "curl")
		}
		if _, err := exec.LookPath("bash"); err != nil {
			missing = append(missing, "bash")
		}
		if len(missing) > 0 {
			return fmt.Errorf("claude is not installed and these tools are required to install it: %s\n\ninstall them, then re-run: kronk launch claude", strings.Join(missing, ", "))
		}

	default:
		return fmt.Errorf("claude is not installed and automatic install is not supported on %s\n\ninstall it manually: %s", goos, claudeInstallHint(goos))
	}

	return nil
}

// claudeInstallHint returns the human-readable install command for the given
// OS.
func claudeInstallHint(goos string) string {
	switch goos {
	case "windows":
		return "irm https://claude.ai/install.ps1 | iex"
	case "darwin", "linux":
		return claudeInstallScript
	default:
		return "see https://docs.claude.com/en/docs/claude-code for installation instructions"
	}
}

// claudeInstallerCommand returns the command that installs Claude Code on the
// given OS using Anthropic's native installer.
func claudeInstallerCommand(goos string) (string, []string, error) {
	switch goos {
	case "windows":
		return "powershell", []string{"-NoProfile", "-Command", "irm https://claude.ai/install.ps1 | iex"}, nil
	case "darwin", "linux":
		return "bash", []string{"-c", "set -o pipefail; " + claudeInstallScript}, nil
	default:
		return "", nil, fmt.Errorf("unsupported platform for claude install: %s", goos)
	}
}
