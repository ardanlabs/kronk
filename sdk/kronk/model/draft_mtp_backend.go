package model

import (
	"context"
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	yzmaspec "github.com/hybridgroup/yzma/exp/speculative"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// mtpBackend owns the architecture-specific MTP loading and runtime contract.
// Artifact location is independent from runtime behavior: embedded and
// companion Qwen35 heads both select the same own-KV semantics.
type mtpBackend interface {
	name() string
	sharedKV() bool
	fixedDraftPosition() bool
	load(mtpLoadRequest) (drafter, error)
}

// mtpBackendDrafter is implemented by every MTP drafter. Runtime code asks the
// selected backend for architecture behavior instead of inspecting a concrete
// drafter type.
type mtpBackendDrafter interface {
	drafter
	mtpBackend() mtpBackend
}

type mtpLoadRequest struct {
	ctx             context.Context
	log             applog.Logger
	cfg             Config
	targetCtx       llama.Context
	targetModel     llama.Model
	targetCtxParams llama.ContextParams
}

type qwen35OwnKVBackend struct {
	artifact mtpArtifact
}

func (qwen35OwnKVBackend) name() string             { return "qwen35-own-kv" }
func (qwen35OwnKVBackend) sharedKV() bool           { return false }
func (qwen35OwnKVBackend) fixedDraftPosition() bool { return false }
func (b qwen35OwnKVBackend) load(req mtpLoadRequest) (drafter, error) {
	switch b.artifact {
	case mtpArtifactEmbedded:
		nLayers := mtpNextNLayers(req.targetModel)
		if nLayers == 0 {
			req.log(req.ctx, "draft-model-mtp", "status", "auto-detect-skipped",
				"backend", b.name(), "reason", "no nextn_predict_layers metadata in target GGUF")
			return nil, nil
		}
		if !yzmaspec.Available() {
			const reason = "target GGUF declares MTP (nextn_predict_layers>0) but the loaded llama library does not export the NextN hidden-state APIs required by Yzma. MTP speculative decoding is DISABLED for this model. Install the llama.cpp version pinned for this Kronk release."
			req.log(req.ctx, "draft-model-mtp", "status", "DISABLED",
				"backend", b.name(), "nextn-layers", nLayers, "reason", reason)
			fmt.Fprintf(os.Stderr, "WARN: MTP DISABLED for this model: %s\n", reason)
			return nil, nil
		}

		d, err := loadDraftModelMTP(req.ctx, req.log, req.targetCtx, req.targetModel, req.targetCtxParams, mtpNDraft(req.cfg))
		if err != nil {
			return nil, err
		}
		d.b = b
		source := "auto-detected"
		if req.cfg.PtrDraftModel != nil && !req.cfg.PtrDraftModel.IsSeparate() {
			source = "auto-detected-configured"
		}
		req.log(req.ctx, "draft-model-mtp", "status", "loaded",
			"backend", b.name(), "source", source,
			"nDraft", d.c.nDraft, "nextn-layers", nLayers,
			"nEmbd", d.c.mtp.EmbeddingSize(), "nCtx", llama.NCtx(d.c.lctx))
		return d, nil

	case mtpArtifactCompanion:
		if !yzmaspec.Available() {
			const reason = "MTPDrafterFile is a Qwen own-KV MTP head but the loaded llama library does not export the NextN hidden-state APIs required by Yzma. MTP speculative decoding is DISABLED for this model."
			req.log(req.ctx, "draft-model-mtp-separate", "status", "DISABLED", "backend", b.name(), "reason", reason)
			fmt.Fprintf(os.Stderr, "WARN: MTP DISABLED for this model: %s\n", reason)
			return nil, nil
		}

		d, err := loadDraftModelMTPSeparate(req.ctx, req.log, req.cfg, req.targetCtx, req.targetModel, req.targetCtxParams, mtpNDraft(req.cfg))
		if err != nil {
			return nil, err
		}
		d.b = b
		req.log(req.ctx, "draft-model-mtp-separate", "status", "loaded",
			"backend", b.name(), "source", "mtp-drafter-file", "file", req.cfg.MTPDrafterFile,
			"nDraft", d.c.nDraft, "nEmbd", d.c.mtp.EmbeddingSize(), "nCtx", llama.NCtx(d.c.lctx))
		return d, nil
	}

	return nil, fmt.Errorf("qwen35 MTP: unsupported artifact %d", b.artifact)
}

type gemmaSharedKVBackend struct{}

func (gemmaSharedKVBackend) name() string             { return "gemma-shared-kv" }
func (gemmaSharedKVBackend) sharedKV() bool           { return true }
func (gemmaSharedKVBackend) fixedDraftPosition() bool { return true }
func (b gemmaSharedKVBackend) load(req mtpLoadRequest) (drafter, error) {
	if !yzmaspec.Available() {
		const reason = "MTPDrafterFile is a gemma4-assistant MTP head but the loaded llama library does not export the NextN hidden-state APIs required by Yzma. MTP speculative decoding is DISABLED for this model."
		req.log(req.ctx, "draft-model-mtp-shared", "status", "DISABLED", "backend", b.name(), "reason", reason)
		fmt.Fprintf(os.Stderr, "WARN: MTP DISABLED for this model: %s\n", reason)
		return nil, nil
	}

	d, err := loadDraftModelMTPShared(req.ctx, req.log, req.cfg, req.targetCtx, req.targetModel, req.targetCtxParams, mtpNDraft(req.cfg))
	if err != nil {
		return nil, err
	}
	d.b = b
	req.log(req.ctx, "draft-model-mtp-shared", "status", "loaded",
		"backend", b.name(), "source", "mtp-drafter-file", "file", req.cfg.MTPDrafterFile,
		"nDraft", d.c.nDraft, "nEmbd", d.c.mtp.EmbeddingSize(), "nCtx", llama.NCtx(d.c.lctx))
	return d, nil
}

func mtpBackendForPlan(plan speculationPlan) (mtpBackend, error) {
	switch plan.MTPArchitecture {
	case mtpArchitectureQwen35OwnKV:
		switch plan.MTPArtifact {
		case mtpArtifactEmbedded, mtpArtifactCompanion:
			return qwen35OwnKVBackend{artifact: plan.MTPArtifact}, nil
		}
	case mtpArchitectureGemmaSharedKV:
		if plan.MTPArtifact == mtpArtifactCompanion {
			return gemmaSharedKVBackend{}, nil
		}
	}

	return nil, fmt.Errorf("unsupported MTP architecture %d artifact %d", plan.MTPArchitecture, plan.MTPArtifact)
}

func selectedMTPBackend(d drafter) (mtpBackend, bool) {
	mtpDraft, ok := d.(mtpBackendDrafter)
	if !ok || mtpDraft.mtpBackend() == nil {
		return nil, false
	}
	return mtpDraft.mtpBackend(), true
}

func mtpUsesSharedKV(d drafter) bool {
	backend, ok := selectedMTPBackend(d)
	return ok && backend.sharedKV()
}

func mtpUsesFixedDraftPosition(d drafter) bool {
	backend, ok := selectedMTPBackend(d)
	return ok && backend.fixedDraftPosition()
}
