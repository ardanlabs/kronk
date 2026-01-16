package model

import (
	"encoding/base64"
	"fmt"
	"strings"
)

type MediaType int

const (
	MediaTypeNone MediaType = iota
	MediaTypeVision
	MediaTypeAudio
)

// detectMediaContent detects if the request contains media data in either:
// - Form1: Plain base64 string as content (hasMedia=true, isOpenAIFormat=false)
// - Form2: OpenAI structured format with image_url, video_url, or input_audio (hasMedia=true, isOpenAIFormat=true)
func detectMediaContent(d D) (mediaType MediaType, isOpenAIFormat bool, msgs chatMessages, err error) {
	msgs, err = toChatMessages(d)
	if err != nil {
		return MediaTypeNone, false, chatMessages{}, fmt.Errorf("detect-media-content: chat message conversion: %w", err)
	}

	for _, msg := range msgs.Messages {
		switch content := msg.Content.(type) {
		case []chatMessageContent:
			for _, cm := range content {
				switch cm.Type {
				case "image_url", "video_url":
					return MediaTypeVision, true, msgs, nil
				case "input_audio":
					return MediaTypeAudio, true, msgs, nil
				}
			}

		case string:
			if mt := detectMediaType(content); mt != MediaTypeNone {
				mediaType = mt
			}
		}
	}

	return mediaType, false, msgs, nil
}

// convertPlainBase64ToBytes converts Form1 plain base64 string content to raw bytes.
// This modifies the document in place.
func convertPlainBase64ToBytes(d D) D {
	msgs, ok := d["messages"].([]D)
	if !ok {
		return d
	}

	d = d.Clone()
	clonedMsgs := make([]D, len(msgs))
	for i, msg := range msgs {
		clonedMsgs[i] = msg.Clone()
	}
	d["messages"] = clonedMsgs

	for _, msg := range clonedMsgs {
		content, exists := msg["content"]
		if !exists {
			continue
		}

		if s, ok := content.(string); ok {
			if decoded := tryDecodeMedia(s); decoded != nil {
				msg["content"] = decoded
			}
		}
	}

	return d
}

func tryDecodeMedia(s string) []byte {
	if len(s) < 100 {
		return nil
	}

	data := s
	if idx := strings.Index(s, ";base64,"); idx != -1 && strings.HasPrefix(s, "data:") {
		data = s[idx+8:]
	}

	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil
	}

	if len(decoded) < 4 {
		return nil
	}

	if decoded[0] == 0xFF && decoded[1] == 0xD8 && decoded[2] == 0xFF {
		return decoded
	}

	if decoded[0] == 0x89 && decoded[1] == 'P' && decoded[2] == 'N' && decoded[3] == 'G' {
		return decoded
	}

	if string(decoded[:3]) == "GIF" {
		return decoded
	}

	if len(decoded) >= 12 && string(decoded[:4]) == "RIFF" && string(decoded[8:12]) == "WEBP" {
		return decoded
	}

	if len(decoded) >= 12 && string(decoded[:4]) == "RIFF" && string(decoded[8:12]) == "WAVE" {
		return decoded
	}

	if decoded[0] == 0xFF && (decoded[1] == 0xFB || decoded[1] == 0xFA || decoded[1] == 0xF3 || decoded[1] == 0xF2) {
		return decoded
	}

	if string(decoded[:3]) == "ID3" {
		return decoded
	}

	if len(decoded) >= 4 && string(decoded[:4]) == "OggS" {
		return decoded
	}

	if len(decoded) >= 4 && string(decoded[:4]) == "fLaC" {
		return decoded
	}

	return nil
}

func detectMediaType(s string) MediaType {
	if len(s) < 100 {
		return MediaTypeNone
	}

	data := s
	if idx := strings.Index(s, ";base64,"); idx != -1 && strings.HasPrefix(s, "data:") {
		data = s[idx+8:]
	}

	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return MediaTypeNone
	}

	if len(decoded) < 4 {
		return MediaTypeNone
	}

	// Vision formats: JPEG, PNG, GIF, WEBP
	if decoded[0] == 0xFF && decoded[1] == 0xD8 && decoded[2] == 0xFF {
		return MediaTypeVision
	}

	if decoded[0] == 0x89 && decoded[1] == 'P' && decoded[2] == 'N' && decoded[3] == 'G' {
		return MediaTypeVision
	}

	if string(decoded[:3]) == "GIF" {
		return MediaTypeVision
	}

	if len(decoded) >= 12 && string(decoded[:4]) == "RIFF" && string(decoded[8:12]) == "WEBP" {
		return MediaTypeVision
	}

	// Audio formats: WAV, MP3, ID3, OGG, FLAC
	if len(decoded) >= 12 && string(decoded[:4]) == "RIFF" && string(decoded[8:12]) == "WAVE" {
		return MediaTypeAudio
	}

	if decoded[0] == 0xFF && (decoded[1] == 0xFB || decoded[1] == 0xFA || decoded[1] == 0xF3 || decoded[1] == 0xF2) {
		return MediaTypeAudio
	}

	if string(decoded[:3]) == "ID3" {
		return MediaTypeAudio
	}

	if len(decoded) >= 4 && string(decoded[:4]) == "OggS" {
		return MediaTypeAudio
	}

	if len(decoded) >= 4 && string(decoded[:4]) == "fLaC" {
		return MediaTypeAudio
	}

	return MediaTypeNone
}

// convertToRawMediaMessage is needed because we want to use a raw media message
// format for processing media since we need the raw bytes.
func convertToRawMediaMessage(d D, msgs chatMessages) (D, error) {
	d, err := toMediaMessage(d, msgs)
	if err != nil {
		return nil, fmt.Errorf("convert-to-raw-media-message: media message conversion: %w", err)
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
		case nil:
			continue

		case string:
			mediaMessages = append(mediaMessages, mediaMessage{
				text: content,
			})
			continue

		case []chatMessageContent:
			for _, cm := range content {
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
		return nil, fmt.Errorf("decode-media-message: URLs are not supported, provide base64 encoded data")
	}

	if idx := strings.Index(data, ";base64,"); idx != -1 && strings.HasPrefix(data, "data:") {
		data = data[idx+8:]
	}

	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("decode-media-message: unable to decode base64 data: %w", err)
	}

	return decoded, nil
}
