package embed_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	if len(testlib.MPEmbedFallback.ModelFiles) == 0 && len(testlib.MPEmbedBatchSeq.ModelFiles) == 0 {
		fmt.Println("embedding models not downloaded, skipping embed tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
