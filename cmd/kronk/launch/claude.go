package launch

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
)

// claudePlaceholderToken is sent as ANTHROPIC_AUTH_TOKEN when the Kronk
// server needs no auth. Claude Code requires a token to skip its login flow,
// but the value is ignored by a token-less Kronk server.
const claudePlaceholderToken = "kronk"

// claudeCode implements Runner for the Claude Code agent. Unlike OpenCode,
// Claude Code has no provider/model config file; it is configured entirely
// through environment variables and talks to Kronk's Anthropic-compatible
// Messages API at /v1/messages.
type claudeCode struct{}

// Run implements Runner. It ensures Claude Code is installed, builds the
// Anthropic environment pointing at the local Kronk server, and execs
// Claude Code with that environment.
func (claudeCode) Run(defaultModel string, chatModels []Model, args []string) error {
	install, err := loadInstall("claude")
	if err != nil {
		return fmt.Errorf("claude: %w", err)
	}

	bin, err := ensureInstalled(install)
	if err != nil {
		return err
	}

	// Honor a model the user selected via pass-through args (e.g.
	// "-- --model X"): Claude Code's CLI --model overrides ANTHROPIC_MODEL, so
	// describe that same model everywhere (tiers and the context window) instead
	// of leaving the launcher default's window pinned under the user's model.
	model := defaultModel
	if override := modelArgValue(args); override != "" {
		model = override
	}

	env, err := buildClaudeEnv(model, chatModels)
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

	// Neutralize any inherited cloud-provider/gateway routing flags so a user's
	// existing CLAUDE_CODE_USE_BEDROCK=1 (or Vertex/Foundry/Mantle/AWS/Gateway)
	// cannot divert requests away from the local Kronk server pointed to by
	// ANTHROPIC_BASE_URL. In gateway mode Claude Code reinterprets
	// ANTHROPIC_BASE_URL/ANTHROPIC_AUTH_TOKEN as a cloud-gateway URL/session
	// token, so an inherited CLAUDE_CODE_USE_GATEWAY would break the local
	// endpoint. These flags are read as truthy, so an empty value disables them;
	// "0" would not (a non-empty string is truthy). Go's exec keeps the last
	// value for duplicate keys, so these override the inherited ones.
	for _, flag := range []string{
		"CLAUDE_CODE_USE_BEDROCK",
		"CLAUDE_CODE_USE_VERTEX",
		"CLAUDE_CODE_USE_FOUNDRY",
		"CLAUDE_CODE_USE_MANTLE",
		"CLAUDE_CODE_USE_ANTHROPIC_AWS",
		"CLAUDE_CODE_USE_GATEWAY",
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
