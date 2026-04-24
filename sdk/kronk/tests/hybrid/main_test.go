package hybrid_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	if len(testlib.MPHybridVision.ModelFiles) == 0 {
		fmt.Println("model Qwen3.6-35B-A3B-UD-Q4_K_M not downloaded, skipping hybrid tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
