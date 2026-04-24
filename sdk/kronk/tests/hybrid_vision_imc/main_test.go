package hybrid_vision_imc_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		fmt.Println("skipping hybrid_vision_imc tests in GitHub Actions")
		os.Exit(0)
	}

	testlib.Setup()

	if len(testlib.MPHybridVision.ModelFiles) == 0 {
		fmt.Println("model Qwen3.6-35B-A3B-UD-Q4_K_M not downloaded, skipping hybrid_vision_imc tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
