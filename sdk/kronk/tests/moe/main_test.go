package moe_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	if len(testlib.MPMoEVision.ModelFiles) == 0 {
		fmt.Println("model gemma-4-26B-A4B-it-UD-Q4_K_M not downloaded, skipping moe tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
