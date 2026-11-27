package mtp

import "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"

// VerifyInput contains callbacks into target sampling and output processing.
type VerifyInput = speculation.MatchInput

// VerifyResult contains the accepted prefix and target bonus token.
type VerifyResult = speculation.MatchResult

// Verify uses MTP's target-sample matching policy. MTP has no draft distribution;
// each proposal must match the request sampler at the corresponding target row.
func Verify(input VerifyInput) VerifyResult {
	return speculation.VerifyMatches(input)
}
