package launch

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	agentdefaults "github.com/ardanlabs/kronk/.agents/default"
)

// launchOpenCode creates an isolated OpenCode environment, runs OpenCode in
// it, and removes the environment after OpenCode exits.
func launchOpenCode(bin string, args, availableModels []string, serverURL string, signals <-chan os.Signal) error {
	fmt.Fprintln(os.Stderr, "[3/4] Preparing an isolated temporary workspace and configuration...")
	root, err := os.MkdirTemp("", "kronk-opencode-")
	if err != nil {
		return fmt.Errorf("creating temporary OpenCode environment: %w", err)
	}
	fmt.Fprintf(os.Stderr, "      Temporary environment: %s\n", root)
	fmt.Fprintln(os.Stderr, "      It will be removed when OpenCode exits.")
	defer func() {
		if err := os.RemoveAll(root); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: unable to remove temporary OpenCode environment %s: %v\n", root, err)
			return
		}
		fmt.Fprintf(os.Stderr, "Removed temporary OpenCode environment: %s\n", root)
	}()

	workspace, env, err := prepareOpenCode(root, os.Environ(), availableModels, serverURL)
	if err != nil {
		return err
	}

	cmd := exec.Command(bin, args...)
	cmd.Dir = workspace
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Fprintln(os.Stderr, "      Existing OpenCode configuration will not be changed.")
	fmt.Fprintln(os.Stderr, "[4/4] Starting OpenCode (the terminal may clear while its interface initializes)...")
	if isInteractive() {
		time.Sleep(time.Second)
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		return err
	case sig := <-signals:
		if err := cmd.Process.Signal(sig); err != nil && !errors.Is(err, os.ErrProcessDone) {
			if killErr := cmd.Process.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
				return errors.Join(err, killErr)
			}
		}

		select {
		case err := <-done:
			return err
		case <-time.After(10 * time.Second):
			if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				return err
			}
			return <-done
		}
	}
}

// prepareOpenCode materializes Kronk's embedded defaults and returns the
// isolated workspace and child-process environment.
func prepareOpenCode(root string, parentEnv, availableModels []string, serverURL string) (string, []string, error) {
	workspace := filepath.Join(root, "workspace")
	configHome := filepath.Join(root, "config")
	configDir := filepath.Join(configHome, "opencode")
	dataHome := filepath.Join(root, "data")
	cacheHome := filepath.Join(root, "cache")
	stateHome := filepath.Join(root, "state")
	tmpDir := filepath.Join(root, "tmp")
	homeDir := filepath.Join(root, "home")

	for _, dir := range []string{workspace, configDir, dataHome, cacheHome, stateHome, tmpDir, homeDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", nil, fmt.Errorf("creating temporary OpenCode directory: %w", err)
		}
	}

	if err := materializeDefaults(configDir); err != nil {
		return "", nil, err
	}
	if err := filterOpenCodeModels(configDir, availableModels); err != nil {
		return "", nil, err
	}

	content, err := runtimeConfig(serverURL)
	if err != nil {
		return "", nil, err
	}

	overrides := []string{
		"HOME=" + homeDir,
		"XDG_CONFIG_HOME=" + configHome,
		"XDG_DATA_HOME=" + dataHome,
		"XDG_CACHE_HOME=" + cacheHome,
		"XDG_STATE_HOME=" + stateHome,
		"TMPDIR=" + tmpDir,
		"TMP=" + tmpDir,
		"TEMP=" + tmpDir,
		"OPENCODE_CONFIG_CONTENT=" + content,
		"OPENCODE_AUTH_CONTENT={}",
		"OPENCODE_DISABLE_PROJECT_CONFIG=1",
	}

	return workspace, isolatedEnv(parentEnv, overrides), nil
}

func materializeDefaults(configDir string) error {
	return fs.WalkDir(agentdefaults.Files, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "." || path == "opencode" || path == "opencode/auth.json" {
			return nil
		}

		destination := filepath.Join(configDir, filepath.FromSlash(path))
		if path == "AGENTS.md" {
			destination = filepath.Join(configDir, "AGENTS.md")
		} else if strings.HasPrefix(path, "opencode/") {
			destination = filepath.Join(configDir, filepath.Base(path))
		}

		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}

		data, err := agentdefaults.Files.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading embedded OpenCode default %s: %w", path, err)
		}

		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return fmt.Errorf("creating OpenCode default directory: %w", err)
		}
		if err := os.WriteFile(destination, data, 0o600); err != nil {
			return fmt.Errorf("writing OpenCode default %s: %w", path, err)
		}

		return nil
	})
}

func runtimeConfig(serverURL string) (string, error) {
	baseURL, err := url.JoinPath(serverURL, "/v1")
	if err != nil {
		return "", fmt.Errorf("building OpenCode server URL: %w", err)
	}

	apiKey := "kronk"
	if os.Getenv("KRONK_TOKEN") != "" {
		apiKey = "{env:KRONK_TOKEN}"
	}

	config := map[string]any{
		"provider": map[string]any{
			"kronk": map[string]any{
				"options": map[string]any{
					"baseURL": baseURL,
					"apiKey":  apiKey,
				},
			},
		},
		"enabled_providers":  []string{"kronk"},
		"disabled_providers": []string{},
	}

	data, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("encoding OpenCode runtime config: %w", err)
	}

	return string(data), nil
}

// isolatedEnv removes inherited OpenCode settings and replaces the environment
// variables that locate OpenCode configuration and state.
func isolatedEnv(parent, overrides []string) []string {
	replaced := make(map[string]bool, len(overrides))
	for _, value := range overrides {
		key, _, _ := strings.Cut(value, "=")
		replaced[key] = true
	}

	env := make([]string, 0, len(parent)+len(overrides))
	for _, value := range parent {
		key, _, ok := strings.Cut(value, "=")
		if !ok || replaced[key] || strings.HasPrefix(key, "OPENCODE_") {
			continue
		}
		env = append(env, value)
	}

	return append(env, overrides...)
}
