package model

import (
	"encoding/base64"
	"testing"
)

func Test_OpenAIToMediaMessage(t *testing.T) {
	// decodeMediaData validates magic bytes, so the payload must look like
	// a real supported format (JPEG header here).
	jpegHeader := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}
	data := append(jpegHeader, make([]byte, 64)...)
	openEncoded := base64.StdEncoding.EncodeToString(data)

	d := D{
		"messages": DocumentArray(
			openAIMediaMessage("what do you see in the picture?", openEncoded),
			D{
				"role":    "user",
				"content": "follow up question",
			},
		),
	}

	mediaType, isOpenAIFormat, chMsgs, err := detectMediaContent(d)
	if err != nil {
		t.Fatalf("detect-media: unable to check document: %s", err)
	}

	if mediaType == MediaTypeNone {
		t.Fatal("expected media to be detected")
	}

	if !isOpenAIFormat {
		t.Fatal("expected OpenAI format to be detected")
	}

	d, err = toMediaMessage(d, chMsgs)
	if err != nil {
		t.Fatalf("converting openai to media message: %s", err)
	}

	msgs := d["messages"].([]D)

	// New normalized shape: ONE output message per input message, preserving
	// role and ordered parts. The input had 2 messages: a 4-part user
	// message (text, image, audio, text) and a text-only follow-up.
	if len(msgs) != 2 {
		t.Fatalf("should have 2 documents in the media message, got %d", len(msgs))
	}

	if role, _ := msgs[0]["role"].(string); role != "user" {
		t.Fatalf("msgs[0] role: got %q, want %q", role, "user")
	}
	if role, _ := msgs[1]["role"].(string); role != "user" {
		t.Fatalf("msgs[1] role: got %q, want %q", role, "user")
	}

	parts, ok := msgs[0]["content"].([]any)
	if !ok {
		t.Fatalf("msgs[0] content should be []any of normalized parts, got %T", msgs[0]["content"])
	}

	if len(parts) != 4 {
		t.Fatalf("msgs[0] should have 4 parts (text, image, audio, text), got %d", len(parts))
	}

	if s, ok := parts[0].(string); !ok || s != "what do you see in the picture?" {
		t.Fatalf("parts[0] should be the question text, got %T %v", parts[0], parts[0])
	}
	if b, ok := parts[1].([]byte); !ok || base64.StdEncoding.EncodeToString(b) != openEncoded {
		t.Fatalf("parts[1] should be the decoded image bytes")
	}
	if b, ok := parts[2].([]byte); !ok || base64.StdEncoding.EncodeToString(b) != openEncoded {
		t.Fatalf("parts[2] should be the decoded audio bytes")
	}
	if s, ok := parts[3].(string); !ok || s != "what do you see in the picture?" {
		t.Fatalf("parts[3] should be the trailing text part, got %T %v", parts[3], parts[3])
	}

	if s, _ := msgs[1]["content"].(string); s != "follow up question" {
		t.Fatalf("msgs[1] content: got %v, want %q", msgs[1]["content"], "follow up question")
	}
}

func Test_PlainBase64MediaDetection(t *testing.T) {
	jpegHeader := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}
	jpegData := append(jpegHeader, make([]byte, 100)...)
	encoded := base64.StdEncoding.EncodeToString(jpegData)

	d := D{
		"messages": DocumentArray(
			D{
				"role":    "user",
				"content": "What is in this image?",
			},
			D{
				"role":    "user",
				"content": encoded,
			},
		),
	}

	mediaType, isOpenAIFormat, _, err := detectMediaContent(d)
	if err != nil {
		t.Fatalf("detect-media: %s", err)
	}

	if mediaType != MediaTypeVision {
		t.Fatalf("expected MediaTypeVision, got %v", mediaType)
	}

	if isOpenAIFormat {
		t.Fatal("expected isOpenAIFormat to be false for plain base64")
	}

	d = convertPlainBase64ToBytes(d)
	msgs := d["messages"].([]D)

	converted := false
	for _, msg := range msgs {
		if data, ok := msg["content"].([]byte); ok {
			converted = true
			if len(data) != len(jpegData) {
				t.Fatalf("expected %d bytes, got %d", len(jpegData), len(data))
			}
		}
	}

	if !converted {
		t.Fatal("expected base64 content to be converted to []byte")
	}
}

func Test_PlainBase64WithDataURI(t *testing.T) {
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	pngData := append(pngHeader, make([]byte, 100)...)
	encoded := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngData)

	d := D{
		"messages": DocumentArray(
			D{
				"role":    "user",
				"content": encoded,
			},
		),
	}

	mediaType, isOpenAIFormat, _, err := detectMediaContent(d)
	if err != nil {
		t.Fatalf("detect-media: %s", err)
	}

	if mediaType != MediaTypeVision {
		t.Fatalf("expected MediaTypeVision, got %v", mediaType)
	}

	if isOpenAIFormat {
		t.Fatal("expected isOpenAIFormat to be false for plain base64 with data URI")
	}
}

func Test_NoMediaDetection(t *testing.T) {
	d := D{
		"messages": DocumentArray(
			D{
				"role":    "user",
				"content": "Hello, how are you?",
			},
			D{
				"role":    "assistant",
				"content": "I'm doing well, thanks!",
			},
		),
	}

	mediaType, isOpenAIFormat, _, err := detectMediaContent(d)
	if err != nil {
		t.Fatalf("detect-media: %s", err)
	}

	if mediaType != MediaTypeNone {
		t.Fatalf("expected MediaTypeNone, got %v", mediaType)
	}

	if isOpenAIFormat {
		t.Fatal("expected isOpenAIFormat to be false for plain text")
	}
}

func Test_PlainBase64AudioDetection(t *testing.T) {
	wavHeader := []byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'A', 'V', 'E'}
	wavData := append(wavHeader, make([]byte, 100)...)
	encoded := base64.StdEncoding.EncodeToString(wavData)

	d := D{
		"messages": DocumentArray(
			D{
				"role":    "user",
				"content": "What do you hear?",
			},
			D{
				"role":    "user",
				"content": encoded,
			},
		),
	}

	mediaType, isOpenAIFormat, _, err := detectMediaContent(d)
	if err != nil {
		t.Fatalf("detect-media: %s", err)
	}

	if mediaType != MediaTypeAudio {
		t.Fatalf("expected MediaTypeAudio, got %v", mediaType)
	}

	if isOpenAIFormat {
		t.Fatal("expected isOpenAIFormat to be false for plain base64")
	}

	d = convertPlainBase64ToBytes(d)
	msgs := d["messages"].([]D)

	converted := false
	for _, msg := range msgs {
		if _, ok := msg["content"].([]byte); ok {
			converted = true
		}
	}

	if !converted {
		t.Fatal("expected WAV base64 content to be converted to []byte")
	}
}

func Test_LongTextNotMedia(t *testing.T) {
	longText := make([]byte, 200)
	for i := range longText {
		longText[i] = 'a'
	}

	d := D{
		"messages": DocumentArray(
			D{
				"role":    "user",
				"content": string(longText),
			},
		),
	}

	mediaType, _, _, err := detectMediaContent(d)
	if err != nil {
		t.Fatalf("detect-media: %s", err)
	}

	if mediaType != MediaTypeNone {
		t.Fatalf("expected MediaTypeNone for long plain text, got %v", mediaType)
	}
}

// Test_OpenAIToMediaMessage_PreservesRole verifies that toMediaMessage keeps
// non-user roles (system, assistant) intact rather than collapsing every
// output message to "user" as the legacy implementation did.
func Test_OpenAIToMediaMessage_PreservesRole(t *testing.T) {
	d := D{
		"messages": DocumentArray(
			D{"role": "system", "content": "be helpful"},
			D{"role": "assistant", "content": "ok"},
			D{"role": "user", "content": "hi"},
		),
	}

	// Add one image-bearing message to trigger the OpenAI media path.
	jpegHeader := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}
	data := append(jpegHeader, make([]byte, 64)...)
	encoded := base64.StdEncoding.EncodeToString(data)

	msgs := d["messages"].([]D)
	msgs = append(msgs, openAIMediaMessage("look", encoded))
	d["messages"] = msgs

	_, isOpenAI, chMsgs, err := detectMediaContent(d)
	if err != nil {
		t.Fatalf("detect-media: %s", err)
	}
	if !isOpenAI {
		t.Fatal("expected OpenAI format")
	}

	d, err = toMediaMessage(d, chMsgs)
	if err != nil {
		t.Fatalf("to-media-message: %s", err)
	}

	out := d["messages"].([]D)
	wantRoles := []string{"system", "assistant", "user", "user"}
	if len(out) != len(wantRoles) {
		t.Fatalf("expected %d messages, got %d", len(wantRoles), len(out))
	}
	for i, want := range wantRoles {
		if got, _ := out[i]["role"].(string); got != want {
			t.Fatalf("msg[%d] role: got %q, want %q", i, got, want)
		}
	}
}

// Test_OpenAIToMediaMessage_MultipleImagesOneMessage verifies that a single
// message containing multiple images is preserved as one message with all
// images in original order — the legacy implementation would silently drop
// the extras.
func Test_OpenAIToMediaMessage_MultipleImagesOneMessage(t *testing.T) {
	jpegHeader := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}
	pngHeader := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	jpegData := append(jpegHeader, make([]byte, 32)...)
	pngData := append(pngHeader, make([]byte, 32)...)

	d := D{
		"messages": DocumentArray(
			D{
				"role": "user",
				"content": []D{
					{"type": "text", "text": "compare these two:"},
					{"type": "image_url", "image_url": D{"url": base64.StdEncoding.EncodeToString(jpegData)}},
					{"type": "image_url", "image_url": D{"url": base64.StdEncoding.EncodeToString(pngData)}},
				},
			},
		),
	}

	_, _, chMsgs, err := detectMediaContent(d)
	if err != nil {
		t.Fatalf("detect-media: %s", err)
	}

	d, err = toMediaMessage(d, chMsgs)
	if err != nil {
		t.Fatalf("to-media-message: %s", err)
	}

	out := d["messages"].([]D)
	if len(out) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out))
	}

	parts, ok := out[0]["content"].([]any)
	if !ok {
		t.Fatalf("content should be []any, got %T", out[0]["content"])
	}
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts (text + 2 images), got %d", len(parts))
	}

	images := 0
	for _, p := range parts {
		if _, ok := p.([]byte); ok {
			images++
		}
	}
	if images != 2 {
		t.Fatalf("expected 2 image parts preserved, got %d", images)
	}
}

// Test_DecodeMediaData_RejectsBadInput verifies that decodeMediaData fails
// fast for empty payloads, invalid base64, and bytes that don't match any
// supported magic-byte signature — instead of letting the failure surface
// later as the opaque "tokenization failed with code 1" from mtmd.
func Test_DecodeMediaData_RejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"empty":                 "",
		"invalid base64":        "not!!!base64@@",
		"unsupported magic":     base64.StdEncoding.EncodeToString([]byte("totally not an image at all just plain text")),
		"empty after data uri":  "data:image/jpeg;base64,",
		"http url not allowed":  "http://example.com/img.jpg",
		"https url not allowed": "https://example.com/img.jpg",
	}

	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeMediaData(input); err == nil {
				t.Fatalf("expected error for %q input, got nil", name)
			}
		})
	}
}

func openAIMediaMessage(text string, media string) D {
	return D{
		"role": "user",
		"content": []D{
			{
				"type": "text",
				"text": text,
			},
			{
				"type": "image_url",
				"image_url": D{
					"url": media,
				},
			},
			{
				"type": "input_audio",
				"input_audio": D{
					"data": media,
				},
			},
			{
				"type": "text",
				"text": text,
			},
		},
	}
}
