package model

import (
	"context"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
)

// withCleanRegistry saves and restores the package-level registry so each
// test starts from a known empty state without leaking factories into
// neighboring tests.
func withCleanRegistry(t *testing.T) {
	t.Helper()
	saved := registeredProcessors
	registeredProcessors = nil
	t.Cleanup(func() {
		registeredProcessors = saved
	})
}

// fakeProcessor is a minimal Processor used to verify registry dispatch
// without pulling in real processor packages (which would create import
// cycles).
type fakeProcessor struct {
	name string
}

func (f fakeProcessor) Name() string                  { return f.name }
func (f fakeProcessor) NewStateMachine() StateMachine { return nil }
func (f fakeProcessor) ParseToolCall(_ context.Context, _ applog.Logger, _ string) []ResponseToolCall {
	return nil
}

// claimingFactory builds a factory that claims a fingerprint when its match
// substring appears in the model name (or returns false for "" match).
func claimingFactory(name, match string) ProcessorFactory {
	return func(fp Fingerprint) (Processor, bool) {
		if match != "" && containsLower(fp.ModelName, match) {
			return fakeProcessor{name: name}, true
		}
		return nil, false
	}
}

// catchAllFactory always claims; used to exercise the standard-processor
// fallback path.
func catchAllFactory(name string) ProcessorFactory {
	return func(_ Fingerprint) (Processor, bool) {
		return fakeProcessor{name: name}, true
	}
}

// containsLower is a tiny helper that mimics the lowercase-substring match
// each real processor does on the model name.
func containsLower(s, substr string) bool {
	if substr == "" {
		return false
	}
	// inline strings.Contains(strings.ToLower(s), substr) without imports
	for i := 0; i+len(substr) <= len(s); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			c := s[i+j]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// TestRegisterProcessor_AppendsInOrder verifies registrations preserve order.
func TestRegisterProcessor_AppendsInOrder(t *testing.T) {
	withCleanRegistry(t)

	RegisterProcessor(claimingFactory("a", ""))
	RegisterProcessor(claimingFactory("b", ""))
	RegisterProcessor(claimingFactory("c", ""))

	if got := len(registeredProcessors); got != 3 {
		t.Fatalf("len(registeredProcessors) = %d, want 3", got)
	}
}

// TestSelectProcessor_FirstClaimerWins verifies registration order determines
// which processor is selected when multiple could claim.
func TestSelectProcessor_FirstClaimerWins(t *testing.T) {
	withCleanRegistry(t)

	// Both "qwen" and the catch-all would claim a Qwen model, but qwen is
	// registered first.
	RegisterProcessor(claimingFactory("qwen", "qwen"))
	RegisterProcessor(catchAllFactory("standard"))

	got := selectProcessor(Fingerprint{ModelName: "Qwen3-Coder-30B"})
	if got == nil {
		t.Fatal("selectProcessor returned nil, want qwen")
	}
	if got.Name() != "qwen" {
		t.Errorf("selected = %q, want qwen", got.Name())
	}
}

// TestSelectProcessor_FallsThroughToCatchAll verifies an unknown model lands
// on the last-registered catch-all.
func TestSelectProcessor_FallsThroughToCatchAll(t *testing.T) {
	withCleanRegistry(t)

	RegisterProcessor(claimingFactory("qwen", "qwen"))
	RegisterProcessor(claimingFactory("mistral", "mistral"))
	RegisterProcessor(catchAllFactory("standard"))

	got := selectProcessor(Fingerprint{ModelName: "Llama-3-8B-Instruct"})
	if got == nil {
		t.Fatal("selectProcessor returned nil, want standard")
	}
	if got.Name() != "standard" {
		t.Errorf("selected = %q, want standard", got.Name())
	}
}

// TestSelectProcessor_NoClaimsReturnsNil verifies the no-catch-all case
// returns nil rather than panicking.
func TestSelectProcessor_NoClaimsReturnsNil(t *testing.T) {
	withCleanRegistry(t)

	RegisterProcessor(claimingFactory("qwen", "qwen"))
	// Intentionally NO catch-all.

	got := selectProcessor(Fingerprint{ModelName: "Llama-3"})
	if got != nil {
		t.Errorf("selectProcessor on unmatched fingerprint = %+v, want nil", got)
	}
}

// TestSelectProcessor_EmptyRegistry verifies an unconfigured registry yields
// nil rather than panicking.
func TestSelectProcessor_EmptyRegistry(t *testing.T) {
	withCleanRegistry(t)

	if got := selectProcessor(Fingerprint{}); got != nil {
		t.Errorf("selectProcessor on empty registry = %+v, want nil", got)
	}
}

// TestSelectProcessor_RegistrationOrderMattersForOverlap verifies that if two
// factories both claim the same model, the earlier-registered one wins.
func TestSelectProcessor_RegistrationOrderMattersForOverlap(t *testing.T) {
	withCleanRegistry(t)

	// Both factories would claim "qwen-coder" — first wins.
	RegisterProcessor(claimingFactory("first", "qwen"))
	RegisterProcessor(claimingFactory("second", "qwen"))

	got := selectProcessor(Fingerprint{ModelName: "qwen-coder"})
	if got == nil {
		t.Fatal("selectProcessor returned nil")
	}
	if got.Name() != "first" {
		t.Errorf("selected = %q, want first", got.Name())
	}
}
