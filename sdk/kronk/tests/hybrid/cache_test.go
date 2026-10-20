package hybrid_test

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func Test_CacheIMCHybrid(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		name := "IMCOff"
		if enabled {
			name = "IMCOn"
		}
		t.Run(name, func(t *testing.T) {
			cfg := testlib.CfgHybridChat()
			cfg.PtrNSeqMax = new(1)
			cfg.PtrIncrementalCache = new(enabled)
			cfg.PtrCacheMinTokens = new(1)
			cfg.Speculation = model.SpeculationDisabled
			testlib.WithModel(t, cfg, func(t *testing.T, krn *kronk.Kronk) {
				testlib.TestModelProfile(t, krn, "Dense", "Hybrid", "qwen35")
				testlib.TestIMCState(t, krn, enabled)
			})
		})
	}
}

func Test_CacheIMCHybridMultiSlot(t *testing.T) {
	cfg := testlib.CfgHybridChat()
	cfg.PtrIncrementalCache = new(true)
	cfg.PtrCacheMinTokens = new(1)
	cfg.Speculation = model.SpeculationDisabled
	testlib.WithModel(t, cfg, func(t *testing.T, krn *kronk.Kronk) {
		testlib.TestModelProfile(t, krn, "Dense", "Hybrid", "qwen35")
		testlib.TestChatIsolation(t, krn, []string{"ALPHA", "BRAVO", "COPPER"}, true)
	})
}
