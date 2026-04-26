package moe_vision_imc_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		fmt.Println("skipping moe_vision_imc tests in GitHub Actions")
		os.Exit(0)
	}

	testlib.Setup()

	if len(testlib.MPMoEVision.ModelFiles) == 0 {
		fmt.Println("model gemma-4-26B-A4B-it-UD-Q4_K_M not downloaded, skipping moe_vision_imc tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
