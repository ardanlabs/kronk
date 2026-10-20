// Package gemma4mtp_test exercises separate-file Gemma shared-KV MTP speculation.
package gemma4mtp_test

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestSuite(t *testing.T) {
	testlib.TestSpeculation(t, testlib.CfgGemma4MTPChat(), "gemma-shared-kv", "draft-model-mtp-shared", "attention-suffix")
}

func TestSuiteMultiSlot(t *testing.T) {
	testlib.SkipOnBackends(t, "two sequences sharing one decode batch come back with corrupted logits", "rocm")
	testlib.TestSpeculation(t, testlib.CfgGemma4MTPChatMultiSlot(), "gemma-shared-kv", "draft-model-mtp-shared", "attention-suffix")
}
