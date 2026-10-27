package model

import "testing"

func TestInputCapabilities(t *testing.T) {
	tests := []struct {
		name   string
		known  bool
		native MTMDHelperCapabilities
		info   ModelInfo
		want   InputCapabilities
	}{
		{"unavailable", false, MTMDHelperCapabilities{Chat: true, Vision: true}, ModelInfo{}, InputCapabilities{Chat: true, Vision: true}},
		{"text", true, MTMDHelperCapabilities{Chat: true}, ModelInfo{}, InputCapabilities{Available: true, Chat: true}},
		{"vision", true, MTMDHelperCapabilities{Chat: true, Vision: true}, ModelInfo{}, InputCapabilities{Available: true, Chat: true, Vision: true}},
		{"audio", true, MTMDHelperCapabilities{Chat: true, Audio: true}, ModelInfo{}, InputCapabilities{Available: true, Chat: true, Audio: true}},
		{"combined", true, MTMDHelperCapabilities{Chat: true, Vision: true, Audio: true}, ModelInfo{}, InputCapabilities{Available: true, Chat: true, Vision: true, Audio: true}},
		{"no chat", true, MTMDHelperCapabilities{TextGen: true, Video: true, AudioGen: true}, ModelInfo{}, InputCapabilities{Available: true}},
		{"embedding", true, MTMDHelperCapabilities{Chat: true}, ModelInfo{IsEmbedModel: true}, InputCapabilities{Available: true}},
		{"reranking", true, MTMDHelperCapabilities{Chat: true}, ModelInfo{IsRerankModel: true}, InputCapabilities{Available: true}},
		{"decision", true, MTMDHelperCapabilities{Chat: true}, ModelInfo{IsDecisionModel: true}, InputCapabilities{Available: true}},
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
