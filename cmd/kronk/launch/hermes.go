package launch

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	yaml "go.yaml.in/yaml/v2"
)

// hermesInstallScript installs Nous Research's Hermes Agent on macOS/Linux.
// "--skip-setup" suppresses Hermes' own interactive provider wizard, since the
// launcher writes the Kronk provider into config.yaml itself.
const hermesInstallScript = "curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash -s -- --skip-setup"

// hermesWindowsInstallURL / hermesWindowsInstallCmd install Hermes on Windows
// via its PowerShell installer, also skipping the setup wizard.
const (
	hermesWindowsInstallURL = "https://hermes-agent.nousresearch.com/install.ps1"
	hermesWindowsInstallCmd = "& ([scriptblock]::Create((irm " + hermesWindowsInstallURL + "))) -SkipSetup"
)

// hermesPlaceholderKey is written as the provider api_key when the Kronk server
// needs no auth. A token-less Kronk server ignores it, and Hermes' "custom"
// provider still reads api_key from config. When a token is required the config
// instead uses "${KRONK_TOKEN}" so Hermes interpolates it from the environment
// at request time (the secret is never persisted to disk).
const hermesPlaceholderKey = "kronk"

// hermesInstall describes how to locate and install Hermes Agent. Hermes is
// distributed via an install script (not npm); the binary lands on PATH, with
// ~/.local/bin as a fallback for the current shell, plus the Windows venv
// script path the installer uses.
var hermesInstall = agentInstall{
	bin:              "hermes",
	display:          "Hermes Agent",
	fallbackDirs:     []string{".local/bin", "AppData/Local/hermes-agent/venv/Scripts"},
	installHint:      hermesInstallHint,
	installerCommand: hermesInstallerCommand,
	checkDeps:        checkHermesInstallDeps,
}

// hermes implements Runner for Nous Research's Hermes Agent. Hermes is a
// personal-assistant platform (CLI plus an optional messaging gateway), but its
// core reads a single primary model from ~/.hermes/config.yaml. To keep the
// launch experience simple and local, Run configures the "custom"
// OpenAI-compatible provider pointing at the local Kronk server (merging and
// backing up any existing config) and then launches Hermes' terminal CLI. No
// messaging gateway is configured.
type hermes struct{}

// Run implements Runner. It ensures Hermes is installed, writes the Kronk
// endpoint and default model into Hermes' config.yaml, and execs Hermes. When
// the caller passes through their own args they are used verbatim; otherwise
// Hermes starts its interactive terminal session against the configured model.
func (hermes) Run(defaultModel string, chatModels []Model, args []string) error {
	bin, err := ensureInstalled(hermesInstall)
	if err != nil {
		return err
	}

	if err := writeHermesConfig(defaultModel, chatModels); err != nil {
		return fmt.Errorf("configure hermes: %w", err)
	}

	cmd := exec.Command(bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// writeHermesConfig writes the Kronk custom-provider settings and default model
// into ~/.hermes/config.yaml, merging into any existing config and backing up
// the previous file first.
func writeHermesConfig(defaultModel string, chatModels []Model) error {
	if defaultModel == "" || len(chatModels) == 0 {
		return fmt.Errorf("a default model and at least one model are required")
	}

	baseURL, err := client.DefaultURL("/v1")
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	apiKey := hermesPlaceholderKey
	if os.Getenv("KRONK_TOKEN") != "" {
		// Hermes interpolates ${ENV} in config.yaml values at request time, so
		// the token is read from the environment (inherited by the launched
		// Hermes process) instead of being written to disk.
		apiKey = "${KRONK_TOKEN}"
	}

	path, err := hermesConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	existing := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		_ = yaml.Unmarshal(data, &existing)
	}

	merged := buildHermesConfig(existing, defaultModel, baseURL, apiKey, contextFor(defaultModel, chatModels))

	data, err := yaml.Marshal(merged)
	if err != nil {
		return err
	}

	return writeFileWithBackup(path, data)
}

// buildHermesConfig merges the Kronk custom-provider settings into an existing
// Hermes config document. Only the managed keys under "model" are changed
// (provider, default, base_url, api_key, and context_length when known); any
// other model settings and all other top-level sections are preserved.
//
// Hermes' single source of truth for the active model is the top-level "model"
// block; with provider "custom" it calls base_url directly using api_key, which
// is the documented, unambiguous path for an OpenAI-compatible endpoint.
func buildHermesConfig(existing map[string]any, defaultModel, baseURL, apiKey string, contextLen int) map[string]any {
	config := existing
	if config == nil {
		config = map[string]any{}
	}

	model := hermesStringMap(config["model"])
	model["provider"] = "custom"
	model["default"] = defaultModel
	model["base_url"] = baseURL
	model["api_key"] = apiKey
	if contextLen > 0 {
		model["context_length"] = contextLen
	}

	config["model"] = model

	return config
}

// hermesStringMap coerces a config subsection into a string-keyed map. YAML v2
// decodes nested maps as map[any]any, so both shapes are handled; a nil or
// unexpected value yields an empty map.
func hermesStringMap(v any) map[string]any {
	switch m := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(m))
		maps.Copy(out, m)
		return out
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			if ks, ok := k.(string); ok {
				out[ks] = val
			}
		}
		return out
	default:
		return map[string]any{}
	}
}

// hermesConfigPath returns the path to Hermes' config.yaml, honoring the
// HERMES_HOME override the installer supports and otherwise defaulting to
// ~/.hermes/config.yaml.
func hermesConfigPath() (string, error) {
	if home := os.Getenv("HERMES_HOME"); home != "" {
		return filepath.Join(home, "config.yaml"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".hermes", "config.yaml"), nil
}

// checkHermesInstallDeps verifies the tools the Hermes install script needs are
// present on macOS/Linux (bash, curl, git).
func checkHermesInstallDeps(goos string) error {
	switch goos {
	case "windows":
		return nil
	case "darwin", "linux":
		var missing []string
		for _, dep := range []string{"bash", "curl", "git"} {
			if _, err := exec.LookPath(dep); err != nil {
				missing = append(missing, dep)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("hermes is not installed and required tools are missing: %v\n\ninstall them first, then re-run: kronk launch hermes", missing)
		}
		return nil
	default:
		return fmt.Errorf("hermes is not installed and automatic install is not supported on %s\n\ninstall it manually: %s", goos, hermesInstallHint(goos))
	}
}

// hermesInstallHint returns the human-readable install command for the given
// OS.
func hermesInstallHint(goos string) string {
	switch goos {
	case "windows":
		return hermesWindowsInstallCmd
	case "darwin", "linux":
		return hermesInstallScript
	default:
		return "see https://hermes-agent.nousresearch.com for installation instructions"
	}
}

// hermesInstallerCommand returns the command that installs Hermes on the given
// OS via its official install script.
func hermesInstallerCommand(goos string) (string, []string, error) {
	switch goos {
	case "windows":
		return "powershell.exe", []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", hermesWindowsInstallCmd}, nil
	case "darwin", "linux":
		return "bash", []string{"-lc", hermesInstallScript}, nil
	default:
		return "", nil, fmt.Errorf("unsupported platform for hermes install: %s", goos)
	}
}
