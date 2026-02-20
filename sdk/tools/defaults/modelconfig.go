package defaults

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed default_model_config.yaml
var embeddedFS embed.FS

const defaultModelConfigFile = "default_model_config.yaml"

// ModelConfigFile returns the path to the model config file. If no override is
// provided, it writes the embedded default to the base path if it doesn't
// already exist and returns that path.
func ModelConfigFile(override string, basePath string) (string, error) {
	if override != "" {
		return override, nil
	}

	basePath = BaseDir(basePath)
	configPath := filepath.Join(basePath, "model_config.yaml")

	if _, err := os.Stat(configPath); err == nil {
		return configPath, nil
	}

	data, err := embeddedFS.ReadFile(defaultModelConfigFile)
	if err != nil {
		return "", fmt.Errorf("model-config-file: reading embedded config: %w", err)
	}

	if err := os.MkdirAll(basePath, 0755); err != nil {
		return "", fmt.Errorf("model-config-file: creating base directory: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return "", fmt.Errorf("model-config-file: writing default config: %w", err)
	}

	return configPath, nil
}
