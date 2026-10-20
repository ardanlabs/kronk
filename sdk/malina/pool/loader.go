package pool

import (
	"context"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/applog"
	malinasdk "github.com/ardanlabs/kronk/sdk/malina"
	"github.com/ardanlabs/kronk/sdk/malina/model"
	malinavram "github.com/ardanlabs/kronk/sdk/malina/vram"
	"github.com/ardanlabs/kronk/sdk/pool/engine/loader"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	malinamodels "github.com/ardanlabs/kronk/sdk/tools/malina/models"
	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
)

type preparedModel struct {
	config model.Config
	memory malinavram.Result
}

// StableDiffusion adapts Malina model bundles to the generic pool engine.
type StableDiffusion struct {
	log         applog.Logger
	models      *malinamodels.Models
	modelConfig map[string]modelconfig.MalinaModelConfig
	resman      *resman.Manager
}

func newStableDiffusion(log applog.Logger, models *malinamodels.Models, modelCfg map[string]modelconfig.MalinaModelConfig, rm *resman.Manager) *StableDiffusion {
	sd := StableDiffusion{
		log:         log,
		models:      models,
		modelConfig: modelCfg,
		resman:      rm,
	}

	return &sd
}

// Prepare resolves the bundle once for planning and loading.
func (sd *StableDiffusion) Prepare(_ context.Context, req loader.LoadRequest) (any, error) {
	if req.Custom != nil {
		return nil, fmt.Errorf("prepare: custom model configurations are not supported")
	}

	modelSize, err := sd.modelSize(req.ModelID)
	if err != nil {
		return nil, fmt.Errorf("prepare: %w", err)
	}
	cfg, err := sd.resolveConfig(req.ModelID)
	if err != nil {
		return nil, fmt.Errorf("prepare: %w", err)
	}

	memory := malinavram.Calculate(malinavram.Input{
		ModelSizeBytes: modelSize,
		Contexts:       int64(cfg.Concurrency),
	})

	return preparedModel{config: cfg, memory: memory}, nil
}

// Plan returns the RAM or VRAM reservation for the resolved model.
func (sd *StableDiffusion) Plan(ctx context.Context, req loader.LoadRequest) (resman.PlanRequest, error) {
	prepared, err := preparedForRequest(req)
	if err != nil {
		return resman.PlanRequest{}, fmt.Errorf("plan: %w", err)
	}

	plan := resman.PlanRequest{Key: req.Key}
	switch {
	case sd.resman.UnifiedMemory():
		plan.RAMBytes = prepared.memory.TotalVRAM
	case sd.resman.HasGPUs():
		plan.VRAMBytes = prepared.memory.TotalVRAM
	default:
		plan.RAMBytes = prepared.memory.TotalVRAM
	}

	sd.log(ctx, "malina-plan-request",
		"key", req.Key,
		"model-id", req.ModelID,
		"contexts", prepared.config.Concurrency,
		"model-bytes", prepared.memory.ModelBytes,
		"runtime-bytes", prepared.memory.RuntimeBytes,
		"vram", plan.VRAMBytes,
		"ram", plan.RAMBytes,
	)

	return plan, nil
}

// Load constructs a Malina handle after the engine reserves its memory.
func (sd *StableDiffusion) Load(ctx context.Context, req loader.LoadRequest) (*malinasdk.Malina, error) {
	prepared, err := preparedForRequest(req)
	if err != nil {
		return nil, fmt.Errorf("load: %w", err)
	}

	handle, err := malinasdk.NewWithContext(ctx, model.WithConfig(prepared.config))
	if err != nil {
		return nil, fmt.Errorf("load: create malina handle: %w", err)
	}

	sd.log(ctx, "malina-load",
		"status", "load new model",
		"model-id", req.ModelID,
		"model-version", handle.ModelInfo().ModelVersion,
		"contexts", prepared.config.Concurrency,
	)

	return handle, nil
}

// Display returns the model's budget and concurrency information.
func (sd *StableDiffusion) Display(handle *malinasdk.Malina, modelID string) loader.Display {
	cfg := handle.ModelConfig()
	display := loader.Display{Slots: cfg.Concurrency}
	if modelSize, err := sd.modelSize(modelID); err == nil {
		display.VRAMTotal = malinavram.Calculate(malinavram.Input{
			ModelSizeBytes: modelSize,
			Contexts:       int64(cfg.Concurrency),
		}).TotalVRAM
	}

	return display
}

func preparedForRequest(req loader.LoadRequest) (preparedModel, error) {
	prepared, ok := req.Prepared.(preparedModel)
	if !ok {
		return preparedModel{}, fmt.Errorf("prepared model is %T, want preparedModel", req.Prepared)
	}

	return prepared, nil
}

func (sd *StableDiffusion) resolveConfig(modelID string) (model.Config, error) {
	name, err := malinamodels.ParseBundleName(modelID)
	if err != nil {
		return model.Config{}, fmt.Errorf("resolve-config: %w: %v", malinamodels.ErrModelNotFound, err)
	}

	manifest, err := sd.models.LoadManifest(name)
	if err != nil {
		return model.Config{}, fmt.Errorf("resolve-config: load manifest: %w", err)
	}

	files := manifest.Files
	if files[malinamodels.RoleUpscaler.String()] != "" {
		return model.Config{}, fmt.Errorf("resolve-config: bundle %q is an upscaler, not an image-generation model", modelID)
	}

	var primary model.Option
	switch {
	case files[malinamodels.RoleModel.String()] != "":
		primary = model.WithModelPath(files[malinamodels.RoleModel.String()])
	case files[malinamodels.RoleDiffusion.String()] != "":
		primary = model.WithDiffusionModelPath(files[malinamodels.RoleDiffusion.String()])
	default:
		return model.Config{}, fmt.Errorf("resolve-config: bundle %q has no model or diffusion component", modelID)
	}

	options := []model.Option{primary}
	if override, ok := sd.modelConfig[modelID]; ok {
		if override.Concurrency != nil {
			options = append(options, model.WithConcurrency(*override.Concurrency))
		}
		if override.QueueDepth != nil {
			options = append(options, model.WithQueueDepth(*override.QueueDepth))
		}
		if override.AdmissionTimeout != nil {
			options = append(options, model.WithAdmissionTimeout(time.Duration(*override.AdmissionTimeout)))
		}
		if override.CPUThreads != nil {
			options = append(options, model.WithCPUThreads(*override.CPUThreads))
		}
	}

	cfg, err := model.NewConfig(options...)
	if err != nil {
		return model.Config{}, fmt.Errorf("resolve-config: model config: %w", err)
	}

	cfg.ClipLPath = files[malinamodels.RoleClipL.String()]
	cfg.ClipGPath = files[malinamodels.RoleClipG.String()]
	cfg.ClipVisionPath = files[malinamodels.RoleClipVision.String()]
	cfg.T5XXLPath = files[malinamodels.RoleT5XXL.String()]
	cfg.LLMPath = files[malinamodels.RoleLLM.String()]
	cfg.LLMVisionPath = files[malinamodels.RoleLLMVision.String()]
	cfg.TokenizerPath = files[malinamodels.RoleTokenizer.String()]
	cfg.HighNoiseDiffusionModelPath = files[malinamodels.RoleHighNoise.String()]
	cfg.EmbeddingsConnectorsPath = files[malinamodels.RoleEmbeddingsConn.String()]
	cfg.VAEPath = files[malinamodels.RoleVAE.String()]
	cfg.TAESDPath = files[malinamodels.RoleTAESD.String()]
	cfg.ControlNetPath = files[malinamodels.RoleControlNet.String()]
	cfg.MotionModulePath = files[malinamodels.RoleMotionModule.String()]
	cfg.ADetailerPath = files[malinamodels.RoleADetailer.String()]
	cfg.PhotoMakerPath = files[malinamodels.RolePhotoMaker.String()]
	cfg.AudioEncoderPath = files[malinamodels.RoleAudioEncoder.String()]

	return cfg, nil
}

func (sd *StableDiffusion) modelSize(modelID string) (int64, error) {
	path, err := sd.models.FullPath(modelID)
	if err != nil {
		return 0, fmt.Errorf("model-size: full path: %w", err)
	}

	var modelSize int64
	for i, size := range path.FileSizes {
		if size <= 0 {
			return 0, fmt.Errorf("model-size: model-id[%s]: file[%d]: missing file size", modelID, i)
		}
		modelSize += size
	}
	if modelSize == 0 {
		return 0, fmt.Errorf("model-size: model-id[%s]: no model files", modelID)
	}

	return modelSize, nil
}
