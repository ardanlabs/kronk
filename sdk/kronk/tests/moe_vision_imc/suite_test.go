package moe_vision_imc_test

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestSuite(t *testing.T) {
	testlib.WithModel(t, testlib.CfgMoEVisionIMC(), func(t *testing.T, krn *kronk.Kronk) {
		testlib.TestModelProfile(t, krn, "MoE", "Attention-only", "gemma4")
		t.Run("MediaState", func(t *testing.T) { testlib.TestMediaState(t, krn) })
	})
}
