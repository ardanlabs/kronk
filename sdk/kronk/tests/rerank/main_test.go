package rerank_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	if len(testlib.MPRerank.ModelFiles) == 0 {
		fmt.Println("model bge-reranker-v2-m3-Q8_0 not downloaded, skipping rerank tests")
		os.Exit(0)
	}

	os.Exit(m.Run())
}
