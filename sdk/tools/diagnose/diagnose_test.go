package diagnose

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCaptureHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	cmd := capture(ctx, commandSpec{name: os.Args[0], args: []string{"-test.run=^$"}})
	if !strings.Contains(cmd.Err, context.Canceled.Error()) {
		t.Fatalf("capture error: got %q, want context cancellation", cmd.Err)
	}
}
