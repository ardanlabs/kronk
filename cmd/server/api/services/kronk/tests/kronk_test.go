package chatapi_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/security/auth"
	"github.com/ardanlabs/kronk/sdk/tools/security"
	"github.com/google/uuid"
)

func Test_API(t *testing.T) {
	test := apitest.New(t, "Test_API")

	tokens := createTokens(t, test.Sec)

	test.Run(t, chatNonStream200(tokens), "chatnonstream-200")
	test.RunStreaming(t, chatStream200(tokens), "chatstream-200")
	test.Run(t, chatEndpoint401(tokens), "chatEndpoint-401")

}

// =============================================================================

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

// =============================================================================

type responseValidator struct {
	resp   *model.ChatResponse
	errors []string
}

func validateResponse(got any) responseValidator {
	return responseValidator{resp: got.(*model.ChatResponse)}
}

func (v responseValidator) hasValidUUID() responseValidator {
	if _, err := uuid.Parse(v.resp.ID); err != nil {
		v.errors = append(v.errors, "expected id to be a UUID")
	}

	return v
}

func (v responseValidator) hasCreated() responseValidator {
	if v.resp.Created <= 0 {
		v.errors = append(v.errors, "expected created to be greater than 0")
	}

	return v
}

func (v responseValidator) hasPrompt() responseValidator {
	if v.resp.Prompt == "" {
		v.errors = append(v.errors, "expected to have a prompt")
	}

	return v
}

func (v responseValidator) hasUsage() responseValidator {
	u := v.resp.Usage

	if u.PromptTokens <= 0 {
		v.errors = append(v.errors, "expected prompt_tokens to be greater than 0")
	}

	if u.ReasoningTokens <= 0 {
		v.errors = append(v.errors, "expected reasoning_tokens to be greater than 0")
	}

	if u.CompletionTokens <= 0 {
		v.errors = append(v.errors, "expected completion_tokens to be greater than 0")
	}

	if u.OutputTokens <= 0 {
		v.errors = append(v.errors, "expected output_tokens to be greater than 0")
	}

	if u.TotalTokens <= 0 {
		v.errors = append(v.errors, "expected total_tokens to be greater than 0")
	}

	if u.TokensPerSecond <= 0 {
		v.errors = append(v.errors, "expected tokens_per_second to be greater than 0")
	}

	return v
}

func (v responseValidator) hasValidChoice() responseValidator {
	if len(v.resp.Choice) == 0 || v.resp.Choice[0].Index <= 0 {
		v.errors = append(v.errors, "expected index to be greater than 0")
	}

	return v
}

func (v responseValidator) hasContentOrReasoning() responseValidator {
	if len(v.resp.Choice) == 0 {
		v.errors = append(v.errors, "expected at least one choice")
		return v
	}

	if v.resp.Choice[0].Delta.Content == "" && v.resp.Choice[0].Delta.Reasoning == "" {
		v.errors = append(v.errors, "expected content or reasoning to be non-empty")
	}

	return v
}

func (v responseValidator) containsInContent(find string) responseValidator {
	if len(v.resp.Choice) == 0 {
		return v
	}

	if !strings.Contains(strings.ToLower(v.resp.Choice[0].Delta.Content), find) {
		v.errors = append(v.errors, fmt.Sprintf("expected to find %q in content", find))
	}

	return v
}

func (v responseValidator) containsInReasoning(find string) responseValidator {
	if len(v.resp.Choice) == 0 {
		return v
	}

	if !strings.Contains(strings.ToLower(v.resp.Choice[0].Delta.Reasoning), find) {
		v.errors = append(v.errors, fmt.Sprintf("expected to find %q in reasoning", find))
	}

	return v
}

func (v responseValidator) result() string {
	if len(v.errors) == 0 {
		return ""
	}

	return strings.Join(v.errors, "; ")
}
