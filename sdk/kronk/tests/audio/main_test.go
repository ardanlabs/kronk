package audio_test

import (
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	testlib.RequireFixtures(testlib.MPAudio.ModelFiles...)
	testlib.RequireFixtures(testlib.MPAudio.ProjFile, testlib.AudioFile)

	os.Exit(m.Run())
}
