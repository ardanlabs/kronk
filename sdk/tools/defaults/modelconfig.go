package defaults

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed yaml/model_config.yaml
var embeddedFS embed.FS

const (
	// modelsDirName is the per-user models directory under BaseDir. It owns
	// the downloaded GGUF files and model_config.yaml so the model config
	// lives next to the assets it configures.
	modelsDirName = "models"

	// modelConfigFileName is the on-disk filename written under modelsDirName.
	modelConfigFileName = "model_config.yaml"

	// embeddedModelConfigPath is the path inside embeddedFS, which
	// mirrors the //go:embed directive above.
	embeddedModelConfigPath = "yaml/model_config.yaml"
)

// ModelConfigFile returns the path to the model config file. If no override
// is provided, it ensures <basePath>/models/ exists and seeds the embedded
// default to <basePath>/models/model_config.yaml when no file is present. If
// an older <basePath>/model_config.yaml exists at the legacy location and no
// new file exists, the legacy file is moved to the new location.
func ModelConfigFile(override string, basePath string) (string, error) {
	if override != "" {
		return override, nil
	}

	basePath = BaseDir(basePath)

	modelsDir := filepath.Join(basePath, modelsDirName)
	target := filepath.Join(modelsDir, modelConfigFileName)

	if _, err := os.Stat(target); err == nil {
		return target, nil
	}

	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return "", fmt.Errorf("model-config-file: creating models directory: %w", err)
	}

	// Migrate the legacy <basePath>/model_config.yaml if it exists.
	legacy := filepath.Join(basePath, modelConfigFileName)
	if _, err := os.Stat(legacy); err == nil {
		if err := os.Rename(legacy, target); err != nil {
			return "", fmt.Errorf("model-config-file: migrating legacy file: %w", err)
		}
		return target, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("model-config-file: stat legacy: %w", err)
	}

	data, err := embeddedFS.ReadFile(embeddedModelConfigPath)
	if err != nil {
		return "", fmt.Errorf("model-config-file: reading embedded config: %w", err)
	}

	if err := os.WriteFile(target, data, 0644); err != nil {
		return "", fmt.Errorf("model-config-file: writing default config: %w", err)
	}

	return target, nil
}
