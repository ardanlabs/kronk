package chatapi_test

import (
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/security/auth"
	"github.com/ardanlabs/kronk/sdk/tools/security"
)

func createTokens(t *testing.T, sec *security.Security) map[string]string {
	tokens := make(map[string]string)

	token, err := sec.GenerateToken(true, nil, 60*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	tokens["admin"] = token

	// -------------------------------------------------------------------------

	token, err = sec.GenerateToken(true, nil, 60*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	tokens["non-admin-no-endpoints"] = token

	// -------------------------------------------------------------------------

	endpoints := map[string]auth.RateLimit{
		"chat-completions": {
			Limit:  0,
			Window: auth.RateUnlimited,
		},
	}

	token, err = sec.GenerateToken(false, endpoints, 60*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	tokens["chat-completions"] = token

	// -------------------------------------------------------------------------

	endpoints = map[string]auth.RateLimit{
		"embeddings": {
			Limit:  0,
			Window: auth.RateUnlimited,
		},
	}

	token, err = sec.GenerateToken(false, endpoints, 60*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	tokens["embeddings"] = token

	return tokens
}
