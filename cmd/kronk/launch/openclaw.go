package launch

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
)

// openclawInstallCmd is the npm command that installs OpenClaw. OpenClaw is
// distributed via npm on every platform, so the same command works on Windows,
// macOS, and Linux.
const openclawInstallCmd = "npm install -g openclaw"

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

// openclawInstall describes how to locate and install OpenClaw. The npm global
// install lands on PATH; the install-script variant can drop the binary in
// ~/.local/bin, which may not be on PATH in the current shell yet, so that
// directory is searched as a fallback.
var openclawInstall = agentInstall{
	bin:              "openclaw",
	display:          "OpenClaw",
	fallbackDirs:     []string{".local/bin"},
	installHint:      openclawInstallHint,
	installerCommand: openclawInstallerCommand,
	checkDeps:        checkOpenClawInstallDeps,
}

// openClaw implements Runner for OpenClaw. Unlike a plain coding CLI, OpenClaw
// is a personal-assistant platform with a gateway daemon, channels, and a web
// UI. To keep the launch experience simple and local, Run configures a custom
// Kronk provider in ~/.openclaw/openclaw.json (merging and backing up any
// existing config) and then launches OpenClaw's local embedded TUI with
// "openclaw chat" (an alias for "openclaw tui --local"), which talks directly
// to the local Kronk server without starting a background gateway.
type openClaw struct{}

// Run implements Runner. It ensures OpenClaw is installed, writes the Kronk
// provider and default model into OpenClaw's config, and execs OpenClaw's
// local TUI. When the caller passes through their own args they are used
// verbatim; otherwise the launcher defaults to "chat" so the session runs
// against the local embedded runtime (no gateway daemon).
func (openClaw) Run(defaultModel string, chatModels []Model, args []string) error {
	bin, err := ensureInstalled(openclawInstall)
	if err != nil {
		return err
	}

	if err := writeOpenClawConfig(defaultModel, chatModels); err != nil {
		return fmt.Errorf("configure openclaw: %w", err)
	}

	// Default to the local embedded TUI ("chat" == "tui --local"), which runs
	// against the local Kronk server without a gateway daemon. If the user
	// passed their own args through, run them verbatim instead.
	oclArgs := args
	if len(args) == 0 {
		oclArgs = []string{"chat"}
	}

	cmd := exec.Command(bin, oclArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// writeOpenClawConfig writes the Kronk provider and default model into
// ~/.openclaw/openclaw.json, merging into any existing config and backing up
// the previous file first.
func writeOpenClawConfig(defaultModel string, chatModels []Model) error {
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

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	path := filepath.Join(home, ".openclaw", "openclaw.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	existing := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &existing)
	}

	merged := buildOpenClawConfig(existing, chatModels, defaultModel, baseURL, apiKey)

	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}

	return writeFileWithBackup(path, data)
}

// buildOpenClawConfig merges the Kronk provider and default model into an
// existing OpenClaw config document. The launcher fully owns the "kronk"
// provider (it is rebuilt each launch so a moved server or changed model set is
// corrected), while other providers the user configured are left untouched. In
// the agent allowlist only "kronk/*" entries are managed; other allowlist
// entries are preserved.
//
// OpenClaw requires both a provider definition (models.providers.kronk) and an
// allowlist entry (agents.defaults.models["kronk/<id>"]) before a model can be
// used, so both are written here.
func buildOpenClawConfig(existing map[string]any, chatModels []Model, defaultModel, baseURL, apiKey string) map[string]any {
	config := existing
	if config == nil {
		config = map[string]any{}
	}

	// models.providers.kronk — the launcher owns this provider entirely.
	models, _ := config["models"].(map[string]any)
	if models == nil {
		models = map[string]any{}
	}

	providers, _ := models["providers"].(map[string]any)
	if providers == nil {
		providers = map[string]any{}
	}

	modelEntries := make([]any, 0, len(chatModels))
	for _, cm := range chatModels {
		modelEntries = append(modelEntries, openClawModelEntry(cm))
	}

	providers[openclawProvider] = map[string]any{
		"baseUrl": baseURL,
		"apiKey":  apiKey,
		"api":     "openai-completions",
		"models":  modelEntries,
	}

	models["providers"] = providers
	config["models"] = models

	// agents.defaults.model.primary and the agent allowlist.
	agents, _ := config["agents"].(map[string]any)
	if agents == nil {
		agents = map[string]any{}
	}

	defaults, _ := agents["defaults"].(map[string]any)
	if defaults == nil {
		defaults = map[string]any{}
	}

	model, _ := defaults["model"].(map[string]any)
	if model == nil {
		model = map[string]any{}
	}
	model["primary"] = openclawRef(defaultModel)
	defaults["model"] = model

	allow, _ := defaults["models"].(map[string]any)
	if allow == nil {
		allow = map[string]any{}
	}

	// Drop our previously-managed allowlist entries so the list reflects
	// exactly what is installed now; leave the user's own entries alone.
	for k := range allow {
		if strings.HasPrefix(k, openclawProvider+"/") {
			delete(allow, k)
		}
	}
	for _, cm := range chatModels {
		allow[openclawRef(cm.ID)] = map[string]any{}
	}
	defaults["models"] = allow

	agents["defaults"] = defaults
	config["agents"] = agents

	return config
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

// checkOpenClawInstallDeps verifies npm (Node.js) is available, since OpenClaw
// is installed via npm on every platform.
func checkOpenClawInstallDeps(goos string) error {
	switch goos {
	case "windows", "darwin", "linux":
		if _, err := exec.LookPath("npm"); err != nil {
			return fmt.Errorf("openclaw is not installed and npm (Node.js) is required to install it: https://nodejs.org/\n\nthen re-run: kronk launch openclaw")
		}
	default:
		return fmt.Errorf("openclaw is not installed and automatic install is not supported on %s\n\ninstall it manually: %s", goos, openclawInstallHint(goos))
	}

	return nil
}

// openclawInstallHint returns the human-readable install command for the given
// OS.
func openclawInstallHint(goos string) string {
	switch goos {
	case "windows", "darwin", "linux":
		return openclawInstallCmd
	default:
		return "see https://openclaw.ai for installation instructions"
	}
}

// openclawInstallerCommand returns the command that installs OpenClaw on the
// given OS via npm.
func openclawInstallerCommand(goos string) (string, []string, error) {
	switch goos {
	case "windows", "darwin", "linux":
		return "npm", []string{"install", "-g", "openclaw"}, nil
	default:
		return "", nil, fmt.Errorf("unsupported platform for openclaw install: %s", goos)
	}
}
