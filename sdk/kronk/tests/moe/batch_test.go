package moe_test

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func testBatchChatConcurrent(t *testing.T, krn *kronk.Kronk) {
	// Exercise MoE routing with distinct histories and oversubscribed slots,
	// reusing the main suite's loaded, speculation-disabled instance.
	testlib.TestChatIsolation(t, krn, []string{"ALPHA", "BRAVO", "COPPER"}, false)
}
