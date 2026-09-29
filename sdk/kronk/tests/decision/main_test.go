package decision_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	if len(testlib.MPDecision.ModelFiles) == 0 {
		fmt.Println("decision model not downloaded, skipping decision tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
