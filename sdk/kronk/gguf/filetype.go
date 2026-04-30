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
	8:  "Q8_1",
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
	21: "IQ3_XXS",
	22: "IQ1_S",
	23: "IQ4_NL",
	24: "IQ3_S",
	25: "IQ2_S",
	26: "IQ4_XS",
	27: "IQ1_M",
	28: "BF16",
	29: "Q4_0_4_4",
	30: "Q4_0_4_8",
	31: "Q4_0_8_8",
	32: "TQ1_0",
	33: "TQ2_0",
}

// FileTypeName returns the human-readable name for the given GGUF
// general.file_type integer. Returns "" for ft == 0 (no quantization
// flag set), or "unknown(N)" when the value is not in the lookup
// table.
func FileTypeName(ft int64) string {
	if name, ok := fileTypeNames[ft]; ok {
		return name
	}

	if ft == 0 {
		return ""
	}

	return fmt.Sprintf("unknown(%d)", ft)
}
