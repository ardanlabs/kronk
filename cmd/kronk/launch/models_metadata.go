package launch

import (
	"embed"
	"fmt"
	"strings"
	"sync"

	"go.yaml.in/yaml/v2"
)

//go:embed yaml/models.yaml
var launchModelsFS embed.FS

// embeddedModelsPath mirrors the //go:embed directive above.
const embeddedModelsPath = "yaml/models.yaml"

// launchModelsSchemaVersion is the schema version this build understands. The
// loader rejects any other value so the format can evolve safely.
const launchModelsSchemaVersion = 1

// launchModelsFile is the top-level shape of yaml/models.yaml.
type launchModelsFile struct {
	Schema int                    `yaml:"schema"`
	Order  []string               `yaml:"order"`
	Models map[string]launchModel `yaml:"models"`
}

// launchModel is the metadata for a single curated launch model.
type launchModel struct {
	// Key is the base model name this entry is keyed by (filled in by the
	// loader from the map key so callers have it without the map).
	Key string `yaml:"-"`

	Alias    string `yaml:"alias"`
	Display  string `yaml:"display"`
	Family   string `yaml:"family"`
	Quant    string `yaml:"quant"`
	PullID   string `yaml:"pull_id"`
	SizeNote string `yaml:"size_note"`
}

var (
	launchModelsOnce sync.Once
	launchModelsData launchModelsFile
	launchModelsErr  error
)

// loadLaunchModels parses the embedded curated-models metadata once, caching
// the result (and any error). It validates the schema version so an
// incompatible file fails loudly rather than silently misbehaving.
func loadLaunchModels() (launchModelsFile, error) {
	launchModelsOnce.Do(func() {
		data, err := launchModelsFS.ReadFile(embeddedModelsPath)
		if err != nil {
			launchModelsErr = fmt.Errorf("read embedded launch models metadata: %w", err)
			return
		}

		if err := yaml.Unmarshal(data, &launchModelsData); err != nil {
			launchModelsErr = fmt.Errorf("parse launch models metadata: %w", err)
			return
		}

		if launchModelsData.Schema != launchModelsSchemaVersion {
			launchModelsErr = fmt.Errorf("launch models metadata: unsupported schema %d (want %d)", launchModelsData.Schema, launchModelsSchemaVersion)
			return
		}

		// Backfill each entry's key so callers can carry a launchModel around
		// without also threading the map key.
		for key, m := range launchModelsData.Models {
			m.Key = key
			launchModelsData.Models[key] = m
		}

		// Every id named in order must have an entry so the preference list can
		// never point at a missing model.
		for _, key := range launchModelsData.Order {
			if _, ok := launchModelsData.Models[key]; !ok {
				launchModelsErr = fmt.Errorf("launch models metadata: order references unknown model %q", key)
				return
			}
		}
	})

	return launchModelsData, launchModelsErr
}

// curatedMatch describes how well a discovered model id matches a curated key.
type curatedMatch int

const (
	// noMatch means the id does not refer to the curated model.
	noMatch curatedMatch = iota

	// baseMatch means the id refers to the base model but carries no profile
	// suffix (e.g. "provider/model" or bare "model"). Usable, but it lacks the
	// AGENT context/sampling profile.
	baseMatch

	// profileMatch means the id refers to the curated model with a profile
	// suffix after it (e.g. ".../model/AGENT"), which carries the large context
	// window and sampling settings an agent needs. Preferred.
	profileMatch
)

// matchCurated classifies how a discovered model id matches the curated model
// named by key. The server reports profile models in several forms
// ("provider/model/AGENT", "model/AGENT", "provider/model", or bare "model"),
// so matching is by path segment: key must appear as one of the "/"-separated
// segments. When a segment follows the key a profile suffix is present.
func matchCurated(id, key string) curatedMatch {
	segs := strings.Split(id, "/")
	for i, seg := range segs {
		if seg != key {
			continue
		}
		if i < len(segs)-1 {
			return profileMatch
		}
		return baseMatch
	}

	return noMatch
}

// installedCuratedModel returns the installed discovered model for the curated
// model named by key, preferring a profile variant (which carries the AGENT
// context/sampling profile) over a bare base match. ok is false when none is
// installed.
func installedCuratedModel(key string, chatModels []Model) (Model, bool) {
	var base Model
	haveBase := false

	for _, m := range chatModels {
		switch matchCurated(m.ID, key) {
		case profileMatch:
			return m, true
		case baseMatch:
			if !haveBase {
				base = m
				haveBase = true
			}
		}
	}

	return base, haveBase
}

// firstInstalledCurated returns the highest-preference curated model that is
// installed, per the metadata order, along with the matching discovered model.
// It returns ok=false when metadata is unavailable or none are installed.
func firstInstalledCurated(chatModels []Model) (launchModel, Model, bool) {
	lm, err := loadLaunchModels()
	if err != nil {
		return launchModel{}, Model{}, false
	}

	for _, key := range lm.Order {
		if m, ok := installedCuratedModel(key, chatModels); ok {
			return lm.Models[key], m, true
		}
	}

	return launchModel{}, Model{}, false
}

// curatedInstallStatus reports, in metadata order, how many of the curated
// launch models are installed and which ones are missing. total is the number
// of curated models; missing lists the ones not installed (empty when all are
// present or metadata is unavailable).
func curatedInstallStatus(chatModels []Model) (total int, missing []launchModel) {
	lm, err := loadLaunchModels()
	if err != nil {
		return 0, nil
	}

	total = len(lm.Order)
	for _, key := range lm.Order {
		if _, ok := installedCuratedModel(key, chatModels); !ok {
			missing = append(missing, lm.Models[key])
		}
	}

	return total, missing
}

// orderedCurated returns the curated models in metadata order. It returns nil
// when metadata is unavailable.
func orderedCurated() []launchModel {
	lm, err := loadLaunchModels()
	if err != nil {
		return nil
	}

	out := make([]launchModel, 0, len(lm.Order))
	for _, key := range lm.Order {
		out = append(out, lm.Models[key])
	}

	return out
}

// lookupCurated returns the curated model whose key or alias matches name
// (case-insensitive). It is how "--model qwen" resolves to a curated entry.
func lookupCurated(name string) (launchModel, bool) {
	lm, err := loadLaunchModels()
	if err != nil {
		return launchModel{}, false
	}

	for _, m := range lm.Models {
		if strings.EqualFold(m.Key, name) || (m.Alias != "" && strings.EqualFold(m.Alias, name)) {
			return m, true
		}
	}

	return launchModel{}, false
}
