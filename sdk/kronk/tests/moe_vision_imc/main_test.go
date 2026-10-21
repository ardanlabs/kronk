package moe_vision_imc_test

import (
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	testlib.RequireFixtures(testlib.MPMoEVision.ModelFiles...)
	testlib.RequireFixtures(testlib.MPMoEVision.ProjFile, testlib.ImageFile)

	os.Exit(m.Run())
}
