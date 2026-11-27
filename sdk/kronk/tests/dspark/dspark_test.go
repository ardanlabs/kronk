// Package dspark_test exercises Inkling2 with an automatically selected DSpark sidecar.
package dspark_test

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestSuite(t *testing.T) {
	testlib.TestSpeculation(t, testlib.CfgDSparkChat(), "dspark", "draft-model-dspark", "attention-suffix")
}

func TestSuiteMultiSlot(t *testing.T) {
	testlib.SkipOnBackends(t, "two sequences sharing one decode batch come back with corrupted logits", "rocm")
	testlib.TestSpeculation(t, testlib.CfgDSparkChatMultiSlot(), "dspark", "draft-model-dspark", "attention-suffix")
}
