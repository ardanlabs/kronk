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

func moeInfoFromGGUF(g gguf.MoEInfo) MoEInfo {
	return MoEInfo{
		IsMoE:            g.IsMoE,
		ExpertCount:      g.ExpertCount,
		ExpertUsedCount:  g.ExpertUsedCount,
		HasSharedExperts: g.HasSharedExperts,
	}
}
