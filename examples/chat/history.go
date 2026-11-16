package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // Register supported file decoders for validation.
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/ardanlabs/kronk/sdk/kronk/imc"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/charmbracelet/x/ansi"
)

// loadHistory reads chat messages, not checkpoint buffers. The next request
// lets IMC find and verify a compatible saved input prefix automatically.
func loadHistory(ctx context.Context, store imc.Store, ref imc.Ref) (request model.D, retErr error) {
	handle, err := store.Open(ctx, ref)
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, handle.Close()) }()
	record := handle.Record()
	if record.Version != 1 {
		return nil, imc.ErrVersion
	}
	var document map[string]any
	if err := json.Unmarshal(record.Request, &document); err != nil {
		return nil, err
	}
	// Convert all nested objects, including tools and image parts, to the
	// SDK's document types just as a fresh request does.
	request = model.MapToModelD(document)
	messages, _ := request["messages"].([]model.D)
	if len(messages) == 0 {
		return nil, errors.New("saved conversation has no messages")
	}
	request["messages"] = append(messages, model.D{"role": "assistant", "content": record.AssistantText})
	return request, nil
}

type attachment struct {
	name string
	data []byte
	kind string
}

// Embed the bytes in the request, so restoring never depends on the source
// file still existing. Limit file size before allocating or decoding it.
func readAttachment(path string) (attachment, error) {
	f, err := os.Open(path)
	if err != nil {
		return attachment{}, err
	}
	data, readErr := io.ReadAll(io.LimitReader(f, (8<<20)+1))
	if err := errors.Join(readErr, f.Close()); err != nil {
		return attachment{}, err
	}
	if len(data) > 8<<20 {
		return attachment{}, errors.New("image exceeds 8 MiB")
	}
	cfg, kind, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return attachment{}, fmt.Errorf("expected PNG or JPEG: %w", err)
	}
	if cfg.Width > 8192 || cfg.Height > 8192 || cfg.Width*cfg.Height > 32_000_000 {
		return attachment{}, errors.New("image exceeds 8192 pixels per side or 32 megapixels")
	}
	return attachment{name: filepath.Base(path), data: data, kind: kind}, nil
}

// Only display text and image labels, never base64 payloads. Avoid copying
// encoded images on every streaming refresh, including JSON-restored history.
func messageText(message model.D) string {
	if text, ok := message["content"].(string); ok {
		if message["role"] == "tool" {
			var result struct {
				Status string
				Data   map[string]any
			}
			if err := json.Unmarshal([]byte(text), &result); err == nil {
				return terminalText(result.Status + ": " + fmt.Sprint(result.Data))
			}
		}
		return terminalText(text)
	}
	var text []string
	appendPart := func(part map[string]any) {
		switch part["type"] {
		case "text":
			value, _ := part["text"].(string)
			text = append(text, terminalText(value))
		case "image_url":
			text = append(text, "[image attached]")
		}
	}
	switch parts := message["content"].(type) {
	case []model.D:
		for _, part := range parts {
			appendPart(part)
		}
	case []any:
		for _, part := range parts {
			if value, ok := part.(map[string]any); ok {
				appendPart(value)
			}
		}
	}
	return strings.Join(text, "\n")
}

// Model output, filenames, and stored previews are untrusted terminal input.
func terminalText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, ansi.Strip(s))
}
