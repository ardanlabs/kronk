// DO NOT CHANGE THIS CODE WITHOUT TALKING TO BILL FIRST!
// THIS CODE IS WORKING WELL WITH TOOL CALLING CONSISTENCY.

package model

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"strings"

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
		//
		// This function is PURE with respect to the caller-supplied D and its
		// inner message maps. Earlier versions mutated each doc["content"] in
		// place, which caused subtle bugs when the same message map was
		// rendered more than once: the second pass would find no []byte
		// (already replaced with marker text) but the rendered prompt would
		// still contain the markers, producing a marker/bitmap mismatch and
		// mtmd.Tokenize returning code 1.
		msgs, ok := d["messages"].([]D)
		if !ok {
			prompt, err := m.applyJinjaTemplate(ctx, d)
			if err != nil {
				return "", nil, err
			}
			return prompt, nil, nil
		}

		marker := fmt.Sprintf("%s\n", mtmd.DefaultMarker())
		defaultMarker := mtmd.DefaultMarker()

		var media [][]byte
		renderMsgs := make([]D, len(msgs))

		for i, doc := range msgs {
			content, hasContent := doc["content"]
			if !hasContent {
				renderMsgs[i] = doc
				continue
			}

			switch value := content.(type) {
			case []byte:
				// Single media payload — emit one marker, capture bytes.
				media = append(media, value)
				renderMsgs[i] = cloneDocWithContent(doc, marker)

			case []any:
				// Normalized multipart parts (strings and []byte). Flatten
				// to a single string for the template, emitting one marker
				// per []byte part in original order. The number of markers
				// emitted is provably equal to the number of media buffers
				// appended.
				rendered, msgMedia, err := renderNormalizedParts(value, marker, defaultMarker, i)
				if err != nil {
					return "", nil, err
				}
				media = append(media, msgMedia...)
				renderMsgs[i] = cloneDocWithContent(doc, rendered)

			case string:
				// Reject literal marker text in non-media content. A user
				// supplying a string that already contains the media marker
				// would inflate the prompt's marker count without providing
				// a matching bitmap, causing mtmd.Tokenize to fail.
				if strings.Contains(value, defaultMarker) {
					return "", nil, fmt.Errorf("apply-request-jinja-template: message[%d] content contains reserved media marker %q", i, defaultMarker)
				}
				renderMsgs[i] = doc

			default:
				renderMsgs[i] = doc
			}
		}

		// Shallow-clone the top-level D so add_generation_prompt / bos_token /
		// eos_token mutations from applyJinjaTemplate do not leak back to the
		// caller.
		renderD := make(D, len(d))
		maps.Copy(renderD, d)
		renderD["messages"] = renderMsgs

		prompt, err := m.applyJinjaTemplate(ctx, renderD)
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
	// messages (IMC), we set this to false so the cached tokens form a valid
	// prefix that can be extended with additional messages in subsequent requests.
	if _, ok := d["add_generation_prompt"]; !ok {
		d["add_generation_prompt"] = true
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

// cloneDocWithContent returns a shallow clone of doc with content replaced.
// Used to avoid mutating the caller's message map when injecting media markers.
func cloneDocWithContent(doc D, content any) D {
	out := make(D, len(doc))
	maps.Copy(out, doc)
	out["content"] = content
	return out
}

// renderNormalizedParts flattens a normalized multipart content slice (the
// output of toMediaMessage for messages with interleaved text and media) into
// a single string suitable for Jinja templating, while collecting all media
// payloads in original order.
//
// Invariant: the returned string contains exactly one marker per []byte part
// in parts, and the returned media slice has exactly that many entries in the
// same order. This guarantees the marker count emitted into the prompt equals
// the bitmap count fed to mtmd.Tokenize.
func renderNormalizedParts(parts []any, marker, defaultMarker string, msgIdx int) (string, [][]byte, error) {
	var b strings.Builder
	var media [][]byte

	for j, part := range parts {
		switch v := part.(type) {
		case string:
			if strings.Contains(v, defaultMarker) {
				return "", nil, fmt.Errorf("apply-request-jinja-template: message[%d].content[%d] contains reserved media marker %q", msgIdx, j, defaultMarker)
			}
			b.WriteString(v)

		case []byte:
			b.WriteString(marker)
			media = append(media, v)

		default:
			return "", nil, fmt.Errorf("apply-request-jinja-template: message[%d].content[%d] has unsupported part type %T", msgIdx, j, part)
		}
	}

	return b.String(), media, nil
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
