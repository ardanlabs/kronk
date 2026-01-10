package model

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/hybridgroup/yzma/pkg/mtmd"
	"github.com/nikolalohinski/gonja/v2"
	"github.com/nikolalohinski/gonja/v2/builtins"
	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/loaders"
)

func (m *Model) applyRequestJinjaTemplate(ctx context.Context, d D) (string, [][]byte, error) {
	// We need to identify if there is media in the request. If there is
	// we want to replace the actual media with a media marker `<__media__>`.
	// We will move the media to it's own slice. The next call that will happen
	// is `processBitmap` which will process the prompt and media.

	var media [][]byte

	for _, doc := range d["messages"].([]D) {
		if content, exists := doc["content"]; exists {
			switch value := content.(type) {
			case []byte:
				media = append(media, value)
				doc["content"] = fmt.Sprintf("%s\n", mtmd.DefaultMarker())
			}
		}
	}

	prompt, err := m.applyJinjaTemplate(ctx, d)
	if err != nil {
		return "", nil, err
	}

	return prompt, media, nil
}

func (m *Model) applyJinjaTemplate(ctx context.Context, d D) (string, error) {
	m.log(ctx, "applyJinjaTemplate", "template", m.template.FileName)

	if m.template.Script == "" {
		return "", errors.New("apply-jinja-template: no template found")
	}

	gonja.DefaultLoader = &noFSLoader{}

	t, err := newTemplateWithFixedItems(m.template.Script)
	if err != nil {
		return "", fmt.Errorf("apply-jinja-template: failed to parse template: %w", err)
	}

	data := exec.NewContext(d)

	s, err := t.ExecuteToString(data)
	if err != nil {
		return "", fmt.Errorf("apply-jinja-template: failed to execute template: %w", err)
	}

	return s, nil
}

// =============================================================================

type noFSLoader struct{}

func (nl *noFSLoader) Read(path string) (io.Reader, error) {
	return nil, errors.New("no-fs-loader:filesystem access disabled")
}

func (nl *noFSLoader) Resolve(path string) (string, error) {
	return "", errors.New("no-fs-loader:filesystem access disabled")
}

func (nl *noFSLoader) Inherit(from string) (loaders.Loader, error) {
	return nil, errors.New("no-fs-loader:filesystem access disabled")
}

// =============================================================================

// newTemplateWithFixedItems creates a gonja template with a fixed items() method
// that properly returns key-value pairs (the built-in one only returns values).
func newTemplateWithFixedItems(source string) (*exec.Template, error) {
	rootID := fmt.Sprintf("root-%s", string(sha256.New().Sum([]byte(source))))

	loader, err := loaders.NewFileSystemLoader("")
	if err != nil {
		return nil, err
	}

	shiftedLoader, err := loaders.NewShiftedLoader(rootID, bytes.NewReader([]byte(source)), loader)
	if err != nil {
		return nil, err
	}

	// Create custom environment with fixed items() method
	customContext := builtins.GlobalFunctions.Inherit()
	customContext.Set("add_generation_prompt", true)
	customContext.Set("strftime_now", func(format string) string {
		return time.Now().Format("2006-01-02")
	})
	customContext.Set("raise_exception", func(msg string) (string, error) {
		return "", errors.New(msg)
	})

	customFilters := builtins.Filters.Update(exec.NewFilterSet(map[string]exec.FilterFunction{}))
	customFilters.Register("items", func(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
		if !in.IsDict() {
			return exec.AsValue([][]any{})
		}
		dict := in.ToGoSimpleType(false)
		if m, ok := dict.(map[string]any); ok {
			items := make([][]any, 0, len(m))
			for key, value := range m {
				items = append(items, []any{key, value})
			}
			return exec.AsValue(items)
		}
		return exec.AsValue([][]any{})
	})

	env := exec.Environment{
		Context:           customContext,
		Filters:           customFilters,
		Tests:             builtins.Tests,
		ControlStructures: builtins.ControlStructures,
		Methods: exec.Methods{
			Dict: exec.NewMethodSet(map[string]exec.Method[map[string]any]{
				"keys": func(self map[string]any, selfValue *exec.Value, arguments *exec.VarArgs) (any, error) {
					if err := arguments.Take(); err != nil {
						return nil, err
					}
					keys := make([]string, 0, len(self))
					for key := range self {
						keys = append(keys, key)
					}
					sort.Strings(keys)
					return keys, nil
				},
				"items": func(self map[string]any, selfValue *exec.Value, arguments *exec.VarArgs) (any, error) {
					if err := arguments.Take(); err != nil {
						return nil, err
					}
					// Return [][]any where each inner slice is [key, value]
					// This allows gonja to unpack: for k, v in dict.items()
					items := make([][]any, 0, len(self))
					for key, value := range self {
						items = append(items, []any{key, value})
					}
					return items, nil
				},
			}),
			Str:   builtins.Methods.Str,
			List:  builtins.Methods.List,
			Bool:  builtins.Methods.Bool,
			Float: builtins.Methods.Float,
			Int:   builtins.Methods.Int,
		},
	}

	return exec.NewTemplate(rootID, gonja.DefaultConfig, shiftedLoader, &env)
}

func readJinjaTemplate(fileName string) (string, error) {
	data, err := os.ReadFile(fileName)
	if err != nil {
		return "", fmt.Errorf("read-jinja-template: failed to read file: %w", err)
	}

	return string(data), nil
}

// =============================================================================

func isOpenAIMediaMessage(d D) (bool, chatMessages, error) {
	msgs, err := toChatMessages(d)
	if err != nil {
		return false, chatMessages{}, fmt.Errorf("chat message conversion: %w", err)
	}

	for _, msg := range msgs.Messages {
		_, ok := msg.Content.([]chatMessageContent)
		if ok {
			return true, msgs, nil
		}
	}

	return false, chatMessages{}, nil
}

// convertToRawMediaMessage is needed because we want to use a raw media message
// format for processing media since we need the raw bytes.
func convertToRawMediaMessage(d D, msgs chatMessages) (D, error) {
	d, err := toMediaMessage(d, msgs)
	if err != nil {
		return nil, fmt.Errorf("media message conversion: %w", err)
	}

	return d, nil
}

func toMediaMessage(d D, msgs chatMessages) (D, error) {
	type mediaMessage struct {
		text string
		data []byte
	}

	var mediaMessages []mediaMessage

	var found int
	var mediaText string
	var mediaData string

	// -------------------------------------------------------------------------

	for _, msg := range msgs.Messages {
		switch content := msg.Content.(type) {
		case string:
			mediaMessages = append(mediaMessages, mediaMessage{
				text: content,
			})
			continue

		default:
			for _, cm := range msg.Content.([]chatMessageContent) {
				switch cm.Type {
				case "text":
					found++
					mediaText = cm.Text

				case "image_url":
					found++
					mediaData = cm.ImageURL.URL

				case "video_url":
					found++
					mediaData = cm.VideoURL.URL

				case "input_audio":
					found++
					mediaData = cm.AudioData.Data
				}

				if found == 2 {
					decoded, err := decodeMediaData(mediaData)
					if err != nil {
						return d, err
					}

					mediaMessages = append(mediaMessages, mediaMessage{
						text: mediaText,
						data: decoded,
					})

					found = 0
					mediaText = ""
					mediaData = ""
				}
			}
		}
	}

	// -------------------------------------------------------------------------

	// Here is take all the data we found (text, data) and convert everything
	// to the MediaMessage format is a generic format most model templates
	// support.

	docs := make([]D, 0, len(mediaMessages))

	for _, mm := range mediaMessages {
		if len(mm.data) > 0 {
			msgs := RawMediaMessage(mm.text, mm.data)
			docs = append(docs, msgs...)
			continue
		}

		docs = append(docs, TextMessage("user", mm.text))
	}

	d["messages"] = docs

	return d, nil
}

func decodeMediaData(data string) ([]byte, error) {
	if strings.HasPrefix(data, "http://") || strings.HasPrefix(data, "https://") {
		return nil, fmt.Errorf("to-media-message: URLs are not supported, provide base64 encoded data")
	}

	if idx := strings.Index(data, ";base64,"); idx != -1 && strings.HasPrefix(data, "data:") {
		data = data[idx+8:]
	}

	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("to-media-message: unable to decode base64 data: %w", err)
	}

	return decoded, nil
}
