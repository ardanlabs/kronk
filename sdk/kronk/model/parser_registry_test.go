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
	saved := registeredParsers
	registeredParsers = nil
	t.Cleanup(func() {
		registeredParsers = saved
	})
}

// fakeParser is a minimal Parser used to verify registry dispatch
// without pulling in real parser packages (which would create import
// cycles).
type fakeParser struct {
	name string
}

func (f fakeParser) Name() string                  { return f.name }
func (f fakeParser) NewStateMachine() StateMachine { return nil }
func (f fakeParser) ToolCall(_ context.Context, _ applog.Logger, _ string) []ResponseToolCall {
	return nil
}

// claimingFactory builds a factory that claims a fingerprint when its match
// substring appears in the model name (or returns false for "" match).
func claimingFactory(name, match string) ParserFactory {
	return func(fp Fingerprint) (Parser, bool) {
		if match != "" && containsLower(fp.ModelName, match) {
			return fakeParser{name: name}, true
		}
		return nil, false
	}
}

// catchAllFactory always claims; used to exercise the standard-parser
// fallback path.
func catchAllFactory(name string) ParserFactory {
	return func(_ Fingerprint) (Parser, bool) {
		return fakeParser{name: name}, true
	}
}

// containsLower is a tiny helper that mimics the lowercase-substring match
// each real parser does on the model name.
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

// TestRegisterParser_AppendsInOrder verifies registrations preserve order.
func TestRegisterParser_AppendsInOrder(t *testing.T) {
	withCleanRegistry(t)

	RegisterParser(claimingFactory("a", ""))
	RegisterParser(claimingFactory("b", ""))
	RegisterParser(claimingFactory("c", ""))

	if got := len(registeredParsers); got != 3 {
		t.Fatalf("len(registeredParsers) = %d, want 3", got)
	}
}

// TestSelectParser_FirstClaimerWins verifies registration order determines
// which parser is selected when multiple could claim.
func TestSelectParser_FirstClaimerWins(t *testing.T) {
	withCleanRegistry(t)

	// Both "qwen" and the catch-all would claim a Qwen model, but qwen is
	// registered first.
	RegisterParser(claimingFactory("qwen", "qwen"))
	RegisterParser(catchAllFactory("standard"))

	got := selectParser(Fingerprint{ModelName: "Qwen3-Coder-30B"})
	if got == nil {
		t.Fatal("selectParser returned nil, want qwen")
	}
	if got.Name() != "qwen" {
		t.Errorf("selected = %q, want qwen", got.Name())
	}
}

// TestSelectParser_FallsThroughToCatchAll verifies an unknown model lands
// on the last-registered catch-all.
func TestSelectParser_FallsThroughToCatchAll(t *testing.T) {
	withCleanRegistry(t)

	RegisterParser(claimingFactory("qwen", "qwen"))
	RegisterParser(claimingFactory("mistral", "mistral"))
	RegisterParser(catchAllFactory("standard"))

	got := selectParser(Fingerprint{ModelName: "Llama-3-8B-Instruct"})
	if got == nil {
		t.Fatal("selectParser returned nil, want standard")
	}
	if got.Name() != "standard" {
		t.Errorf("selected = %q, want standard", got.Name())
	}
}

// TestSelectParser_NoClaimsReturnsNil verifies the no-catch-all case
// returns nil rather than panicking.
func TestSelectParser_NoClaimsReturnsNil(t *testing.T) {
	withCleanRegistry(t)

	RegisterParser(claimingFactory("qwen", "qwen"))
	// Intentionally NO catch-all.

	got := selectParser(Fingerprint{ModelName: "Llama-3"})
	if got != nil {
		t.Errorf("selectParser on unmatched fingerprint = %+v, want nil", got)
	}
}

// TestSelectParser_EmptyRegistry verifies an unconfigured registry yields
// nil rather than panicking.
func TestSelectParser_EmptyRegistry(t *testing.T) {
	withCleanRegistry(t)

	if got := selectParser(Fingerprint{}); got != nil {
		t.Errorf("selectParser on empty registry = %+v, want nil", got)
	}
}

// TestSelectParser_RegistrationOrderMattersForOverlap verifies that if two
// factories both claim the same model, the earlier-registered one wins.
func TestSelectParser_RegistrationOrderMattersForOverlap(t *testing.T) {
	withCleanRegistry(t)

	// Both factories would claim "qwen-coder" — first wins.
	RegisterParser(claimingFactory("first", "qwen"))
	RegisterParser(claimingFactory("second", "qwen"))

	got := selectParser(Fingerprint{ModelName: "qwen-coder"})
	if got == nil {
		t.Fatal("selectParser returned nil")
	}
	if got.Name() != "first" {
		t.Errorf("selected = %q, want first", got.Name())
	}
}
