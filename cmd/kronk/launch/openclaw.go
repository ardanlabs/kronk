package launch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
)

// openclawProvider is the provider id written into OpenClaw's config for the
// local Kronk server. OpenClaw refers to a model as "<provider>/<id>", so the
// fully-qualified refs the launcher writes are "kronk/<model-id>".
const openclawProvider = "kronk"

// openclawPlaceholderKey is written as the provider apiKey when the Kronk
// server needs no auth. A token-less Kronk server ignores it, and OpenClaw
// still requires a value for a custom provider. When a token is required the
// config instead uses "${KRONK_TOKEN}" so OpenClaw interpolates it from the
// environment at request time (the secret is never persisted to disk).
const openclawPlaceholderKey = "kronk"

// openclawCmdTimeout bounds the "openclaw config get/patch" subprocess calls so
// a wedged OpenClaw CLI cannot hang the launch. It is generous (these are quick
// file operations) so it never fires on a legitimately slow-but-working CLI.
const openclawCmdTimeout = 30 * time.Second

// openclawSessionKey is the dedicated session the launcher runs its default
// chat TUI under. OpenClaw persists a per-session model pin (modelProvider +
// model) that outranks agents.defaults.model.primary, so resuming the user's
// default "main" session could keep a stale — or cloud — model instead of the
// Kronk model launch just configured. Running under a dedicated key isolates
// launch from the user's own sessions while still resuming across launches; the
// first use has no stored pin and so adopts the configured primary.
const openclawSessionKey = "kronk"

// openClaw implements Runner for OpenClaw. Unlike a plain coding CLI, OpenClaw
// is a personal-assistant platform with a gateway daemon, channels, and a web
// UI. To keep the launch experience simple and local, Run configures a custom
// Kronk provider in OpenClaw's config and then launches OpenClaw's local
// embedded TUI with "openclaw chat" (an alias for "openclaw tui --local"),
// which talks directly to the local Kronk server without starting a background
// gateway.
type openClaw struct{}

// Run implements Runner. It ensures OpenClaw is installed, writes the Kronk
// provider and default model into OpenClaw's config, and execs OpenClaw's
// local TUI. When the caller passes through their own args they are used
// verbatim; otherwise the launcher defaults to "chat" so the session runs
// against the local embedded runtime (no gateway daemon).
func (openClaw) Run(defaultModel string, chatModels []Model, args []string) error {
	install, err := loadInstall("openclaw")
	if err != nil {
		return fmt.Errorf("openclaw: %w", err)
	}

	bin, err := ensureInstalled(install)
	if err != nil {
		return err
	}

	// OpenClaw's --profile/--dev global options select which config/state file
	// it reads. Extract them from the pass-through args so the config we write
	// lands in the same file the launched session will read (writing to the
	// default profile while the session ran under another would leave the
	// provider missing at runtime).
	scope, err := openclawScopeArgs(args)
	if err != nil {
		return fmt.Errorf("openclaw: %w", err)
	}

	if err := writeOpenClawConfig(bin, scope, defaultModel, chatModels); err != nil {
		return fmt.Errorf("configure openclaw: %w", err)
	}

	// Default to the local embedded TUI ("chat" == "tui --local"), which runs
	// against the local Kronk server without a gateway daemon.
	oclArgs := openclawRunArgs(args, scope)

	cmd := exec.Command(bin, oclArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = openclawEnv()

	return cmd.Run()
}

// openclawEnv returns the environment for OpenClaw subprocesses with
// OPENCLAW_CONTAINER removed. OpenClaw uses OPENCLAW_CONTAINER as the default
// value of --container, which would run OpenClaw (and the launcher's own "config
// get"/"config patch" calls) inside a container where the generated
// "http://localhost:11435" provider URL points at the container rather than the
// host's Kronk server. launch cannot wire that up correctly and already rejects
// an explicit --container, so the inherited env form is neutralized the same
// way. Every other variable is inherited unchanged.
func openclawEnv() []string {
	var out []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "OPENCLAW_CONTAINER=") {
			continue
		}
		out = append(out, kv)
	}

	return out
}

// openclawScopeArgs extracts the OpenClaw global options that select which
// config/state file OpenClaw uses (--profile <name> and --dev) from the
// pass-through args, so the launcher's "config get"/"config patch" calls act on
// the same file the launched session reads. Returned in the order OpenClaw
// expects them (before the subcommand).
//
// --container is rejected: it runs OpenClaw inside a container, where the
// generated "http://localhost:11435" provider URL points at the container
// rather than the host's Kronk server, so launch cannot wire it up correctly.
func openclawScopeArgs(args []string) ([]string, error) {
	var scope []string

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--container" || strings.HasPrefix(a, "--container="):
			return nil, errors.New("--container is not supported by kronk launch openclaw: the agent would run inside the container and could not reach the local Kronk server at the host's localhost address")

		case a == "--dev":
			scope = append(scope, "--dev")

		case a == "--profile":
			if i+1 >= len(args) {
				return nil, errors.New("--profile requires a name")
			}
			scope = append(scope, "--profile", args[i+1])
			i++

		case strings.HasPrefix(a, "--profile="):
			scope = append(scope, a)
		}
	}

	return scope, nil
}

// openclawRunArgs returns the arguments to exec OpenClaw with:
//
//   - No passthrough args: default to the local chat TUI ("chat") under the
//     dedicated launch session (see openclawSessionKey).
//   - Only scope flags (--profile/--dev) and no subcommand: append "chat"
//     (again under the launch session) so the TUI still launches under that
//     profile. Because a profile can only be selected via passthrough, a bare
//     "-- --profile x" must not drop the subcommand and leave OpenClaw with
//     nothing to run.
//   - Anything else (a subcommand, --help, or an unrecognized flag): used
//     verbatim, so the caller stays in control — including their own --session.
//
// The "--session" is added only when the launcher itself supplies "chat"; a
// caller who provides their own command keeps full control of the session. It
// follows "chat" because --session is a TUI subcommand option, and "chat"
// follows any leading scope flags because global options must precede the
// subcommand.
//
// scope is the slice openclawScopeArgs consumed; when it consumed every
// passthrough arg, the passthrough was only scope flags.
func openclawRunArgs(args, scope []string) []string {
	if len(args) == 0 {
		return []string{"chat", "--session", openclawSessionKey}
	}

	if len(scope) == len(args) {
		return append(append([]string{}, args...), "chat", "--session", openclawSessionKey)
	}

	return args
}

// writeOpenClawConfig writes the Kronk provider and default model into
// OpenClaw's config using OpenClaw's own "config patch" command rather than
// editing the JSON file directly. Delegating to OpenClaw is what makes this
// robust:
//
//   - It reads and merges the user's existing config in OpenClaw's own format
//     (JSON5, i.e. comments and trailing commas are allowed), which a strict
//     Go JSON parser would reject.
//   - It writes to whatever config file OpenClaw actually reads, honoring
//     OPENCLAW_CONFIG_PATH (inherited by the subprocess) and any profile/dev
//     flags the user passed, so a relocated config is never silently missed.
//   - The write is validated and atomic, so a half-written or schema-invalid
//     config is never left behind.
//
// The launcher fully owns the "kronk" provider (--replace-path replaces that
// subtree so a removed model or moved server is corrected), while other
// providers and allowlist entries the user configured are merged untouched.
// Previously-managed "kronk/*" allowlist entries that are no longer installed
// are pruned via null-deletes so the allowlist reflects exactly the current
// models.
func writeOpenClawConfig(bin string, scope []string, defaultModel string, chatModels []Model) error {
	if defaultModel == "" || len(chatModels) == 0 {
		return fmt.Errorf("a default model and at least one model are required")
	}

	baseURL, err := client.DefaultURL("/v1")
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	apiKey := openclawPlaceholderKey
	if os.Getenv("KRONK_TOKEN") != "" {
		// OpenClaw interpolates ${ENV} in config values at request time, so the
		// token is read from the environment (inherited by the launched
		// OpenClaw process) instead of being written to disk.
		apiKey = "${KRONK_TOKEN}"
	}

	staleRefs := staleOpenClawRefs(bin, scope, chatModels)

	patch := buildOpenClawPatch(defaultModel, chatModels, baseURL, apiKey, staleRefs)

	data, err := json.Marshal(patch)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), openclawCmdTimeout)
	defer cancel()

	// --replace-path makes OpenClaw replace the whole kronk provider subtree
	// instead of merging into it, so the launcher owns that provider entirely
	// while everything else in the user's config is left as-is. scope carries
	// the profile/dev selection so the patch targets the same config file the
	// launched session reads; global options must precede the subcommand.
	patchArgs := append(append([]string{}, scope...), "config", "patch", "--stdin", "--replace-path", "models.providers."+openclawProvider)
	cmd := exec.CommandContext(ctx, bin, patchArgs...)
	cmd.Stdin = bytes.NewReader(data)
	cmd.Env = openclawEnv()

	if out, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("openclaw config patch timed out after %s; the openclaw CLI did not respond", openclawCmdTimeout)
		}
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return fmt.Errorf("openclaw config patch: %w", err)
		}
		return fmt.Errorf("openclaw config patch failed: %s", msg)
	}

	return nil
}

// staleOpenClawRefs returns the launcher-managed "kronk/*" allowlist refs
// currently in OpenClaw's config that are no longer installed, so they can be
// pruned. It reads them via "openclaw config get" (whose output is strict JSON
// and preserves our refs faithfully — only the already-lowercase "kronk"
// provider segment is normalized), so a JSON5 config is never parsed directly.
// Any error (missing path on a fresh config, unreadable config) yields no refs,
// which just means nothing is pruned.
func staleOpenClawRefs(bin string, scope []string, chatModels []Model) []string {
	ctx, cancel := context.WithTimeout(context.Background(), openclawCmdTimeout)
	defer cancel()

	getArgs := append(append([]string{}, scope...), "config", "get", "agents.defaults.models")
	cmd := exec.CommandContext(ctx, bin, getArgs...)
	cmd.Env = openclawEnv()

	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var allow map[string]any
	if err := json.Unmarshal(out, &allow); err != nil {
		return nil
	}

	current := make(map[string]bool, len(chatModels))
	for _, cm := range chatModels {
		current[openclawRef(cm.ID)] = true
	}

	var stale []string
	for ref := range allow {
		if strings.HasPrefix(ref, openclawProvider+"/") && !current[ref] {
			stale = append(stale, ref)
		}
	}

	return stale
}

// buildOpenClawPatch builds the "config patch" document for OpenClaw. It sets
// the launcher-owned kronk provider, the default primary model, and the agent
// allowlist entries for the installed models. staleRefs are previously-managed
// "kronk/*" allowlist entries to prune; each is set to null, which tells
// "config patch" to delete that path. Objects merge recursively into the
// user's config, so only these paths are affected.
//
// OpenClaw requires both a provider definition (models.providers.kronk) and an
// allowlist entry (agents.defaults.models["kronk/<id>"]) before a model can be
// used, so both are written here.
func buildOpenClawPatch(defaultModel string, chatModels []Model, baseURL, apiKey string, staleRefs []string) map[string]any {
	modelEntries := make([]any, 0, len(chatModels))
	allow := make(map[string]any, len(chatModels)+len(staleRefs))
	for _, cm := range chatModels {
		modelEntries = append(modelEntries, openClawModelEntry(cm))
		allow[openclawRef(cm.ID)] = map[string]any{}
	}

	// A null value tells "config patch" to delete that allowlist path. Skip any
	// stale ref that is also a current model (never delete what we just added).
	for _, ref := range staleRefs {
		if _, current := allow[ref]; !current {
			allow[ref] = nil
		}
	}

	return map[string]any{
		"models": map[string]any{
			"providers": map[string]any{
				openclawProvider: map[string]any{
					"baseUrl": baseURL,
					"apiKey":  apiKey,
					"api":     "openai-completions",
					"models":  modelEntries,
				},
			},
		},
		"agents": map[string]any{
			"defaults": map[string]any{
				"model": map[string]any{
					"primary": openclawRef(defaultModel),
				},
				"models": allow,
			},
		},
	}
}

// openclawRef returns the OpenClaw model ref for a Kronk model id (its
// provider-qualified name, e.g. "kronk/Qwen3-8B-Q8_0").
func openclawRef(id string) string {
	return openclawProvider + "/" + id
}

// openClawModelEntry builds one OpenClaw provider model entry for a Kronk
// model. For a custom provider the extra fields are optional; the resolved
// context window is forwarded when known so OpenClaw sizes prompts to the
// server's limit instead of overflowing it.
func openClawModelEntry(m Model) map[string]any {
	name := m.Name
	if name == "" {
		name = m.ID
	}

	entry := map[string]any{
		"id":   m.ID,
		"name": name,
	}

	if m.Vision {
		entry["input"] = []any{"text", "image"}
	} else {
		entry["input"] = []any{"text"}
	}

	if m.Reasoning {
		entry["reasoning"] = true
	}

	if m.Context > 0 {
		entry["contextWindow"] = m.Context
	}

	return entry
}
