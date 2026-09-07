package hybrid_vision_imc_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	// The model is base-tier, so a hosted runner does have it, but this
	// suite is interleaved multi-modal inference: it needs the GPU fleets,
	// not a small CPU-only box.
	if os.Getenv("KRONK_TEST_HOSTED") != "" {
		fmt.Println("skipping hybrid_vision_imc tests on a hosted runner")
		os.Exit(0)
	}

	testlib.Setup()

	if len(testlib.MPHybridVision.ModelFiles) == 0 {
		fmt.Println("model Qwopus3.5-4B-Coder.Q4_K_M not downloaded, skipping hybrid_vision_imc tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
