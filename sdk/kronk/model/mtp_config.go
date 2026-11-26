package model

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	internalspec "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	mtpengine "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation/mtp"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
	yzmaspec "github.com/hybridgroup/yzma/exp/speculative"
)

var (
	mtpArtifactEmbedded  = internalspec.MTPArtifactEmbedded
	mtpArtifactCompanion = internalspec.MTPArtifactCompanion
)

// defMTPNDraft is the default number of speculative tokens to draft per
// round when MTP is auto-enabled and no explicit count was configured.
// This matches llama.cpp's MTP default.
const defMTPNDraft = 3

// resolveMTPSpeculationPlan adapts model configuration and detected capabilities
// to the internal speculation plan without loading native resources.
func resolveMTPSpeculationPlan(cfg Config, companion modelprofile.MTPArchitecture) (internalspec.Plan, error) {
	embedded, err := modelFilesMTPArchitecture(cfg.ModelFiles)
	if err != nil {
		return internalspec.Plan{}, fmt.Errorf("detect embedded MTP: %w", err)
	}

	architecture := companion
	if architecture == modelprofile.MTPArchitectureNone {
		architecture = embedded
	}

	return internalspec.Resolve(internalspec.Config{
		Mode:                     cfg.SpeculationMode(),
		ClassicConfigured:        cfg.PtrDraftModel != nil && cfg.PtrDraftModel.IsSeparate(),
		ClassicNDraft:            configuredClassicNDraft(cfg),
		MTPNDraft:                mtpNDraft(cfg),
		EmbeddedMTPArchitecture:  internalMTPArchitecture(embedded),
		CompanionMTPArchitecture: internalMTPArchitecture(companion),
		MTPAvailable:             yzmaspec.Available(),
		MTPArchitectureEnabled:   modelprofile.MTPEnabled(architecture),
	})
}

func internalMTPArchitecture(architecture modelprofile.MTPArchitecture) internalspec.MTPArchitecture {
	switch architecture {
	case modelprofile.MTPArchitectureQwen35OwnKV:
		return internalspec.MTPArchitectureQwen35OwnKV
	case modelprofile.MTPArchitectureQwen4ExpOwnKV:
		return internalspec.MTPArchitectureQwen4ExpOwnKV
	case modelprofile.MTPArchitectureNemotronOwnKV:
		return internalspec.MTPArchitectureNemotronOwnKV
	case modelprofile.MTPArchitectureGemmaSharedKV:
		return internalspec.MTPArchitectureGemmaSharedKV
	case modelprofile.MTPArchitectureGLM5Next:
		return internalspec.MTPArchitectureGLM5Next
	default:
		return internalspec.MTPArchitectureNone
	}
}

// modelFilesMTPArchitecture reports the MTP runtime contract declared by the
// first GGUF shard, including recognized contracts that Kronk does not enable.
// Qwen35 also requires its first MTP block's projection in the tensor directory
// of one of the shards: converted models can retain NextN metadata after
// stripping the MTP weights. This check must run before llama loads the model:
// llama.cpp skips gated MTP tensors unless ModelParams.LoadMTP is enabled.
func modelFilesMTPArchitecture(modelFiles []string) (modelprofile.MTPArchitecture, error) {
	if len(modelFiles) == 0 {
		return modelprofile.MTPArchitectureNone, fmt.Errorf("no model files provided")
	}

	data, err := gguf.ReadHeaderBytes(modelFiles[0])
	if err != nil {
		return modelprofile.MTPArchitectureNone, err
	}

	metadata, err := gguf.ParseMetadata(data)
	if err != nil {
		return modelprofile.MTPArchitectureNone, err
	}

	return mtpengine.DetectArchitecture(metadata, func(head string) (bool, error) {
		for i, file := range modelFiles {
			if i > 0 {
				data, err = gguf.ReadHeaderBytes(file)
				if err != nil {
					return false, err
				}
			}
			_, tensors, err := gguf.ParseHeaderAndTensors(data, 0)
			if err != nil {
				return false, fmt.Errorf("read MTP tensor directory %q: %w", file, err)
			}
			for _, tensor := range tensors {
				if tensor.Name == head {
					return true, nil
				}
			}
		}
		return false, nil
	})
}

// mtpNDraft returns the starting (ceiling) number of draft tokens for the
// auto-detected MTP drafter. An MTP nDraft override — a DraftModel block
// with no model files — sets the count explicitly; otherwise the default
// defMTPNDraft is used.
func mtpNDraft(cfg Config) int {
	if cfg.PtrDraftModel != nil && !cfg.PtrDraftModel.IsSeparate() && cfg.PtrDraftModel.NDraft > 0 {
		return cfg.PtrDraftModel.NDraft
	}
	return defMTPNDraft
}

// mtpFilesEnabled applies the architecture activation gate when metadata can
// be read. Only enabled runtime contracts reserve MTP resources; NextN tensors
// alone do not activate MTP. Unreadable files retain the conservative estimate
// until model loading performs strict capability resolution.
func mtpFilesEnabled(files []string) bool {
	architecture, err := modelFilesMTPArchitecture(files)
	return err != nil || modelprofile.MTPEnabled(architecture)
}

// probeMTPCompanion reports the runtime shape declared by a separate-file MTP
// companion. Unsupported architectures return MTPArchitectureNone.
func probeMTPCompanion(ctx context.Context, log applog.Logger, file string) modelprofile.MTPArchitecture {
	architecture, err := modelFilesMTPArchitecture([]string{file})
	if err != nil {
		if log != nil {
			log(ctx, "draft-model-mtp", "status", "probe-skip", "file", file, "err", err)
		}
		return modelprofile.MTPArchitectureNone
	}

	return architecture
}
