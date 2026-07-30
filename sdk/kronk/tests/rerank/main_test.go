package rerank_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	if len(testlib.MPRerankBatchSeq.ModelFiles) == 0 {
		fmt.Println("reranker model not downloaded, skipping rerank tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
