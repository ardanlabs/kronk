package gguf

import "fmt"

// fileTypeNames maps the GGUF general.file_type integer to a
// human-readable quantization name. These values come from the
// llama.cpp LLAMA_FTYPE_* enum.
var fileTypeNames = map[int64]string{
	0:  "F32",
	1:  "F16",
	2:  "Q4_0",
	3:  "Q4_1",
	7:  "Q8_0",
	8:  "Q5_0",
	9:  "Q5_1",
	10: "Q2_K",
	11: "Q3_K_S",
	12: "Q3_K_M",
	13: "Q3_K_L",
	14: "Q4_K_S",
	15: "Q4_K_M",
	16: "Q5_K_S",
	17: "Q5_K_M",
	18: "Q6_K",
	19: "IQ2_XXS",
	20: "IQ2_XS",
	21: "Q2_K_S",
	22: "IQ3_XS",
	23: "IQ3_XXS",
	24: "IQ1_S",
	25: "IQ4_NL",
	26: "IQ3_S",
	27: "IQ3_M",
	28: "IQ2_S",
	29: "IQ2_M",
	30: "IQ4_XS",
	31: "IQ1_M",
	32: "BF16",
	36: "TQ1_0",
	37: "TQ2_0",
	38: "MXFP4_MOE",
	39: "NVFP4",
	40: "Q1_0",
	41: "Q2_0",
}

// FileTypeName returns the human-readable name for the given GGUF
// general.file_type integer, or "unknown(N)" when the value is not in the
// lookup table.
func FileTypeName(ft int64) string {
	if name, ok := fileTypeNames[ft]; ok {
		return name
	}

	return fmt.Sprintf("unknown(%d)", ft)
}
