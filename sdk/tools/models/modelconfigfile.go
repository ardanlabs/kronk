package models

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v2"
)

const modelConfigVersion = 1

// UpgradeModelConfig upgrades a legacy model config in place to the current
// document format. Version 0 stored model overrides at the document root;
// version 1 stores them under models and adds an explicit version field.
func UpgradeModelConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("upgrade-model-config: reading model config file: %w", err)
	}

	var header struct {
		Version int `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &header); err != nil {
		return fmt.Errorf("upgrade-model-config: unmarshaling model config: %w", err)
	}

	switch header.Version {
	case 0:
		var configs map[string]ModelConfig
		if err := yaml.Unmarshal(data, &configs); err != nil {
			return fmt.Errorf("upgrade-model-config: unmarshaling version 0 model config: %w", err)
		}

		doc := struct {
			Version int                    `yaml:"version"`
			Models  map[string]ModelConfig `yaml:"models"`
		}{
			Version: modelConfigVersion,
			Models:  configs,
		}

		upgraded, err := yaml.Marshal(doc)
		if err != nil {
			return fmt.Errorf("upgrade-model-config: marshaling version 1 model config: %w", err)
		}
		if err := os.WriteFile(path, upgraded, 0644); err != nil {
			return fmt.Errorf("upgrade-model-config: writing model config file: %w", err)
		}

		return nil

	case modelConfigVersion:
		return nil

	default:
		return fmt.Errorf("upgrade-model-config: unsupported config version %d", header.Version)
	}
}

// LoadModelConfig parses the model_config.yaml file at path and returns
// the per-model overrides keyed by model id. The caller supplies the
// path; an empty/missing file returns an empty map.
func LoadModelConfig(path string) (map[string]ModelConfig, error) {
	if err := UpgradeModelConfig(path); err != nil {
		return nil, fmt.Errorf("load-model-config: upgrading model config: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load-model-config: reading model config file: %w", err)
	}

	var header struct {
		Version int                    `yaml:"version"`
		Models  map[string]ModelConfig `yaml:"models"`
	}
	if err := yaml.Unmarshal(data, &header); err != nil {
		return nil, fmt.Errorf("load-model-config: unmarshaling model config: %w", err)
	}

	var configs map[string]ModelConfig
	switch header.Version {
	case modelConfigVersion:
		configs = header.Models
	default:
		return nil, fmt.Errorf("load-model-config: unsupported config version %d", header.Version)
	}

	if configs == nil {
		configs = map[string]ModelConfig{}
	}

	return configs, nil
}
