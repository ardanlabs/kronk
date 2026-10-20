package hybrid_test

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func testBatchChatConcurrent(t *testing.T, krn *kronk.Kronk) {
	// Like the Qwen batch test, this hits the known HIP defect where two
	// sequences sharing one llama_decode batch can return corrupted logits.
	// https://github.com/ggml-org/llama.cpp/issues/28537
	testlib.SkipOnBackends(t, "two sequences sharing one decode batch come back with corrupted logits", "rocm")

	// Three distinct histories oversubscribe two decode slots without
	// exceeding the shared suite's default four-request admission capacity.
	testlib.TestChatIsolation(t, krn, []string{"ALPHA", "BRAVO", "COPPER"}, false)
}
