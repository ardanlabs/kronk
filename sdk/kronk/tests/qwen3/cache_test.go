package qwen3_test

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func Test_CacheIMCDeterministic(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		name := "IMCOff"
		if enabled {
			name = "IMCOn"
		}
		t.Run(name, func(t *testing.T) {
			cfg := testlib.CfgThinkToolChat()
			cfg.PtrNSeqMax = new(1)
			cfg.PtrIncrementalCache = new(enabled)
			cfg.PtrCacheMinTokens = new(1)
			cfg.Speculation = model.SpeculationDisabled
			testlib.WithModel(t, cfg, func(t *testing.T, krn *kronk.Kronk) {
				testlib.TestModelProfile(t, krn, "Dense", "Attention-only", "qwen3")
				testlib.TestIMCState(t, krn, enabled)
			})
		})
	}
}

func Test_CacheIMCDeterministicMultiSlot(t *testing.T) {
	// Three warmed conversations exceed two decode slots, but fit the default
	// four-session admission capacity. Each conversation has a distinct prefix.
	cfg := testlib.CfgThinkToolChat()
	cfg.PtrIncrementalCache = new(true)
	cfg.PtrCacheMinTokens = new(1)
	cfg.Speculation = model.SpeculationDisabled
	testlib.WithModel(t, cfg, func(t *testing.T, krn *kronk.Kronk) {
		testlib.TestModelProfile(t, krn, "Dense", "Attention-only", "qwen3")
		testlib.TestChatIsolation(t, krn, []string{"ALPHA", "BRAVO", "COPPER"}, true)
	})
}
