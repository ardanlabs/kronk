package questions

import (
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// Problems lists the combinations of answers the SDK refuses when it loads
// the model (validate-config in sdk/kronk/model/config.go). Each question is
// valid alone; these come from two answers together. The review shows them
// and doesn't save until they are fixed.
func Problems(mc models.ModelConfig) []string {
	var out []string

	switch mc.CacheTypeV {
	case model.GGMLTypeQ4_0, model.GGMLTypeQ4_1, model.GGMLTypeQ5_0, model.GGMLTypeQ5_1, model.GGMLTypeQ8_0:
		if mc.FlashAttention != nil && *mc.FlashAttention == model.FlashAttentionDisabled {
			out = append(out, fmt.Sprintf("A %s KV cache needs flash attention on or auto, not disabled.", mc.CacheTypeV))
		}
	}

	if mc.PtrIMCSessionCapacity != nil {
		nseq, qd := 1, model.DefaultQueueDepth
		if mc.PtrNSeqMax != nil {
			nseq = max(*mc.PtrNSeqMax, 1)
		}
		if mc.PtrQueueDepth != nil && *mc.PtrQueueDepth > 0 {
			qd = *mc.PtrQueueDepth
		}
		if *mc.PtrIMCSessionCapacity < nseq*qd {
			out = append(out, fmt.Sprintf("Cached conversations (%d) must be at least %d: requests at the same time × queue depth.",
				*mc.PtrIMCSessionCapacity, nseq*qd))
		}
	}

	if len(mc.TensorSplit) > 0 && len(mc.Devices) > 0 && len(mc.TensorSplit) != len(mc.Devices) {
		out = append(out, fmt.Sprintf("The GPU shares have %d numbers for %d selected GPUs; give one per GPU.", len(mc.TensorSplit), len(mc.Devices)))
	}

	if mc.ProjDevice != "" && mc.PtrProjOnCPU != nil && *mc.PtrProjOnCPU {
		out = append(out, "The image projector can't run on the CPU and on "+mc.ProjDevice+" at the same time.")
	}

	if mc.MoE != nil && mc.MoE.Mode.String() == "keep_top_n" && mc.MoE.PtrKeepExpertsOnGPUForTopNLayers == nil {
		out = append(out, "Keeping the top N layers' experts on the GPU needs the number of layers.")
	}

	return out
}
