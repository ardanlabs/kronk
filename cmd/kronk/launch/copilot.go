package launch

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
)

// copilotOutputReserveFraction is the share of a model's context window held
// back for the agent's output. Copilot does not know our model, so we tell it
// the prompt/output budgets explicitly; reserving a slice for output keeps
// prompt+output within the server's context window and avoids overflow.
const copilotOutputReserveFraction = 4 // reserve 1/4 of the window for output

// copilot implements Runner for GitHub Copilot CLI. Copilot CLI has no
// provider/model config file to touch; it is configured entirely through its
// documented BYOK environment variables and talks to Kronk's
// OpenAI-compatible Chat Completions API at /v1/chat/completions.
type copilot struct{}

// Run implements Runner. It ensures Copilot CLI is installed, builds the BYOK
// environment pointing at the local Kronk server, and execs Copilot with that
// environment (args are passed straight through).
func (copilot) Run(defaultModel string, chatModels []Model, args []string) error {
	install, err := loadInstall("copilot")
	if err != nil {
		return fmt.Errorf("copilot: %w", err)
	}

	bin, err := ensureInstalled(install)
	if err != nil {
		return err
	}

	// Honor a model the user selected via pass-through args (e.g.
	// "-- --model X") instead of pinning the launcher's default over it.
	env, err := buildCopilotEnv(defaultModel, chatModels, modelArgValue(args))
	if err != nil {
		return fmt.Errorf("build copilot env: %w", err)
	}

	cmd := exec.Command(bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), env...)

	return cmd.Run()
}

// buildCopilotEnv returns the BYOK environment variables that point Copilot
// CLI at the local Kronk server:
//
//   - COPILOT_PROVIDER_TYPE=openai: use the OpenAI-compatible provider (Kronk
//     serves Chat Completions). Set explicitly so an inherited
//     COPILOT_PROVIDER_TYPE (azure/anthropic) from the user's environment
//     cannot divert Copilot to the wrong wire protocol.
//   - COPILOT_PROVIDER_BASE_URL: the Kronk /v1 base URL; Copilot appends
//     "/chat/completions" itself.
//   - COPILOT_PROVIDER_API_KEY: forwarded as the bearer token. Uses
//     KRONK_TOKEN when set; left empty otherwise, since a token-less Kronk
//     server needs no auth (Copilot's own docs note the key is not required
//     for local providers).
//   - COPILOT_MODEL: the default model, unless the user selected their own via
//     pass-through "--model" (overrideModel), in which case the model-naming
//     vars are cleared so the CLI selection wins (see below).
//   - COPILOT_PROVIDER_MAX_PROMPT_TOKENS / COPILOT_PROVIDER_MAX_OUTPUT_TOKENS:
//     derived from the effective model's resolved context window so Copilot
//     (which would otherwise assume a large window for an unrecognized model
//     name) keeps prompt+output within the server's window instead of
//     overflowing it. Omitted when the window is unknown.
//
// All routing-affecting BYOK keys are pinned explicitly (not left to their
// defaults) so a user's pre-existing BYOK environment — set up for a different
// provider — cannot leak in and divert Copilot away from the local Kronk
// server. Go's exec keeps the last value for duplicate keys, so these override
// any inherited value:
//
//   - COPILOT_PROVIDER_WIRE_API=completions: Kronk serves Chat Completions;
//     an inherited "responses" would post to an endpoint Kronk does not serve
//     for Copilot.
//   - COPILOT_PROVIDER_TRANSPORT=http: the default; blocks an inherited
//     "websockets".
//   - COPILOT_PROVIDER_BEARER_TOKEN: a bearer token takes precedence over the
//     API key in Copilot, so it is pinned to the same value (KRONK_TOKEN, or
//     empty for a token-less server) as COPILOT_PROVIDER_API_KEY. An empty
//     value is falsy, so Copilot falls back to the (also empty) API key and
//     sends no auth; a set value neutralizes any inherited bearer token.
//   - COPILOT_PROVIDER_MODEL_ID / COPILOT_PROVIDER_WIRE_MODEL: both default to
//     COPILOT_MODEL, but an inherited explicit value would override it and send
//     the wrong model name to Kronk, so both are pinned to the default model.
//
// When overrideModel is set (the user passed their own "--model" through), the
// three model-naming vars are set to empty instead. An empty value is falsy for
// Copilot, so it neutralizes any inherited model name yet still lets the model
// passed on Copilot's own command line take effect — without it, pinning the
// launcher default would silently override the user's selection.
func buildCopilotEnv(defaultModel string, chatModels []Model, overrideModel string) ([]string, error) {
	if defaultModel == "" || len(chatModels) == 0 {
		return nil, fmt.Errorf("a default model and at least one model are required")
	}

	baseURL, err := client.DefaultURL("/v1")
	if err != nil {
		return nil, fmt.Errorf("default-url: %w", err)
	}

	token := os.Getenv("KRONK_TOKEN")

	env := []string{
		"COPILOT_PROVIDER_TYPE=openai",
		"COPILOT_PROVIDER_BASE_URL=" + baseURL,
		"COPILOT_PROVIDER_API_KEY=" + token,
		"COPILOT_PROVIDER_BEARER_TOKEN=" + token,
		"COPILOT_PROVIDER_WIRE_API=completions",
		"COPILOT_PROVIDER_TRANSPORT=http",
	}

	// Pin the launcher default only when the user did not select their own
	// model; otherwise clear these so the CLI "--model" wins while still
	// neutralizing any inherited value. The token budgets below are sized for
	// whichever model is effective.
	effectiveModel := defaultModel
	if overrideModel == "" {
		env = append(env,
			"COPILOT_MODEL="+defaultModel,
			"COPILOT_PROVIDER_MODEL_ID="+defaultModel,
			"COPILOT_PROVIDER_WIRE_MODEL="+defaultModel,
		)
	} else {
		effectiveModel = overrideModel
		env = append(env,
			"COPILOT_MODEL=",
			"COPILOT_PROVIDER_MODEL_ID=",
			"COPILOT_PROVIDER_WIRE_MODEL=",
		)
	}

	if cw := contextFor(effectiveModel, chatModels); cw > 0 {
		out := max(cw/copilotOutputReserveFraction, 1)
		prompt := cw - out

		env = append(env,
			"COPILOT_PROVIDER_MAX_PROMPT_TOKENS="+strconv.Itoa(prompt),
			"COPILOT_PROVIDER_MAX_OUTPUT_TOKENS="+strconv.Itoa(out),
		)
	}

	return env, nil
}
