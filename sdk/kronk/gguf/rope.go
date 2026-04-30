package gguf

// RopeFacts contains RoPE (Rotary Position Embedding) configuration
// extracted from GGUF metadata.
type RopeFacts struct {
	FreqBase    float64
	FreqScale   float64
	ScalingType string
	OriginalCtx int64
	DimCount    int64
}

// ParseRopeFacts extracts the RoPE configuration for the given
// architecture from GGUF metadata. Missing fields default to their
// zero values.
func ParseRopeFacts(metadata map[string]string, arch string) RopeFacts {
	var r RopeFacts

	if v, err := ParseFloat64(metadata, arch+".rope.freq_base"); err == nil {
		r.FreqBase = v
	}

	if v, err := ParseFloat64(metadata, arch+".rope.freq_scale"); err == nil {
		r.FreqScale = v
	}

	if v, ok := metadata[arch+".rope.scaling.type"]; ok {
		r.ScalingType = v
	} else if v, ok := metadata["rope_scaling"]; ok {
		r.ScalingType = v
	}

	if v, err := ParseInt64(metadata, arch+".rope.scaling.original_context_length"); err == nil {
		r.OriginalCtx = v
	}

	if v, err := ParseInt64(metadata, arch+".rope.dimension_count"); err == nil {
		r.DimCount = v
	}

	return r
}
