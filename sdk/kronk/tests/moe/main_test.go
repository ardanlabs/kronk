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
		fmt.Println("model Qwen3-VL-30B-A3B-Instruct-Q8_0 not downloaded, skipping moe tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
