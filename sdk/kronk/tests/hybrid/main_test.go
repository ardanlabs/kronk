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
		fmt.Println("model Qwen_Qwen3.5-35B-A3B-Q8_0 not downloaded, skipping hybrid tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
