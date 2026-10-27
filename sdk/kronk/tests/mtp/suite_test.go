// Package mtp_test exercises embedded Qwen own-KV MTP speculation.
package mtp_test

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestSuite(t *testing.T) {
	testlib.TestSpeculation(t, testlib.CfgMTPChat(), "qwen35-own-kv", "draft-model-mtp", "recurrent-native")
}

func TestSuiteMultiSlot(t *testing.T) {
	testlib.SkipOnBackends(t, "two sequences sharing one decode batch come back with corrupted logits", "rocm")
	testlib.TestSpeculation(t, testlib.CfgMTPChatMultiSlot(), "qwen35-own-kv", "draft-model-mtp", "recurrent-native")
}
