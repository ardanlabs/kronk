package chatapi_test

import (
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/uuid"
)

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
