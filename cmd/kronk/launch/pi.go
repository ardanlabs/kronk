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

// piInstallCmd is the npm command that installs the Pi coding agent. Pi is
// distributed via npm on every platform, so the same command works on
// Windows, macOS, and Linux.
const piInstallCmd = "npm install -g @earendil-works/pi-coding-agent"

// piProvider is the provider id written into Pi's models.json for the local
// Kronk server.
const piProvider = "kronk"

// piPlaceholderKey is written as the provider apiKey when the Kronk server
// needs no auth. Pi treats keyless local servers as still requiring a value
// before a model appears in its picker, and a token-less Kronk server ignores
// it. When a token is required the config instead uses "$KRONK_TOKEN" so Pi
// interpolates it from the environment at request time (the secret is never
// persisted to disk).
const piPlaceholderKey = "kronk"

// piLaunchMarker flags model entries in Pi's models.json that this launcher
// manages. Re-launching refreshes only the marked entries, so models the user
// added under the same provider are preserved untouched.
const piLaunchMarker = "_launch"

// piInstall describes how to locate and install the Pi coding agent. The npm
// global install lands on PATH; the install-script variant drops the binary in
// ~/.local/bin, which may not be on PATH in the current shell yet, so that
// directory is searched as a fallback.
var piInstall = agentInstall{
	bin:              "pi",
	display:          "Pi",
	fallbackDirs:     []string{".local/bin"},
	installHint:      piInstallHint,
	installerCommand: piInstallerCommand,
	checkDeps:        checkPiInstallDeps,
}

// pi implements Runner for the Pi coding agent. Unlike the other agents Pi has
// no environment variable for its model provider; it is configured only
// through ~/.pi/agent/models.json. So Run writes/merges that file (with a
// backup) before launching, pointing Pi at Kronk's OpenAI-compatible Chat
// Completions API at /v1/chat/completions.
type pi struct{}

// Run implements Runner. It ensures Pi is installed, writes the Kronk provider
// into Pi's models.json, and execs Pi with the default model selected on the
// command line (so the user's saved Pi defaults are left unchanged).
func (pi) Run(defaultModel string, chatModels []Model, args []string) error {
	bin, err := ensureInstalled(piInstall)
	if err != nil {
		return err
	}

	if err := writePiConfig(chatModels); err != nil {
		return fmt.Errorf("configure pi: %w", err)
	}

	// Select the model for this session on the command line rather than
	// mutating the user's saved defaults in settings.json. If the user passed
	// their own model selector through, leave it alone.
	piArgs := args
	if !hasModelArg(args) {
		piArgs = append([]string{"--model", defaultModel}, args...)
	}

	cmd := exec.Command(bin, piArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// writePiConfig writes the Kronk provider and its installed chat models into
// ~/.pi/agent/models.json, merging into any existing config and backing up the
// previous file first.
func writePiConfig(chatModels []Model) error {
	if len(chatModels) == 0 {
		return fmt.Errorf("at least one model is required")
	}

	baseURL, err := client.DefaultURL("/v1")
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	apiKey := piPlaceholderKey
	if os.Getenv("KRONK_TOKEN") != "" {
		// Pi interpolates $ENV at request time, so the token is read from the
		// environment (inherited by the launched Pi process) instead of being
		// written to disk.
		apiKey = "$KRONK_TOKEN"
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	path := filepath.Join(home, ".pi", "agent", "models.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	existing := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &existing)
	}

	merged := buildPiConfig(existing, chatModels, baseURL, apiKey)

	data, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}

	return writeFileWithBackup(path, data)
}

// buildPiConfig merges the Kronk provider and models into an existing Pi
// models.json document. The Kronk provider is (re)pointed at baseURL/apiKey so
// a moved server is corrected on the next launch; user-added models under the
// provider (those without the launch marker) are preserved, while previously
// managed models are replaced by the current set.
func buildPiConfig(existing map[string]any, chatModels []Model, baseURL, apiKey string) map[string]any {
	config := existing
	if config == nil {
		config = map[string]any{}
	}

	providers, _ := config["providers"].(map[string]any)
	if providers == nil {
		providers = map[string]any{}
	}

	provider, _ := providers[piProvider].(map[string]any)
	if provider == nil {
		provider = map[string]any{}
	}

	provider["baseUrl"] = baseURL
	provider["api"] = "openai-completions"
	provider["apiKey"] = apiKey

	// Preserve user-managed models; drop our previously-managed ones so the
	// list reflects exactly what is installed now.
	existingModels, _ := provider["models"].([]any)
	var models []any
	for _, m := range existingModels {
		mo, ok := m.(map[string]any)
		if !ok {
			continue
		}
		if isPiLaunchModel(mo) {
			continue
		}
		models = append(models, mo)
	}

	for _, cm := range chatModels {
		models = append(models, piModelEntry(cm))
	}

	provider["models"] = models
	providers[piProvider] = provider
	config["providers"] = providers

	return config
}

// piModelEntry builds one Pi model config entry for a Kronk model, tagged with
// the launch marker so it can be refreshed or removed on later launches.
func piModelEntry(m Model) map[string]any {
	entry := map[string]any{
		"id":           m.ID,
		piLaunchMarker: true,
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

// isPiLaunchModel reports whether a model config entry is one this launcher
// manages.
func isPiLaunchModel(cfg map[string]any) bool {
	v, ok := cfg[piLaunchMarker].(bool)
	return ok && v
}

// hasModelArg reports whether the pass-through args already select a model, so
// the launcher does not add a conflicting --model of its own.
func hasModelArg(args []string) bool {
	for _, a := range args {
		if a == "--model" || a == "-m" ||
			strings.HasPrefix(a, "--model=") || strings.HasPrefix(a, "-m=") {
			return true
		}
	}

	return false
}

// writeFileWithBackup writes data to path, first copying any existing file to
// "<path>.bak" so a previous config can be restored.
func writeFileWithBackup(path string, data []byte) error {
	if existing, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".bak", existing, 0o644); err != nil {
			return fmt.Errorf("back up %s: %w", path, err)
		}
	}

	return os.WriteFile(path, data, 0o644)
}

// checkPiInstallDeps verifies npm (Node.js) is available, since Pi is installed
// via npm on every platform.
func checkPiInstallDeps(goos string) error {
	switch goos {
	case "windows", "darwin", "linux":
		if _, err := exec.LookPath("npm"); err != nil {
			return fmt.Errorf("pi is not installed and npm (Node.js) is required to install it: https://nodejs.org/\n\nthen re-run: kronk launch pi")
		}
	default:
		return fmt.Errorf("pi is not installed and automatic install is not supported on %s\n\ninstall it manually: %s", goos, piInstallHint(goos))
	}

	return nil
}

// piInstallHint returns the human-readable install command for the given OS.
func piInstallHint(goos string) string {
	switch goos {
	case "windows", "darwin", "linux":
		return piInstallCmd
	default:
		return "see https://pi.dev for installation instructions"
	}
}

// piInstallerCommand returns the command that installs Pi on the given OS via
// npm.
func piInstallerCommand(goos string) (string, []string, error) {
	switch goos {
	case "windows", "darwin", "linux":
		return "npm", []string{"install", "-g", "@earendil-works/pi-coding-agent"}, nil
	default:
		return "", nil, fmt.Errorf("unsupported platform for pi install: %s", goos)
	}
}
