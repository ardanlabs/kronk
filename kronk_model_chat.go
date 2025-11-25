package kronk

import (
	"context"
	"fmt"
	"strings"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func (m *model) chat(ctx context.Context, messages []ChatMessage, params Params) (string, error) {
	ch := m.chatStreaming(ctx, messages, params)

	var finalResponse strings.Builder

	for msg := range ch {
		if msg.Err != nil {
			return "", fmt.Errorf("error from model: %w", msg.Err)
		}

		finalResponse.WriteString(msg.Response)
	}

	return finalResponse.String(), nil
}

func (m *model) chatStreaming(ctx context.Context, messages []ChatMessage, params Params) <-chan ChatResponse {
	ch := make(chan ChatResponse)

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				ch <- ChatResponse{Err: fmt.Errorf("%s", rec)}
			}

			close(ch)
		}()

		lctx, err := llama.InitFromModel(m.model, m.ctxParams)
		if err != nil {
			ch <- ChatResponse{Err: fmt.Errorf("unable to init from model: %w", err)}
			return
		}

		defer func() {
			llama.Synchronize(lctx)
			llama.Free(lctx)
		}()

		prompt := m.applyChatTemplate(messages)
		m.processTokens(ctx, lctx, modeChat, prompt, params, ch)
	}()

	return ch
}

func (m *model) applyChatTemplate(messages []ChatMessage) string {
	msgs := make([]llama.ChatMessage, len(messages))
	for i, msg := range messages {
		msgs[i] = llama.NewChatMessage(msg.Role, msg.Content)
	}

	buf := make([]byte, m.cfg.ContextWindow)
	l := llama.ChatApplyTemplate(m.template, msgs, true, buf)

	return string(buf[:l])
}
