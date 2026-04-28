package models

import (
	"fmt"
	"os"

	"go.yaml.in/yaml/v2"
)

// LoadModelConfig parses the model_config.yaml file at path and returns
// the per-model overrides keyed by model id. The caller supplies the
// path; an empty/missing file returns an empty map.
func LoadModelConfig(path string) (map[string]ModelConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load-model-config: reading model config file: %w", err)
	}

	var configs map[string]ModelConfig
	if err := yaml.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("load-model-config: unmarshaling model config: %w", err)
	}

	if configs == nil {
		configs = map[string]ModelConfig{}
	}

	return configs, nil
}
