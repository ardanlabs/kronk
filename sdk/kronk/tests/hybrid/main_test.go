package hybrid_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	if len(testlib.MPHybridChat.ModelFiles) == 0 {
		fmt.Println("model Qwen3-Coder-Next-Q4_0 not downloaded, skipping hybrid tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
