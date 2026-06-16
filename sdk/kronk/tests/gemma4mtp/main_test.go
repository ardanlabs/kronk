package gemma4mtp_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	if len(testlib.MPMoEVision.ModelFiles) == 0 {
		fmt.Println("model gemma-4-26B-A4B-it-UD-Q4_K_M not downloaded, skipping gemma4 mtp tests")
		os.Exit(0)
	}

	if testlib.MPMoEVision.MTPFile == "" {
		fmt.Println("gemma-4-26B-A4B-it-UD-Q4_K_M has no co-located mtp-*.gguf assistant on disk, skipping gemma4 mtp tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
