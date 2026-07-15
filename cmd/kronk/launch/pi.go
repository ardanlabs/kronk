package launch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
)

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

// pi implements Runner for the Pi coding agent. Unlike the other agents Pi has
// no environment variable for its model provider; it is configured only
// through its models.json (under PI_CODING_AGENT_DIR when set, otherwise
// ~/.pi/agent). So Run writes/merges that file (with a backup) before
// launching, pointing Pi at Kronk's OpenAI-compatible Chat Completions API at
// /v1/chat/completions.
type pi struct{}

// Run implements Runner. It ensures Pi is installed, writes the Kronk provider
// into Pi's models.json, and execs Pi with the default model selected on the
// command line (so the user's saved Pi defaults are left unchanged). The
// selected model is provider-qualified ("kronk/<id>") so Pi resolves it to the
// Kronk provider even when another configured provider exposes the same id.
func (pi) Run(defaultModel string, chatModels []Model, args []string) error {
	install, err := loadInstall("pi")
	if err != nil {
		return fmt.Errorf("pi: %w", err)
	}

	bin, err := ensureInstalled(install)
	if err != nil {
		return err
	}

	if err := writePiConfig(chatModels); err != nil {
		return fmt.Errorf("configure pi: %w", err)
	}

	// Select the model for this session on the command line rather than
	// mutating the user's saved defaults in settings.json. If the user passed
	// their own model selector through, leave it alone. The id is
	// provider-qualified so Pi picks the Kronk provider rather than a bare-id
	// match under some other configured provider.
	piArgs := args
	if !hasModelArg(args) {
		piArgs = append([]string{"--model", piProvider + "/" + defaultModel}, args...)
	}

	cmd := exec.Command(bin, piArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// piModelsPath returns the path to Pi's models.json. Pi resolves its config
// directory from PI_CODING_AGENT_DIR when that env var is set (with leading "~"
// expanded to the user's home), otherwise it uses ~/.pi/agent. The launcher
// must write to the same file Pi reads, or the Kronk provider would land in a
// file Pi ignores.
func piModelsPath() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR")); dir != "" {
		return filepath.Join(expandUserPath(dir), "models.json"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".pi", "agent", "models.json"), nil
}

// expandUserPath expands a leading "~" (alone or as a "~/" prefix) to the
// user's home directory, matching how Pi interprets its configured directory.
// Any other path is returned unchanged. If the home directory cannot be
// resolved the original path is returned so the caller still gets a usable path.
func expandUserPath(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			if p == "~" {
				return home
			}
			return filepath.Join(home, p[2:])
		}
	}

	return p
}

// writePiConfig writes the Kronk provider and its installed chat models into
// Pi's models.json (see piModelsPath), merging into any existing config and
// backing up the previous file first.
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

	path, err := piModelsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	existing := map[string]any{}
	if err := readExistingConfig(path, &existing, unmarshalJSONC); err != nil {
		return err
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

// modelArgValue returns the model selector value from the pass-through args, or
// "" when none is present. It understands "--model X", "-m X", "--model=X", and
// "-m=X". It lets a runner honor a user's own model selection instead of pinning
// the launcher's default.
func modelArgValue(args []string) string {
	for i, a := range args {
		switch {
		case a == "--model" || a == "-m":
			if i+1 < len(args) {
				return args[i+1]
			}
		case strings.HasPrefix(a, "--model="):
			return strings.TrimPrefix(a, "--model=")
		case strings.HasPrefix(a, "-m="):
			return strings.TrimPrefix(a, "-m=")
		}
	}

	return ""
}

// jsonLineCommentRE matches a JSON string literal or a "//" line comment, so a
// comment can be stripped while string contents (which may contain "//") are
// preserved.
var jsonLineCommentRE = regexp.MustCompile(`"(?:\\.|[^"\\])*"|//[^\n]*`)

// jsonTrailingCommaRE matches a JSON string literal or a trailing comma before a
// closing "}" or "]", so the comma can be dropped without disturbing string
// contents.
var jsonTrailingCommaRE = regexp.MustCompile(`"(?:\\.|[^"\\])*"|,(\s*[}\]])`)

// unmarshalJSONC parses the JSONC subset Pi accepts in models.json: standard
// JSON plus "//" line comments and trailing commas before "}" or "]". Pi strips
// exactly these before parsing, so a config Pi reads without complaint must not
// be rejected by the launcher's stricter parser. Anything still invalid after
// stripping is a real parse error and surfaces as before.
func unmarshalJSONC(data []byte, v any) error {
	return json.Unmarshal(stripJSONComments(data), v)
}

// stripJSONComments removes "//" line comments and trailing commas (before a
// closing "}" or "]") from JSON, matching the tolerance Pi applies to
// models.json. Content inside string literals is left untouched. Block comments
// ("/* */") are not handled, which also matches Pi.
func stripJSONComments(data []byte) []byte {
	s := jsonLineCommentRE.ReplaceAllStringFunc(string(data), func(m string) string {
		if m[0] == '"' {
			return m
		}
		return ""
	})

	s = jsonTrailingCommaRE.ReplaceAllStringFunc(s, func(m string) string {
		if m[0] == '"' {
			return m
		}
		// The match is ",<space><bracket>"; drop the leading comma and keep the
		// whitespace and closing bracket.
		return m[1:]
	})

	return []byte(s)
}

// readExistingConfig reads and parses an existing agent config file into dst
// using unmarshal. A missing or empty file is not an error (dst is left as-is).
// A file that exists with content but cannot be parsed is a hard error, so a
// user's hand-edited config is never silently discarded and overwritten by the
// launcher's merge-and-backup path.
func readExistingConfig(path string, dst any, unmarshal func([]byte, any) error) error {
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("read existing %s: %w", path, err)
	}

	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}

	if err := unmarshal(data, dst); err != nil {
		return fmt.Errorf("parse existing %s (fix or remove it, then re-run): %w", path, err)
	}

	return nil
}

// writeFileWithBackup writes data to path atomically, preserving the user's
// original config as "<path>.bak" the first time it replaces an existing file.
//
// The backup is created only when no "<path>.bak" exists yet, so re-launching
// never clobbers the pristine original with an already-launcher-modified
// version (the whole point of the backup is to be able to restore what the user
// had before Kronk touched it). These config files can carry the user's other
// provider keys, so both the backup and the written file use 0600 rather than a
// world-readable mode.
//
// The write goes to a temp file in the same directory and is then renamed over
// the target. os.Rename is atomic on the same filesystem, so a reader (or the
// launched agent) never sees a half-written config and a crash mid-write leaves
// the previous file intact. Concurrent launches are not locked; the last rename
// wins, which is acceptable because each writes the same managed provider —
// atomicity prevents the serious failure (a corrupted/truncated config).
func writeFileWithBackup(path string, data []byte) error {
	if existing, err := os.ReadFile(path); err == nil {
		bak := path + ".bak"
		if _, statErr := os.Stat(bak); errors.Is(statErr, os.ErrNotExist) {
			if err := os.WriteFile(bak, existing, 0o600); err != nil {
				return fmt.Errorf("back up %s: %w", path, err)
			}
		}
	}

	// os.CreateTemp creates the file with mode 0600, which is preserved across
	// the rename.
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("write %s: tempfile: %w", path, err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write %s: %w", path, err)
	}

	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("write %s: close: %w", path, err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("write %s: rename: %w", path, err)
	}

	return nil
}
