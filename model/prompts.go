package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"github.com/nikolalohinski/gonja/v2"
	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/loaders"
)

func (m *Model) applyChatRequestJinjaTemplate(cr ChatRequest, addAssistantPrompt bool) (string, error) {
	// If the call to process the jinja template fails, we fall back to the
	// default template if we are working with a GPT model. We currently have
	// a bug processing GPT based jinja templates.

	t, err := m.applyJinjaTemplate(cr.Messages, cr.Tools, addAssistantPrompt)
	if err != nil {
		if m.modelInfo.IsGPT {
			return m.applyDefaultTemplate(cr.Messages), nil
		}

		return "", err
	}

	return t, nil
}

func (m *Model) applyVisionRequestJinjaTemplate(vr VisionRequest, addAssistantPrompt bool) (string, error) {
	// If we don't add the mtmd marker, the model will not be able to understand
	// the vision request.
	messages := []ChatMessage{
		vr.Message,
		{
			Role:    "user",
			Content: mtmd.DefaultMarker(),
		},
	}

	t, err := m.applyJinjaTemplate(messages, nil, addAssistantPrompt)
	if err != nil {
		if m.modelInfo.IsGPT {
			return m.applyDefaultTemplate(messages), nil
		}

		return "", err
	}

	return t, nil
}

func (m *Model) applyJinjaTemplate(messages []ChatMessage, tools []Tool, addAssistantPrompt bool) (string, error) {
	if m.template == "" {
		return "", errors.New("no template found")
	}

	gonja.DefaultLoader = &noFSLoader{}

	t, err := gonja.FromString(m.template)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	jsonData, err := json.Marshal(messages)
	if err != nil {
		return "", fmt.Errorf("failed to marshal messages: %w", err)
	}

	var msgs []map[string]any
	if err := json.Unmarshal(jsonData, &msgs); err != nil {
		return "", fmt.Errorf("failed to unmarshal messages: %w", err)
	}

	var toolCalls []map[string]any
	if len(tools) > 0 {
		jsonData, err = json.Marshal(tools)
		if err != nil {
			return "", fmt.Errorf("failed to marshal tools: %w", err)
		}

		if err := json.Unmarshal(jsonData, &toolCalls); err != nil {
			return "", fmt.Errorf("failed to unmarshal tools: %w", err)
		}
	}

	data := exec.NewContext(map[string]any{
		"messages":              msgs,
		"tools":                 toolCalls,
		"add_generation_prompt": addAssistantPrompt,
	})

	s, err := t.ExecuteToString(data)
	if err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return s, nil
}

func (m *Model) applyDefaultTemplate(messages []ChatMessage) string {
	msgs := make([]llama.ChatMessage, len(messages))
	for i, msg := range messages {
		msgs[i] = llama.NewChatMessage(msg.Role, msg.Content)
	}

	buf := make([]byte, m.cfg.ContextWindow)
	l := llama.ChatApplyTemplate(m.template, msgs, true, buf)

	return string(buf[:l])
}

// =============================================================================

type noFSLoader struct{}

func (nl *noFSLoader) Read(path string) (io.Reader, error) {
	return nil, errors.New("filesystem access disabled")
}

func (nl *noFSLoader) Resolve(path string) (string, error) {
	return "", errors.New("filesystem access disabled")
}

func (nl *noFSLoader) Inherit(from string) (loaders.Loader, error) {
	return nil, errors.New("filesystem access disabled")
}

// =============================================================================

func readJinjaTemplate(fileName string) (string, error) {
	data, err := os.ReadFile(fileName)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return string(data), nil
}
