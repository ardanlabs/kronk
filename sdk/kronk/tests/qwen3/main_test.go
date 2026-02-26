package qwen3_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	if len(testlib.MPThinkToolChat.ModelFiles) == 0 {
		fmt.Println("model Qwen3-8B-Q8_0 not downloaded, skipping qwen3 tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
