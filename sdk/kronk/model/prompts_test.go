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

	d, err := convertToRawMediaMessage(d)
	if err != nil {
		t.Fatalf("convering openai to media message: %s", err)
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
