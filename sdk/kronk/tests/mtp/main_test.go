package mtp_test

import (
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	testlib.RequireFixtures(testlib.MPMTP.ModelFiles...)

	os.Exit(m.Run())
}
