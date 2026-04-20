// DO NOT CHANGE THIS CODE WITHOUT TALKING TO BILL FIRST!
// THIS CODE IS WORKING WELL WITH TOOL CALLING CONSISTENCY.

package model

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/ardanlabs/jinja"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

func (m *Model) applyRequestJinjaTemplate(ctx context.Context, d D) (string, [][]byte, error) {
	switch m.projFile {
	case "":
		// Text-only: pass D directly to the jinja engine.
		prompt, err := m.applyJinjaTemplate(ctx, d)
		if err != nil {
			return "", nil, err
		}
		return prompt, nil, nil

	default:
		// Media models: extract []byte content and replace with markers.
		// The input d is already cloned by prepareMediaContext, so mutation is safe.
		var media [][]byte
		if msgs, ok := d["messages"].([]D); ok {
			for _, doc := range msgs {
				if content, exists := doc["content"]; exists {
					if value, ok := content.([]byte); ok {
						media = append(media, value)
						doc["content"] = fmt.Sprintf("%s\n", mtmd.DefaultMarker())
					}
				}
			}
		}

		prompt, err := m.applyJinjaTemplate(ctx, d)
		if err != nil {
			return "", nil, err
		}

		return prompt, media, nil
	}
}

func (m *Model) applyJinjaTemplate(ctx context.Context, d map[string]any) (string, error) {
	messages, _ := d["messages"].([]D)
	m.log(ctx, "applyJinjaTemplate", "template", m.template.FileName, "messages", len(messages))

	if m.template.Script == "" {
		return "", errors.New("apply-jinja-template: no template found")
	}

	// Compile template once and reuse across all requests.
	m.templateOnce.Do(func() {
		tmpl, err := jinja.Compile(m.template.Script)
		m.compiledTmpl = &compiledTemplate{tmpl: tmpl, err: err}
	})

	if m.compiledTmpl.err != nil {
		return "", fmt.Errorf("apply-jinja-template: failed to parse template: %w", m.compiledTmpl.err)
	}

	// Ensure add_generation_prompt is set (default true if not specified).
	// This tells the Jinja template to append the assistant role prefix at the
	// end of the prompt, signaling the model to generate a response. When caching
	// messages (SPC or IMC), we set this to false so the cached tokens form a valid
	// prefix that can be extended with additional messages in subsequent requests.
	if _, ok := d["add_generation_prompt"]; !ok {
		d["add_generation_prompt"] = true
	}

	// Ensure enable_thinking is set. If the model config specifies a default
	// (e.g., enable_thinking: false in sampling-parameters), use that. Otherwise
	// default to true. When using the SDK directly, callers may not set this
	// field, but templates (e.g., gemma-4.jinja) rely on it to control whether
	// reasoning is enabled.
	if _, ok := d["enable_thinking"]; !ok {
		if m.cfg.DefaultParams.Thinking != "" {
			d["enable_thinking"] = m.cfg.DefaultParams.Thinking
		} else {
			d["enable_thinking"] = true
		}
	}

	// Normalize enable_thinking to a bool so Jinja "is false" / "is true" tests
	// work correctly. The value may arrive as a string ("true"/"false") from the
	// CLI or catalog config, but templates require a real boolean.
	if v, ok := d["enable_thinking"]; ok {
		switch val := v.(type) {
		case string:
			d["enable_thinking"] = val == "true"
		}
	}

	// Provide bos_token and eos_token from the model vocabulary. Templates
	// like gemma-4 require these to produce a valid prompt. When the tokenizer
	// already prepends BOS (addBOSToken=true), we set bos_token to empty to
	// avoid a double-BOS that corrupts inference.
	if _, ok := d["bos_token"]; !ok {
		if m.addBOSToken {
			d["bos_token"] = ""
		} else {
			d["bos_token"] = tokenText(m.vocab, llama.VocabBOS(m.vocab))
		}
	}
	if _, ok := d["eos_token"]; !ok {
		d["eos_token"] = tokenText(m.vocab, llama.VocabEOS(m.vocab))
	}

	s, err := m.compiledTmpl.tmpl.Render(d)
	if err != nil {
		return "", fmt.Errorf("apply-jinja-template: failed to execute template: %w", err)
	}

	return s, nil
}

// tokenText converts a token ID to its string representation.
func tokenText(vocab llama.Vocab, token llama.Token) string {
	buf := make([]byte, 128)
	n := llama.TokenToPiece(vocab, token, buf, 0, true)
	if n < 0 {
		return ""
	}
	return string(buf[:n])
}

func readJinjaTemplate(fileName string) (string, error) {
	data, err := os.ReadFile(fileName)
	if err != nil {
		return "", fmt.Errorf("read-jinja-template: failed to read file: %w", err)
	}

	return string(data), nil
}
