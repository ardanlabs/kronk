package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
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

// choice is one model the user can configure.
type choice struct {
	ID        string
	Installed bool
	InFile    bool
	Size      int64

	// Validated is false for an installed model whose files were never
	// checked, which includes downloads that stopped early.
	Validated bool

	// Endpoint is the catalog's endpoint for the model (embeddings, rerank,
	// decision) when it is not a chat model. Empty for chat or unknown.
	Endpoint string
}

// facts is what the questions need to know about the picked model and the
// machine.
type facts struct {
	ID       string
	Info     models.ModelInfo
	Analysis models.Analysis

	// HardwareKnown is false when the llama.cpp libraries could not be
	// loaded, so devices and the memory check are unavailable.
	HardwareKnown bool
	HardwareErr   error
	GPUs          int

	// Devices is the hardware the analysis and the memory check use: this
	// machine's, or the one the user described when Other is set.
	Devices devices.Devices
	Other   *machine

	// TemplateKnown is false when the GGUF has no chat template. The
	// thinking and reasoning questions are then shown anyway.
	TemplateKnown bool
	Thinking      bool
	Reasoning     bool

	// TemplateVars are the names the chat template checks with "is defined",
	// shown as hints for the custom variables question.
	TemplateVars []string
}

func (f facts) moe() bool        { return f.Analysis.Model.MoE != nil }
func (f facts) projection() bool { return f.Info.HasProjection }
func (f facts) mtp() bool        { return f.Info.HasMTP }
func (f facts) swa() bool        { return f.Analysis.Model.Attention.SlidingWindow > 0 }

// source reads the local Kronk data: models, catalog and the config file.
type source struct {
	basePath string
	mdls     *models.Models

	initOnce sync.Once
	initErr  error
}

func newSource(basePath string) (*source, error) {
	mdls, err := models.NewWithPaths(basePath)
	if err != nil {
		return nil, fmt.Errorf("unable to create models system: %w", err)
	}

	return &source{basePath: basePath, mdls: mdls}, nil
}

// configFile resolves the file the server reads, the same way the server
// does: KRONK_POOL_MODEL_CONFIG_FILE first, then <base>/models/model_config.yaml.
func (s *source) configFile() (string, error) {
	return defaults.ModelConfigFile(os.Getenv("KRONK_POOL_MODEL_CONFIG_FILE"), s.basePath)
}

// loadDoc reads the config file. A missing file returns an empty document.
func (s *source) loadDoc(path string) (modelconfig.Document, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return modelconfig.Document{Models: map[string]models.ModelConfig{}}, nil
	}

	return modelconfig.Load(path)
}

// choices lists installed models first, then catalog models that are not
// downloaded yet.
func (s *source) choices(doc modelconfig.Document) ([]choice, error) {
	files, err := s.mdls.Files()
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}

	cat, err := s.mdls.Catalog()
	if err != nil {
		return nil, fmt.Errorf("load catalog: %w", err)
	}

	endpoint := func(id string) string {
		if e := cat.Models[id].Capabilities.Endpoint; e != "chat_completion" {
			return e
		}
		return ""
	}

	seen := make(map[string]bool)
	var installed []choice
	for _, f := range files {
		id := f.ID
		if f.OwnedBy != "" {
			id = f.OwnedBy + "/" + f.ID
		}
		if seen[id] {
			continue
		}
		seen[id] = true

		_, inFile := doc.Models[id]
		installed = append(installed, choice{ID: id, Installed: true, InFile: inFile, Size: f.Size, Validated: f.Validated, Endpoint: endpoint(id)})
	}

	var catalog []choice
	for id, entry := range cat.Models {
		if seen[id] {
			continue
		}

		size := entry.MMProjSize + entry.MTPSize
		for _, n := range entry.FileSizes {
			size += n
		}

		_, inFile := doc.Models[id]
		catalog = append(catalog, choice{ID: id, InFile: inFile, Size: size, Endpoint: endpoint(id)})
	}

	sort.Slice(installed, func(i, j int) bool { return installed[i].ID < installed[j].ID })
	sort.Slice(catalog, func(i, j int) bool { return catalog[i].ID < catalog[j].ID })

	return append(installed, catalog...), nil
}

// facts reads the model metadata and runs the AutoTune analysis. Installed
// models are read from disk; catalog models from the GGUF header, which is
// cached or fetched with a range request.
func (s *source) facts(ctx context.Context, c choice, other *machine) (facts, error) {
	info, err := s.modelInfo(ctx, c)
	if err != nil {
		return facts{}, err
	}

	f := facts{ID: c.ID, Info: info, Other: other}

	devs := devices.Devices{}
	switch {
	case other != nil:
		devs = other.devices()
		f.HardwareKnown = true
	case s.initLibs() != nil:
		f.HardwareErr = errNoLibs
	default:
		devs = devices.List()
		f.HardwareKnown = true
	}
	for _, d := range devs.Devices {
		if strings.HasPrefix(d.Type, "gpu") {
			f.GPUs++
		}
	}
	f.Devices = devs

	analysis, err := models.Analyze(info, devs)
	if err != nil {
		return facts{}, fmt.Errorf("analyze model: %w", err)
	}
	f.Analysis = analysis

	if tpl := gguf.ChatTemplate(info.Metadata); tpl != "" {
		f.TemplateKnown = true
		f.Thinking = strings.Contains(tpl, "enable_thinking")
		f.Reasoning = strings.Contains(tpl, "reasoning_effort")
		f.TemplateVars = templateVars(tpl)
	} else {
		f.Thinking = true
		f.Reasoning = true
	}

	return f, nil
}

// check runs the analysis again with the answers as constraints, on the
// same model facts and hardware as the questions.
func (s *source) check(f facts, cfg models.ModelConfig) (models.Analysis, error) {
	return models.AnalyzeWithConfig(f.Info, f.Devices, cfg)
}

func (s *source) modelInfo(ctx context.Context, c choice) (models.ModelInfo, error) {
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

	metadata, err := gguf.ParseMetadata(data)
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

// errNoLibs is reported when llama.cpp can't be loaded, so the machine's
// devices are unknown.
var errNoLibs = errors.New("llama.cpp libraries not installed, run: kronk libs --local")

// initLibs loads llama.cpp once. Device listing and the memory check need it.
func (s *source) initLibs() error {
	s.initOnce.Do(func() {
		if err := kronk.Init(); err != nil {
			s.initErr = errNoLibs
		}
	})

	return s.initErr
}
