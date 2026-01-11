package model

import (
	"encoding/base64"
	"testing"
)

func Test_OpenAIToMediaMessage(t *testing.T) {
	data := []byte("this is not really an image but it will do")
	openEncoded := base64.StdEncoding.EncodeToString(data)

	d := D{
		"messages": DocumentArray(
			openAIMediaMessage("what do you see in the picture?", openEncoded),
			D{
				"content": "follow up question",
			},
		),
	}

	hasMedia, isOpenAIFormat, chMsgs, err := detectMediaContent(d)
	if err != nil {
		t.Fatalf("detect-media: unable to check document: %s", err)
	}

	if !hasMedia {
		t.Fatal("expected media to be detected")
	}

	if !isOpenAIFormat {
		t.Fatal("expected OpenAI format to be detected")
	}

	d, err = convertToRawMediaMessage(d, chMsgs)
	if err != nil {
		t.Fatalf("converting openai to media message: %s", err)
	}

	msgs := d["messages"].([]D)

	if len(msgs) != 5 {
		t.Fatalf("should have 5 documents in the media message, got %d", len(msgs))
	}

	for _, d := range msgs {
		data, ok := d["content"].([]byte)
		if ok {
			mediaEncoded := base64.StdEncoding.EncodeToString(data)
			if openEncoded != mediaEncoded {
				t.Fatalf("media mismatch from input to output\ngot:[%s]\nexp:[%s]", openEncoded, mediaEncoded)
			}
		}
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

	hasMedia, isOpenAIFormat, _, err := detectMediaContent(d)
	if err != nil {
		t.Fatalf("detect-media: %s", err)
	}

	if !hasMedia {
		t.Fatal("expected media to be detected for plain base64 JPEG")
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

	hasMedia, isOpenAIFormat, _, err := detectMediaContent(d)
	if err != nil {
		t.Fatalf("detect-media: %s", err)
	}

	if !hasMedia {
		t.Fatal("expected media to be detected for data URI PNG")
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

	hasMedia, isOpenAIFormat, _, err := detectMediaContent(d)
	if err != nil {
		t.Fatalf("detect-media: %s", err)
	}

	if hasMedia {
		t.Fatal("expected no media to be detected for plain text")
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

	hasMedia, isOpenAIFormat, _, err := detectMediaContent(d)
	if err != nil {
		t.Fatalf("detect-media: %s", err)
	}

	if !hasMedia {
		t.Fatal("expected media to be detected for plain base64 WAV")
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

	hasMedia, _, _, err := detectMediaContent(d)
	if err != nil {
		t.Fatalf("detect-media: %s", err)
	}

	if hasMedia {
		t.Fatal("expected no media for long plain text")
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
