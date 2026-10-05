// Package modelconfig loads backend-specific model settings for the model server.
package modelconfig

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/tools/models"
	"go.yaml.in/yaml/v2"
)

const currentVersion = 2

// BuckyModelConfig contains per-model Whisper runtime overrides.
type BuckyModelConfig struct {
	NSeqMax          *int             `yaml:"nseq-max,omitempty"`
	QueueDepth       *int             `yaml:"queue-depth,omitempty"`
	AdmissionTimeout *models.Duration `yaml:"admission-timeout,omitempty"`
	NThreads         *int32           `yaml:"nthreads,omitempty"`
}

// Validate checks Bucky model settings before the pool starts.
func (cfg BuckyModelConfig) Validate() error {
	if cfg.NSeqMax != nil && *cfg.NSeqMax < 1 {
		return errors.New("nseq-max must be positive")
	}
	if cfg.QueueDepth != nil && *cfg.QueueDepth < 0 {
		return errors.New("queue-depth cannot be negative")
	}
	if cfg.AdmissionTimeout != nil && time.Duration(*cfg.AdmissionTimeout) <= 0 {
		return errors.New("admission-timeout must be positive")
	}
	if cfg.NThreads != nil && *cfg.NThreads < 0 {
		return errors.New("nthreads cannot be negative")
	}

	return nil
}

// MalinaModelConfig contains per-model image-generation runtime overrides.
type MalinaModelConfig struct {
	Concurrency      *int             `yaml:"concurrency,omitempty"`
	QueueDepth       *int             `yaml:"queue-depth,omitempty"`
	AdmissionTimeout *models.Duration `yaml:"admission-timeout,omitempty"`
	CPUThreads       *int32           `yaml:"cpu-threads,omitempty"`
}

// Validate checks Malina model settings before the pool starts.
func (cfg MalinaModelConfig) Validate() error {
	if cfg.Concurrency != nil && *cfg.Concurrency < 1 {
		return errors.New("concurrency must be positive")
	}
	if cfg.QueueDepth != nil && *cfg.QueueDepth < 0 {
		return errors.New("queue-depth cannot be negative")
	}
	if cfg.AdmissionTimeout != nil && time.Duration(*cfg.AdmissionTimeout) <= 0 {
		return errors.New("admission-timeout must be positive")
	}
	if cfg.CPUThreads != nil && *cfg.CPUThreads < 0 {
		return errors.New("cpu-threads cannot be negative")
	}

	return nil
}

// Document contains every backend's per-model configuration.
type Document struct {
	Version      int                           `yaml:"version"`
	Models       map[string]models.ModelConfig `yaml:"models"`
	BuckyModels  map[string]BuckyModelConfig   `yaml:"bucky-models"`
	MalinaModels map[string]MalinaModelConfig  `yaml:"malina-models"`
}

// Load reads the model configuration document at path.
func Load(path string) (Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Document{}, fmt.Errorf("load: reading model config file: %w", err)
	}

	var doc Document
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return Document{}, fmt.Errorf("load: unmarshaling model config: %w", err)
	}

	switch doc.Version {
	case 1:
		if len(doc.BuckyModels) > 0 || len(doc.MalinaModels) > 0 {
			return Document{}, fmt.Errorf("load: bucky-models and malina-models require config version %d", currentVersion)
		}
	case currentVersion:
	default:
		return Document{}, fmt.Errorf("load: unsupported config version %d", doc.Version)
	}

	if doc.Models == nil {
		doc.Models = map[string]models.ModelConfig{}
	}
	if doc.BuckyModels == nil {
		doc.BuckyModels = map[string]BuckyModelConfig{}
	}
	if doc.MalinaModels == nil {
		doc.MalinaModels = map[string]MalinaModelConfig{}
	}

	for modelID, cfg := range doc.BuckyModels {
		if strings.TrimSpace(modelID) == "" {
			return Document{}, fmt.Errorf("load: bucky model id cannot be empty")
		}
		if err := cfg.Validate(); err != nil {
			return Document{}, fmt.Errorf("load: bucky model %q: %w", modelID, err)
		}
	}
	for modelID, cfg := range doc.MalinaModels {
		if strings.TrimSpace(modelID) == "" {
			return Document{}, fmt.Errorf("load: malina model id cannot be empty")
		}
		if err := cfg.Validate(); err != nil {
			return Document{}, fmt.Errorf("load: malina model %q: %w", modelID, err)
		}
	}

	return doc, nil
}
