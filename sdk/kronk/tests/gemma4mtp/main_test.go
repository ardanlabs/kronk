package gemma4mtp_test

import (
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	testlib.RequireFixtures(testlib.MPMoEVision.ModelFiles...)
	testlib.RequireFixtures(testlib.MPMoEVision.MTPFile)

	os.Exit(m.Run())
}
