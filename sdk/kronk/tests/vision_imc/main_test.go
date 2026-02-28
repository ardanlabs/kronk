package vision_imc_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		fmt.Println("skipping vision_imc tests in GitHub Actions")
		os.Exit(0)
	}

	testlib.Setup()

	if len(testlib.MPSimpleVision.ModelFiles) == 0 {
		fmt.Println("model Qwen2.5-VL-3B-Instruct-Q8_0 not downloaded, skipping vision_imc tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
