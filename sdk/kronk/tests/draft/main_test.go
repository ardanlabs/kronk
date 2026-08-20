package draft_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	if len(testlib.MPThinkToolChat.ModelFiles) == 0 {
		fmt.Println("model Qwen3-1.7B-Q4_K_M not downloaded, skipping draft tests")
		os.Exit(0)
	}

	if len(testlib.MPDraft.ModelFiles) == 0 {
		fmt.Println("model Qwen3-0.6B-Q8_0 (draft) not downloaded, skipping draft tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
