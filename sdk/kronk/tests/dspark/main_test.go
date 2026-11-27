package dspark_test

import (
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	testlib.RequireFixtures(testlib.MPDSpark.ModelFiles...)
	testlib.RequireFixtures(testlib.MPDSpark.DraftFile)

	os.Exit(m.Run())
}
