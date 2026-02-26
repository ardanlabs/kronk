package audio_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	if len(testlib.MPAudio.ModelFiles) == 0 {
		fmt.Println("model Qwen2-Audio-7B.Q8_0 not downloaded, skipping audio tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
