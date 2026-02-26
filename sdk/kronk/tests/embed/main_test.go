package embed_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	if len(testlib.MPEmbed.ModelFiles) == 0 {
		fmt.Println("model embeddinggemma-300m-qat-Q8_0 not downloaded, skipping embed tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
