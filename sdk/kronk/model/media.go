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

		case []byte:
			// Raw bytes from SDK callers (Form1, RawMediaMessage). Identify
			// directly from the magic bytes — there's no base64 round-trip
			// in the new toChatMessages walker, so we don't have a string
			// to feed detectMediaType.
			if mt := mediaTypeFromMagicBytes(content); mt != MediaTypeNone {
				mediaType = mt
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
// Deep clones messages before mutating since the original slice may be shared.
func convertPlainBase64ToBytes(d D) D {
	msgs, ok := d["messages"].([]D)
	if !ok {
		return d
	}

	// Deep clone messages since we mutate content in place and the original
	// messages slice may be shared across concurrent requests.
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
	if _, after, ok := strings.Cut(s, ";base64,"); ok && strings.HasPrefix(s, "data:") {
		data = after
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

// messageHasMedia checks if a single message D contains media content.
// Handles:
//   - The post-normalization shapes:
//   - content is []byte (single media payload)
//   - content is []any with at least one []byte part (interleaved
//     text/media parts produced by normalizeMediaMessages).
//   - The raw OpenAI structured format ([]any/[]D of typed parts with
//     "image_url", "video_url", "input_audio") for code paths that may run
//     before normalization.
//   - Plain base64 encoded media strings.
func messageHasMedia(msg D) bool {
	content, ok := msg["content"]
	if !ok {
		return false
	}

	switch c := content.(type) {
	case []byte:
		return true

	case []any:
		for _, part := range c {
			if _, ok := part.([]byte); ok {
				return true
			}
			if partHasMediaType(part) {
				return true
			}
		}

	case []D:
		for _, part := range c {
			if partHasMediaType(part) {
				return true
			}
		}

	case string:
		if detectMediaType(c) != MediaTypeNone {
			return true
		}
	}

	return false
}

// partHasMediaType checks if a content part has a media type field.
func partHasMediaType(part any) bool {
	var m map[string]any

	switch v := part.(type) {
	case map[string]any:
		m = v
	case D:
		m = v
	default:
		return false
	}

	switch m["type"] {
	case "image_url", "video_url", "input_audio":
		return true
	}

	return false
}

func detectMediaType(s string) MediaType {
	if len(s) < 100 {
		return MediaTypeNone
	}

	data := s
	if _, after, ok := strings.Cut(s, ";base64,"); ok && strings.HasPrefix(s, "data:") {
		data = after
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

// toMediaMessage normalizes OpenAI structured multipart messages into Kronk's
// canonical internal shape. It produces ONE output message per input message,
// preserving:
//
//   - the original role (user, assistant, system, tool)
//   - the original order of parts within a message
//   - the multiplicity of text and media parts (multiple texts and multiple
//     media in a single message are all retained in original order)
//
// Output content shapes (per message):
//
//   - string                       — message had only text parts (joined)
//   - []byte                       — message had a single media part and no text
//   - []any of string and []byte   — message had interleaved text/media parts
//
// Media payloads are decoded and validated up front (decodeMediaData rejects
// empty payloads, bad base64, and unsupported magic bytes), so any malformed
// media surfaces here as a clear error rather than as a downstream
// "tokenization failed with code 1".
//
// This replaces the previous implementation, which used cross-message state
// to pair text and media into RawMediaMessage entries. That approach silently
// dropped images, paired text from one message with media from another,
// collapsed every output to the user role, and could not represent multiple
// text or media parts in a single message.
func toMediaMessage(d D, msgs chatMessages) (D, error) {
	docs := make([]D, 0, len(msgs.Messages))

	for i, msg := range msgs.Messages {
		role := msg.Role
		if role == "" {
			role = RoleUser
		}

		switch content := msg.Content.(type) {
		case nil:
			docs = append(docs, D{"role": role})

		case string:
			docs = append(docs, D{"role": role, "content": content})

		case []byte:
			docs = append(docs, D{"role": role, "content": content})

		case []chatMessageContent:
			parts, err := normalizePartsFromOpenAI(content, i)
			if err != nil {
				return d, err
			}

			out := D{"role": role}
			switch v := compactNormalizedParts(parts).(type) {
			case nil:
				// Empty content; leave the message with only its role.
			default:
				out["content"] = v
			}
			docs = append(docs, out)
		}
	}

	d["messages"] = docs

	return d, nil
}

// normalizePartsFromOpenAI converts the typed multipart content of a single
// OpenAI-style message into an ordered slice where each element is either a
// string (text) or []byte (decoded media). Empty text parts are dropped.
func normalizePartsFromOpenAI(content []chatMessageContent, msgIdx int) ([]any, error) {
	parts := make([]any, 0, len(content))

	for j, cm := range content {
		switch cm.Type {
		case "text":
			if cm.Text == "" {
				continue
			}
			parts = append(parts, cm.Text)

		case "image_url":
			decoded, err := decodeMediaData(cm.ImageURL.URL)
			if err != nil {
				return nil, fmt.Errorf("normalize-media: message[%d].content[%d] image_url: %w", msgIdx, j, err)
			}
			parts = append(parts, decoded)

		case "video_url":
			decoded, err := decodeMediaData(cm.VideoURL.URL)
			if err != nil {
				return nil, fmt.Errorf("normalize-media: message[%d].content[%d] video_url: %w", msgIdx, j, err)
			}
			parts = append(parts, decoded)

		case "input_audio":
			decoded, err := decodeMediaData(cm.AudioData.Data)
			if err != nil {
				return nil, fmt.Errorf("normalize-media: message[%d].content[%d] input_audio: %w", msgIdx, j, err)
			}
			parts = append(parts, decoded)
		}
	}

	return parts, nil
}

// compactNormalizedParts collapses a parts slice to the simplest shape that
// can represent it without losing information:
//
//   - 0 parts          -> nil       (caller should omit the content key)
//   - all-text parts   -> string    (joined verbatim)
//   - 1 media-only     -> []byte
//   - mixed/multi      -> []any preserved as-is
func compactNormalizedParts(parts []any) any {
	if len(parts) == 0 {
		return nil
	}

	allText := true
	mediaCount := 0
	for _, p := range parts {
		switch p.(type) {
		case string:
			// still text
		case []byte:
			allText = false
			mediaCount++
		default:
			allText = false
		}
	}

	if allText {
		var b strings.Builder
		for _, p := range parts {
			b.WriteString(p.(string))
		}
		return b.String()
	}

	if len(parts) == 1 && mediaCount == 1 {
		return parts[0].([]byte)
	}

	return parts
}

func decodeMediaData(data string) ([]byte, error) {
	if strings.HasPrefix(data, "http://") || strings.HasPrefix(data, "https://") {
		return nil, fmt.Errorf("decode-media-message: URLs are not supported, provide base64 encoded data")
	}

	if idx := strings.Index(data, ";base64,"); idx != -1 && strings.HasPrefix(data, "data:") {
		data = data[idx+8:]
	}

	if data == "" {
		return nil, fmt.Errorf("decode-media-message: empty media payload")
	}

	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("decode-media-message: unable to decode base64 data: %w", err)
	}

	if len(decoded) == 0 {
		return nil, fmt.Errorf("decode-media-message: decoded media payload is empty")
	}

	if mt := mediaTypeFromMagicBytes(decoded); mt == MediaTypeNone {
		return nil, fmt.Errorf("decode-media-message: media payload does not match any supported image or audio format")
	}

	return decoded, nil
}

// mediaTypeFromMagicBytes inspects the first bytes of decoded media to identify
// supported image or audio container formats. Returns MediaTypeNone when the
// payload does not match any known signature.
func mediaTypeFromMagicBytes(decoded []byte) MediaType {
	if len(decoded) < 4 {
		return MediaTypeNone
	}

	// Vision formats: JPEG, PNG, GIF, WEBP.
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

	// Audio formats: WAV, MP3, ID3, OGG, FLAC.
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
