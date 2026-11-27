package mtp

import (
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// Backend describes an MTP architecture's loading and KV ownership contract.
// Artifact location is independent of runtime behavior: embedded and companion
// Qwen heads both use own-KV synchronization.
type Backend struct {
	architecture  string
	artifact      speculation.MTPArtifact
	sharedKV      bool
	wideEmbedding bool
}

// BackendForPlan resolves only supported architecture/artifact combinations.
func BackendForPlan(plan speculation.Plan) (Backend, error) {
	switch plan.MTPArchitecture {
	case speculation.MTPArchitectureQwen35OwnKV:
		switch plan.MTPArtifact {
		case speculation.MTPArtifactEmbedded, speculation.MTPArtifactCompanion:
			return Backend{architecture: "qwen35", artifact: plan.MTPArtifact}, nil
		}
	case speculation.MTPArchitectureQwen4ExpOwnKV:
		switch plan.MTPArtifact {
		case speculation.MTPArtifactEmbedded, speculation.MTPArtifactCompanion:
			return Backend{architecture: "qwen4exp", artifact: plan.MTPArtifact, wideEmbedding: true}, nil
		}
	case speculation.MTPArchitectureNemotronOwnKV:
		if plan.MTPArtifact == speculation.MTPArtifactEmbedded {
			return Backend{architecture: "nemotron", artifact: plan.MTPArtifact}, nil
		}
	case speculation.MTPArchitectureGemmaSharedKV:
		if plan.MTPArtifact == speculation.MTPArtifactCompanion {
			return Backend{architecture: "gemma", artifact: plan.MTPArtifact, sharedKV: true}, nil
		}
	}
	return Backend{}, fmt.Errorf("unsupported MTP architecture %s artifact %s", plan.MTPArchitecture, plan.MTPArtifact)
}

// Name returns the architecture and KV ownership name used for diagnostics.
func (b Backend) Name() string {
	if b.sharedKV {
		return "gemma-shared-kv"
	}
	return b.architecture + "-own-kv"
}

// SharedKV reports whether the draft context aliases the target's memory.
func (b Backend) SharedKV() bool { return b.sharedKV }

// FixedDraftPosition reports whether draft steps reuse the target KV position.
func (b Backend) FixedDraftPosition() bool { return b.sharedKV }

func (b Backend) embeddingWidth(model llama.Model) int {
	if b.wideEmbedding {
		return int(llama.ModelNEmbdOut(model))
	}
	return int(llama.ModelNEmbd(model))
}

// ownKVContextParams matches an own-KV MTP context to the target execution and
// positional layout, with no recurrent rollback and one output per sequence.
func ownKVContextParams(params, target llama.ContextParams) llama.ContextParams {
	params.CtxType = llama.ContextTypeMTP
	params.NCtx = target.NCtx
	params.NBatch = target.NBatch
	params.NUbatch = target.NUbatch
	params.NSeqMax = target.NSeqMax
	params.NRsSeq = 0
	params.NOutputsMax = target.NSeqMax
	params.NOutputsMaxPerSeq = 1
	params.NThreads = target.NThreads
	params.NThreadsBatch = target.NThreadsBatch
	params.FlashAttentionType = target.FlashAttentionType
	params.TypeK = target.TypeK
	params.TypeV = target.TypeV
	params.Offload_kqv = target.Offload_kqv
	params.OpOffload = target.OpOffload
	params.KVUnified = target.KVUnified
	params.SwaFull = target.SwaFull
	params.RopeScalingType = target.RopeScalingType
	params.RopeFreqBase = target.RopeFreqBase
	params.RopeFreqScale = target.RopeFreqScale
	params.YarnExtFactor = target.YarnExtFactor
	params.YarnAttnFactor = target.YarnAttnFactor
	params.YarnBetaFast = target.YarnBetaFast
	params.YarnBetaSlow = target.YarnBetaSlow
	params.YarnOrigCtx = target.YarnOrigCtx
	return params
}

func sharedContextParams(params, target llama.ContextParams, targetCtx llama.Context) llama.ContextParams {
	params.CtxType = llama.ContextTypeMTP
	params.CtxOther = targetCtx
	params.NCtx = target.NCtx
	params.NBatch = target.NBatch
	params.NUbatch = target.NUbatch
	params.NSeqMax = target.NSeqMax
	params.NThreads = target.NThreads
	params.NThreadsBatch = target.NThreadsBatch
	params.FlashAttentionType = target.FlashAttentionType
	params.TypeK = target.TypeK
	params.TypeV = target.TypeV
	params.Offload_kqv = target.Offload_kqv
	params.OpOffload = target.OpOffload
	// These define the borrowed cache topology and must match the target.
	params.KVUnified = target.KVUnified
	params.SwaFull = target.SwaFull
	return params
}
