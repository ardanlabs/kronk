package client

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"maps"
	"net/http"
	"os"
	"strconv"
)

type LLM struct {
	cln           *Client
	clnSSE        *SSEClient[ChatSSE]
	url           string
	model         string
	contextWindow int
}

func NewLLM(url string, model string) *LLM {
	contextWindow := 1024 * 4
	if v := os.Getenv("OLLAMA_CONTEXT_LENGTH"); v != "" {
		var err error
		contextWindow, err = strconv.Atoi(v)
		if err != nil {
			log.Fatal(err)
		}
	}

	return &LLM{
		cln:           New(FmtLogger),
		clnSSE:        NewSSE[ChatSSE](FmtLogger),
		url:           url,
		model:         model,
		contextWindow: contextWindow,
	}
}

type withParam struct {
	typ string
	d   D
}

func WithImage(mimeType string, image []byte) withParam {
	dataBase64 := base64.StdEncoding.EncodeToString(image)

	return withParam{
		typ: "image",
		d: D{
			"type": "image_url",
			"image_url": D{
				"url": fmt.Sprintf("data:%s;base64,%s", mimeType, dataBase64),
			},
		},
	}
}

func WithParams(temperature float32, topP float32, topK int) withParam {
	return withParam{
		typ: "params",
		d: D{
			"temperature": temperature,
			"top_p":       topP,
			"top_k":       topK,
		},
	}
}

func (llm *LLM) ChatCompletions(ctx context.Context, text string, options ...withParam) (string, error) {
	content := []D{
		{
			"type": "text",
			"text": text,
		},
	}

	params := D{
		"temperature": 1.0,
		"top_p":       0.5,
		"top_k":       20,
	}

	for _, opt := range options {
		switch opt.typ {
		case "image":
			content = append(content, opt.d)
		case "params":
			params = opt.d
		}
	}

	d := D{
		"model": llm.model,
		"messages": []D{
			{
				"role":    "user",
				"content": content,
			},
		},
		"max_tokens": llm.contextWindow,
	}

	maps.Copy(d, params)

	var chat Chat
	if err := llm.cln.Do(ctx, http.MethodPost, llm.url, d, &chat); err != nil {
		return "", fmt.Errorf("do: %w", err)
	}

	if len(chat.Choices) == 0 {
		return "", fmt.Errorf("no response")
	}

	return chat.Choices[0].Message.Content, nil
}

func (llm *LLM) ChatCompletionsSSE(ctx context.Context, content string) (chan ChatSSE, error) {
	d := D{
		"model": llm.model,
		"messages": []D{
			{
				"role":    "user",
				"content": content,
			},
		},
		"temperature": 1.0,
		"top_p":       0.5,
		"top_k":       20,
		"stream":      true,
		"max_tokens":  llm.contextWindow,
	}

	ch := make(chan ChatSSE, 100)
	if err := llm.clnSSE.Do(ctx, http.MethodPost, llm.url, d, ch); err != nil {
		return nil, fmt.Errorf("do: %w", err)
	}

	return ch, nil
}

func (llm *LLM) EmbedText(ctx context.Context, input string) ([]float64, error) {
	d := D{
		"model":              llm.model,
		"truncate":           true,
		"truncate_direction": "right",
		"input":              input,
	}

	var resp Embedding
	if err := llm.cln.Do(ctx, http.MethodPost, llm.url, d, &resp); err != nil {
		return nil, fmt.Errorf("do: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embedding")
	}

	return resp.Data[0].Embedding, nil
}

func (llm *LLM) EmbedWithImage(ctx context.Context, description string, image []byte, mimeType string) ([]float64, error) {
	dataBase64 := base64.StdEncoding.EncodeToString(image)

	d := D{
		"model": llm.model,
		"input": []D{
			{
				"type": "image_url",
				"image_url": D{
					"url": fmt.Sprintf("data:%s;base64,%s", mimeType, dataBase64),
				},
			},
		},
	}

	var resp Embedding
	if err := llm.cln.Do(ctx, http.MethodPost, llm.url, d, &resp); err != nil {
		return nil, fmt.Errorf("do: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embedding")
	}

	return resp.Data[0].Embedding, nil
}
