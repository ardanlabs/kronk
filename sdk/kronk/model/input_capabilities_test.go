package model

import (
	"testing"

	"github.com/hybridgroup/yzma/pkg/mtmd"
)

func TestInputCapabilities(t *testing.T) {
	tests := []struct {
		name   string
		known  bool
		native mtmd.HelperCap
		info   ModelInfo
		want   InputCapabilities
	}{
		{"unavailable", false, mtmd.HelperCap{Chat: true, Vision: true}, ModelInfo{}, InputCapabilities{Chat: true, Vision: true}},
		{"text", true, mtmd.HelperCap{Chat: true}, ModelInfo{}, InputCapabilities{Available: true, Chat: true}},
		{"vision", true, mtmd.HelperCap{Chat: true, Vision: true}, ModelInfo{}, InputCapabilities{Available: true, Chat: true, Vision: true}},
		{"audio", true, mtmd.HelperCap{Chat: true, Audio: true}, ModelInfo{}, InputCapabilities{Available: true, Chat: true, Audio: true}},
		{"combined", true, mtmd.HelperCap{Chat: true, Vision: true, Audio: true}, ModelInfo{}, InputCapabilities{Available: true, Chat: true, Vision: true, Audio: true}},
		{"no chat", true, mtmd.HelperCap{TextGen: true, Video: true, AudioGen: true}, ModelInfo{}, InputCapabilities{Available: true}},
		{"embedding", true, mtmd.HelperCap{Chat: true}, ModelInfo{IsEmbedModel: true}, InputCapabilities{Available: true}},
		{"reranking", true, mtmd.HelperCap{Chat: true}, ModelInfo{IsRerankModel: true}, InputCapabilities{Available: true}},
		{"decision", true, mtmd.HelperCap{Chat: true}, ModelInfo{IsDecisionModel: true}, InputCapabilities{Available: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{mtmdCapsKnown: tt.known, mtmdCapabilities: tt.native, modelInfo: tt.info}
			if got := m.InputCapabilities(); got != tt.want {
				t.Errorf("InputCapabilities: got %+v, want %+v", got, tt.want)
			}
		})
	}
}
