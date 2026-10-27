package vision_imc_test

import (
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	testlib.RequireFixtures(testlib.MPSimpleVision.ModelFiles...)
	testlib.RequireFixtures(testlib.MPSimpleVision.ProjFile, testlib.ImageFile)

	os.Exit(m.Run())
}
