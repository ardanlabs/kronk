package arch

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// GLM5Next normalizes GLM5 Next attention, recurrent state, and MTP metadata.
type GLM5Next struct{}

// Name returns the adapter's diagnostic name.
func (GLM5Next) Name() string { return "glm5-next" }

// Claims reports whether the architecture uses this adapter.
func (GLM5Next) Claims(architecture string) bool {
	return strings.EqualFold(architecture, "glm5-next")
}

// Apply normalizes per-layer attention, recurrent state, and MTP dimensions.
func (GLM5Next) Apply(metadata Metadata, profile *modelprofile.Profile) error {
	profile.MemorySemantics = modelprofile.MemoryRecurrent
	if profile.Speculation.NextNPredictLayers > 0 {
		profile.Speculation.MTPArchitecture = modelprofile.MTPArchitectureGLM5Next
	}

	blockCount := profile.Dimensions.BlockCount
	if blockCount <= 0 {
		return nil
	}

	trunkLayers := max(blockCount-max(profile.Speculation.NextNPredictLayers, 0), 0)
	a := &profile.Attention
	a.RecurrentPattern = make([]bool, blockCount)
	a.RecurrentLayers = 0
	a.FullAttentionLayers = 0

	var fullHeadCountTotal int64
	for layer := range trunkLayers {
		if layer >= int64(len(a.HeadCountKV)) {
			continue
		}

		headCountKV := a.HeadCountKV[layer]
		if headCountKV == 0 {
			a.RecurrentPattern[layer] = true
			a.RecurrentLayers++
			continue
		}

		a.FullAttentionLayers++
		fullHeadCountTotal += headCountKV
	}
	if a.FullAttentionLayers > 0 {
		a.FullHeadCountKV = fullHeadCountTotal / a.FullAttentionLayers
	}

	arch := profile.Architecture
	convKernel, convErr := gguf.ParseInt64(metadata.values, arch+".ssm.conv_kernel")
	headDim, headDimErr := gguf.ParseInt64(metadata.values, arch+".kda.head_dim")
	headCount := profile.Dimensions.HeadCount
	if convErr != nil || headDimErr != nil || convKernel <= 0 || headDim <= 0 || headCount <= 0 {
		return nil
	}

	rWidth := 3 * (convKernel - 1) * (headCount * headDim)
	sWidth := headDim * headDim * headCount
	a.RecurrentStateBytes = 4 * (rWidth + sWidth)
	return nil
}
