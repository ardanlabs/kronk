package mtp_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	if len(testlib.MPMTP.ModelFiles) == 0 {
		fmt.Println("model mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL not downloaded, skipping mtp tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
