package lfm_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	if len(testlib.MPLFMChat.ModelFiles) == 0 {
		fmt.Println("model LFM2-700M-Q8_0 not downloaded, skipping lfm tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
