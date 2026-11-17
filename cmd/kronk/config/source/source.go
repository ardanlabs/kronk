// Package source reads the local Kronk data the config tool works from: the
// installed and catalog models, a model's GGUF facts and AutoTune analysis,
// this machine's devices or the machine the user describes, and the chat
// template the server would use.
package source

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// loraAdapters lists the adapter IDs in <base>/lora: GGUF file paths relative
// to the folder, without .gguf.
func loraAdapters(dir string) []string {
	var out []string
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".gguf") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err == nil {
			out = append(out, filepath.ToSlash(strings.TrimSuffix(rel, ".gguf")))
		}
		return nil
	})
	return out
}

// Source reads the local Kronk data: models, catalog and the config file.
type Source struct {
	basePath string
	mdls     *models.Models

	initOnce sync.Once
	initErr  error
}

// New returns a Source for the Kronk data under basePath.
func New(basePath string) (*Source, error) {
	mdls, err := models.NewWithPaths(basePath)
	if err != nil {
		return nil, fmt.Errorf("unable to create models system: %w", err)
	}

	return &Source{basePath: basePath, mdls: mdls}, nil
}

// ConfigFile resolves the file the server reads, the same way the server
// does: KRONK_POOL_MODEL_CONFIG_FILE first, then <base>/models/model_config.yaml.
func (s *Source) ConfigFile() (string, error) {
	return defaults.ModelConfigFile(os.Getenv("KRONK_POOL_MODEL_CONFIG_FILE"), s.basePath)
}

// LoadDoc reads the config file. A missing file returns an empty document.
func (s *Source) LoadDoc(path string) (modelconfig.Document, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return modelconfig.Document{Models: map[string]models.ModelConfig{}}, nil
	}

	return modelconfig.Load(path)
}

// Choices lists installed models first, then catalog models that are not
// downloaded yet.
func (s *Source) Choices(doc modelconfig.Document) ([]Choice, error) {
	files, err := s.mdls.Files()
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}

	cat, err := s.mdls.Catalog()
	if err != nil {
		return nil, fmt.Errorf("load catalog: %w", err)
	}

	// endpoint is what the model does, empty for a chat model. A model the
	// catalog doesn't know is read from its local GGUF header.
	endpoint := func(id string) string {
		entry, ok := cat.Models[id]
		e := entry.Capabilities.Endpoint
		if !ok || e == "" {
			if info, err := s.mdls.ModelInformation(id); err == nil {
				e = models.CapabilitiesForModel(info.Metadata, info.HasProjection, id).Endpoint
			}
		}
		if e == EndpointChat {
			return ""
		}
		return e
	}

	seen := make(map[string]bool)
	var installed []Choice
	for _, f := range files {
		id := f.ID
		if f.OwnedBy != "" {
			id = f.OwnedBy + "/" + f.ID
		}
		if seen[id] {
			continue
		}
		seen[id] = true

		inFile := len(EntriesFor(doc, id)) > 0
		installed = append(installed, Choice{ID: id, Installed: true, InFile: inFile, Size: f.Size, Validated: f.Validated, Endpoint: endpoint(id), Tokenizer: f.TokenizerFingerprint})
	}

	var catalog []Choice
	for id, entry := range cat.Models {
		if seen[id] {
			continue
		}

		size := entry.MMProjSize + entry.MTPSize
		for _, n := range entry.FileSizes {
			size += n
		}

		inFile := len(EntriesFor(doc, id)) > 0
		catalog = append(catalog, Choice{ID: id, InFile: inFile, Size: size, Endpoint: endpoint(id)})
	}

	byID := func(a, b Choice) int { return strings.Compare(a.ID, b.ID) }
	slices.SortFunc(installed, byID)
	slices.SortFunc(catalog, byID)

	// Installed models first, then the catalog; chat before audio and
	// image in each.
	mediaInstalled, mediaCatalog := s.mediaChoices(doc)
	return slices.Concat(installed, mediaInstalled, catalog, mediaCatalog), nil
}

// EntriesFor lists the entries in models: for a model: its own and its
// variants (id/NAME), sorted.
func EntriesFor(doc modelconfig.Document, id string) []string {
	var out []string
	for key := range doc.Models {
		if key == id || strings.HasPrefix(key, id+"/") && !strings.Contains(strings.TrimPrefix(key, id+"/"), "/") {
			out = append(out, key)
		}
	}
	slices.Sort(out)
	return out
}

// Facts reads the model metadata and runs the AutoTune analysis. Installed
// models are read from disk; catalog models from the GGUF header, which is
// cached or fetched with a range request.
func (s *Source) Facts(ctx context.Context, c Choice, other *Machine) (Facts, error) {
	info, err := s.modelInfo(ctx, c)
	if err != nil {
		return Facts{}, err
	}

	f := Facts{ID: c.ID, Info: info, Other: other}
	f.Endpoint = models.CapabilitiesForModel(info.Metadata, info.HasProjection, c.ID).Endpoint
	if info.HasProjection {
		f.Inputs = s.projectorInputs(ctx, c)
	}
	f.Adapters = loraAdapters(filepath.Join(defaults.BaseDir(s.basePath), "lora"))

	devs, err := s.devicesFor(other)
	if err != nil {
		f.HardwareErr = err
	} else {
		f.HardwareKnown = true
	}
	f.Devices = devs

	analysis, err := models.Analyze(info, devs)
	if err != nil {
		return Facts{}, fmt.Errorf("analyze model: %w", err)
	}
	f.Analysis = analysis

	if other == nil {
		f.NUMANodes = numaNodes()
	}
	f.Templates = NewTemplates(c, s.basePath, other == nil, gguf.ChatTemplate(info.Metadata))

	return f, nil
}

// Check runs the analysis again with the answers as constraints, on the
// same model facts and hardware as the questions.
func (s *Source) Check(f Facts, cfg models.ModelConfig) (models.Analysis, error) {
	return models.AnalyzeWithConfig(f.Info, f.Devices, cfg)
}

func (s *Source) modelInfo(ctx context.Context, c Choice) (models.ModelInfo, error) {
	if c.Installed {
		info, err := s.mdls.ModelInformation(c.ID)
		if err != nil {
			return models.ModelInfo{}, fmt.Errorf("read model %s: %w", c.ID, err)
		}
		return info, nil
	}

	entry, ok, err := s.mdls.CatalogEntry(c.ID)
	if err != nil {
		return models.ModelInfo{}, fmt.Errorf("lookup catalog entry: %w", err)
	}
	if !ok {
		return models.ModelInfo{}, fmt.Errorf("catalog entry %q not found", c.ID)
	}

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	data, err := s.mdls.GGUFHead(ctx, entry)
	if err != nil {
		return models.ModelInfo{}, fmt.Errorf("read catalog GGUF header: %w", err)
	}

	return catalogInfo(c, entry, data)
}

// catalogInfo builds the model information of the catalog model c from its
// GGUF header.
func catalogInfo(c Choice, entry models.CatalogEntry, header []byte) (models.ModelInfo, error) {
	metadata, err := gguf.ParseMetadata(header)
	if err != nil {
		return models.ModelInfo{}, fmt.Errorf("parse catalog GGUF header: %w", err)
	}

	var size uint64
	for _, n := range entry.FileSizes {
		if n > 0 {
			size += uint64(n)
		}
	}

	return models.ModelInfoFromMetadata(c.ID, metadata, size, entry.MMProj != "", entry.MTP != ""), nil
}

// devicesFor is the hardware the analysis runs on: the machine the user
// described, or this one, which needs the llama.cpp libraries to be listed.
func (s *Source) devicesFor(other *Machine) (devices.Devices, error) {
	switch {
	case other != nil:
		return other.Devices(), nil
	case s.initLibs() != nil:
		return devices.Devices{}, ErrNoLibs
	}
	return devices.List(), nil
}

// ErrNoLibs is reported when llama.cpp can't be loaded, so the machine's
// devices are unknown.
var ErrNoLibs = errors.New("llama.cpp libraries not installed, run: kronk libs --local")

// initLibs loads llama.cpp once. Device listing and the memory check need it.
func (s *Source) initLibs() error {
	s.initOnce.Do(func() {
		if err := kronk.Init(); err != nil {
			s.initErr = ErrNoLibs
		}
	})

	return s.initErr
}
