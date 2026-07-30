package embed_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	if len(testlib.MPEmbedBatchSeq.ModelFiles) == 0 {
		fmt.Println("embedding model not downloaded, skipping embed tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
