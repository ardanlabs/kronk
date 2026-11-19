package audioapp

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Allow the full 64 KiB SDK input limit even when every byte is JSON-escaped,
// plus space for model/voice names and ignored compatibility fields.
const maxSpeechRequestBytes = 1 << 20

type speechRequest struct {
	Model          string `json:"model"`
	Input          string `json:"input"`
	Voice          string `json:"voice"`
	ResponseFormat string `json:"response_format"`
	StreamFormat   string `json:"stream_format"`
}

func (sr *speechRequest) Decode(data []byte) error {
	if !utf8.Valid(data) {
		return errors.New("speech request must be valid UTF-8")
	}
	return json.Unmarshal(data, sr)
}

func (sr *speechRequest) Validate() error {
	switch {
	case sr.Model == "":
		return errors.New("missing model field")
	case strings.TrimSpace(sr.Input) == "":
		return errors.New("missing input field")
	case sr.Voice == "":
		return errors.New("missing voice field")
	}
	if sr.ResponseFormat == "" {
		sr.ResponseFormat = "wav"
	}
	if speechContentType(sr.ResponseFormat) == "" {
		return fmt.Errorf("unsupported response_format %q; supported formats: wav, pcm, mp3, opus, aac, flac", sr.ResponseFormat)
	}
	if sr.StreamFormat != "" && sr.StreamFormat != "audio" {
		return fmt.Errorf("unsupported stream_format %q; supported format: audio (buffered)", sr.StreamFormat)
	}
	return nil
}

func speechContentType(format string) string {
	switch format {
	case "wav":
		return "audio/wav"
	case "pcm":
		return "audio/pcm"
	case "mp3":
		return "audio/mpeg"
	case "opus":
		return "audio/ogg"
	case "aac":
		return "audio/aac"
	case "flac":
		return "audio/flac"
	default:
		return ""
	}
}

// speechPCM extracts the original samples without resampling or assuming a
// fixed WAV header size. The SDK owns the PCM16 little-endian mono format.
func speechPCM(wav []byte) ([]byte, error) {
	if len(wav) < 12 || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" ||
		uint64(binary.LittleEndian.Uint32(wav[4:8]))+8 != uint64(len(wav)) {
		return nil, errors.New("speech response is not a complete WAV")
	}
	for offset := 12; offset < len(wav); {
		if len(wav)-offset < 8 {
			return nil, errors.New("speech response has a truncated WAV chunk")
		}
		size := int(binary.LittleEndian.Uint32(wav[offset+4 : offset+8]))
		start := offset + 8
		if size > len(wav)-start || size+size%2 > len(wav)-start {
			return nil, errors.New("speech response has a truncated WAV chunk")
		}
		if string(wav[offset:offset+4]) == "data" {
			if size == 0 || size%2 != 0 {
				return nil, errors.New("speech response has invalid PCM16 data")
			}
			return wav[start : start+size], nil
		}
		offset = start + size + size%2
	}
	return nil, errors.New("speech response has no WAV data chunk")
}
