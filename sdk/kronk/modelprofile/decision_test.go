package modelprofile

import (
	"strings"
	"testing"
)

func TestResolveDecisionFacts(t *testing.T) {
	profile := Resolve(map[string]string{
		"general.architecture":                    "modern-bert",
		"modern-bert.decision.type":               "laya",
		"modern-bert.decision.max_head_tokens":    "256",
		"modern-bert.decision.temperature.choice": "0.25",
	})

	if profile.Decision.Type != "laya" {
		t.Errorf("Type: got %q, want %q", profile.Decision.Type, "laya")
	}
	maxHeadTokens, err := profile.Decision.MaxHeadTokens()
	if err != nil {
		t.Fatalf("MaxHeadTokens: %v", err)
	}
	if maxHeadTokens != 256 {
		t.Errorf("MaxHeadTokens: got %d, want 256", maxHeadTokens)
	}
	temperature, exists, err := profile.Decision.Temperature("choice")
	if err != nil {
		t.Fatalf("Temperature: %v", err)
	}
	if !exists || temperature != 0.25 {
		t.Errorf("Temperature: got %v/%t, want 0.25/true", temperature, exists)
	}
}

func TestDecisionFactsRejectInvalidValues(t *testing.T) {
	profile := Resolve(map[string]string{
		"general.architecture":                    "modern-bert",
		"modern-bert.decision.max_head_tokens":    "invalid",
		"modern-bert.decision.temperature.choice": "0",
	})

	if _, err := profile.Decision.MaxHeadTokens(); err == nil || !strings.Contains(err.Error(), `invalid max_head_tokens "invalid"`) {
		t.Fatalf("MaxHeadTokens error: got %v, want invalid value", err)
	}
	if _, exists, err := profile.Decision.Temperature("choice"); !exists || err == nil || !strings.Contains(err.Error(), `modern-bert.decision.temperature.choice="0"`) {
		t.Fatalf("Temperature error: got exists=%t err=%v, want invalid value", exists, err)
	}
	if _, exists, err := profile.Decision.Temperature("score"); exists || err != nil {
		t.Fatalf("missing Temperature: got exists=%t err=%v, want false/nil", exists, err)
	}
}
