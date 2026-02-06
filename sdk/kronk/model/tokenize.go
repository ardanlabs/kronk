package model

import (
	"context"
	"fmt"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// Tokenize returns the token count for a text input.
//
// Supported options in d:
//   - input (string): the text to tokenize (required)
//   - apply_template (bool): if true, wrap input as a user message and apply
//     the model's chat template before tokenizing (default: false)
//   - add_generation_prompt (bool): when apply_template is true, controls whether
//     the assistant role prefix is appended to the prompt (default: true)
//
// When apply_template is true, the returned count includes all template
// overhead (role markers, separators, generation prompt). This reflects the
// actual number of tokens that would be fed to the model.
func (m *Model) Tokenize(ctx context.Context, d D) (TokenizeResponse, error) {
	input, ok := d["input"].(string)
	if !ok || input == "" {
		return TokenizeResponse{}, fmt.Errorf("tokenize: missing or invalid input parameter (expected non-empty string)")
	}

	text := input

	applyTemplate, _ := d["apply_template"].(bool)

	if applyTemplate {
		templateCtx := D{
			"messages": []D{
				{
					"role":    RoleUser,
					"content": input,
				},
			},
		}

		if v, exists := d["add_generation_prompt"]; exists {
			templateCtx["add_generation_prompt"] = v
		}

		prompt, err := m.applyJinjaTemplate(ctx, templateCtx)
		if err != nil {
			return TokenizeResponse{}, fmt.Errorf("tokenize: apply-template: %w", err)
		}

		text = prompt
	}

	tokens := llama.Tokenize(m.vocab, text, m.addBOSToken, true)

	tr := TokenizeResponse{
		Object:  "tokenize",
		Created: time.Now().Unix(),
		Model:   m.modelInfo.ID,
		Tokens:  len(tokens),
	}

	return tr, nil
}
