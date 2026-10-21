package vision_imc_test

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestSuite(t *testing.T) {
	testlib.WithModel(t, testlib.CfgSimpleVisionIMC(), func(t *testing.T, krn *kronk.Kronk) {
		testlib.TestModelProfile(t, krn, "Dense", "Hybrid", "qwen35")
		t.Run("MediaState", func(t *testing.T) { testlib.TestMediaState(t, krn) })
	})
}
