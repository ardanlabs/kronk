package launch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	agentdefaults "github.com/ardanlabs/kronk/.agents/default"
)

func filterOpenCodeModels(configDir string, available []string) error {
	if len(available) == 0 {
		return fmt.Errorf("at least one OpenCode model must be available")
	}

	config, err := readOpenCodeConfig()
	if err != nil {
		return err
	}

	data, err := filteredOpenCodeConfig(config, available)
	if err != nil {
		return err
	}

	path := filepath.Join(configDir, "opencode.jsonc")
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("writing filtered OpenCode config: %w", err)
	}

	return nil
}

func filteredOpenCodeConfig(config map[string]any, available []string) ([]byte, error) {
	entries, err := modelEntries(config)
	if err != nil {
		return nil, err
	}

	keep := make(map[string]bool, len(available))
	for _, id := range available {
		keep[id] = true
	}
	for id := range entries {
		if !keep[id] {
			delete(entries, id)
		}
	}
	for _, id := range available {
		if _, exists := entries[id]; !exists {
			entries[id] = map[string]any{"name": id}
		}
	}
	if mcp, ok := config["mcp"].(map[string]any); ok {
		delete(mcp, "kronk")
	}

	defaultModel, _ := config["model"].(string)
	defaultID := strings.TrimPrefix(defaultModel, "kronk/")
	if !keep[defaultID] {
		defaultID = available[0]
	}
	config["model"] = "kronk/" + defaultID
	config["small_model"] = "kronk/" + defaultID

	data, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		return nil, fmt.Errorf("encoding filtered OpenCode config: %w", err)
	}

	return data, nil
}

func readOpenCodeConfig() (map[string]any, error) {
	data, err := agentdefaults.Files.ReadFile("opencode/opencode.jsonc")
	if err != nil {
		return nil, fmt.Errorf("reading embedded OpenCode config: %w", err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing embedded OpenCode config: %w", err)
	}

	return config, nil
}

func modelEntries(config map[string]any) (map[string]any, error) {
	providers, ok := config["provider"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("OpenCode config is missing provider")
	}
	kronkProvider, ok := providers["kronk"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("OpenCode config is missing the Kronk provider")
	}
	entries, ok := kronkProvider["models"].(map[string]any)
	if !ok || len(entries) == 0 {
		return nil, fmt.Errorf("OpenCode config has no Kronk models")
	}

	return entries, nil
}
