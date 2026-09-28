package kronk

import (
	"context"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func TestDecisionReportsUndetectedProtocol(t *testing.T) {
	krn := Kronk{
		modelInfo: model.ModelInfo{ID: "renamed-model"},
	}

	_, err := krn.Decision(context.Background(), model.DecisionRequest{})
	if err == nil {
		t.Fatal("Decision: got nil error, want undetected protocol error")
	}

	for _, fragment := range []string{"renamed-model", "model.WithDecisionProtocol(...)"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("Decision error %q does not contain %q", err, fragment)
		}
	}
}
