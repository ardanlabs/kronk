package models

import "github.com/ardanlabs/kronk/sdk/kronk/gguf"

// MoEInfo contains Mixture of Experts metadata extracted from GGUF files.
// Field shape mirrors gguf.MoEInfo; this models-side type keeps the
// package's public surface stable without leaking the gguf import.
type MoEInfo struct {
	IsMoE            bool
	ExpertCount      int64
	ExpertUsedCount  int64
	HasSharedExperts bool
}

// detectMoE extracts Mixture of Experts information from GGUF metadata
// via the shared gguf parser and translates the result back into the
// models-side MoEInfo struct.
func detectMoE(metadata map[string]string) MoEInfo {
	g := gguf.DetectMoE(metadata)
	return MoEInfo{
		IsMoE:            g.IsMoE,
		ExpertCount:      g.ExpertCount,
		ExpertUsedCount:  g.ExpertUsedCount,
		HasSharedExperts: g.HasSharedExperts,
	}
}
