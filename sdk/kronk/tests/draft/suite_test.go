// Package draft_test exercises classic separate-GGUF speculation.
package draft_test

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestSuite(t *testing.T) {
	testlib.TestSpeculation(t, testlib.CfgClassicDraftChat(), "classic-own-kv", "draft-model", "attention-suffix")
}
