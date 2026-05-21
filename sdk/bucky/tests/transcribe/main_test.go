package transcribe_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/bucky/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	if len(testlib.MPTinyEn.ModelFiles) == 0 {
		fmt.Println("model tiny.en not downloaded, skipping transcribe tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
