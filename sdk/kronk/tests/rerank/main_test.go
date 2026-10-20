package rerank_test

import (
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestMain(m *testing.M) {
	testlib.Setup()

	testlib.RequireFixtures(testlib.MPRerankBatchSeq.ModelFiles...)

	os.Exit(m.Run())
}
