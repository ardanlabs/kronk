// This TestMain resolves MPMTP through the catalog and exits 0 when it is
// absent. The kronkdiff differential harness resolves its GGUF by explicit
// path instead and must not be short-circuited by that skip, so it supplies
// its own setup and this file is excluded from that build.
//go:build !kronkdiff

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
		fmt.Println("model mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL not downloaded, skipping mtp tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
