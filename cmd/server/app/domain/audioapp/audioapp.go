// Package audioapp provides the audio (speech-to-text) api endpoints.
package audioapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/bucky/model"
	"github.com/ardanlabs/kronk/sdk/pool"
)

// maxUploadBytes matches OpenAI's documented 25 MB cap for the audio
// transcriptions endpoint.
const maxUploadBytes = 25 << 20

type app struct {
	log  *logger.Logger
	pool *pool.Pool
}

func newApp(cfg Config) *app {
	return &app{
		log:  cfg.Log,
		pool: cfg.Pool,
	}
}

func (a *app) transcriptions(ctx context.Context, r *http.Request) web.Encoder {
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		return errs.New(errs.InvalidArgument, fmt.Errorf("parse multipart form: %w", err))
	}

	modelID := r.FormValue("model")
	if modelID == "" {
		return errs.Errorf(errs.InvalidArgument, "missing model field")
	}

	file, hdr, err := r.FormFile("file")
	if err != nil {
		return errs.New(errs.InvalidArgument, fmt.Errorf("file form field: %w", err))
	}
	defer file.Close()

	language := r.FormValue("language")
	prompt := r.FormValue("prompt")
	translate := parseBool(r.FormValue("translate"))

	respFmt := r.FormValue("response_format")
	if respFmt == "" {
		respFmt = "json"
	}
	switch respFmt {
	case "json", "verbose_json", "text", "srt", "vtt":
	default:
		return errs.Errorf(errs.InvalidArgument, "unsupported response_format[%s]", respFmt)
	}

	wantWordTimes := false
	for _, g := range r.Form["timestamp_granularities[]"] {
		if g == "word" {
			wantWordTimes = true
		}
	}

	a.log.Info(ctx, "transcribe", "model", modelID, "filename", hdr.Filename, "size", hdr.Size, "language", language, "response-format", respFmt)

	b, err := a.pool.Bucky.AquireModel(ctx, modelID)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	if !b.ModelInfo().IsMultilingual && language != "" && language != "en" {
		return errs.Errorf(errs.InvalidArgument, "model[%s] is english-only but language[%s] was requested", modelID, language)
	}

	opts := []model.TranscribeOption{}
	if language != "" {
		opts = append(opts, model.WithLanguage(language))
	}
	if prompt != "" {
		opts = append(opts, model.WithInitialPrompt(prompt))
	}
	if translate {
		opts = append(opts, model.WithTranslate(true))
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	tr, err := b.TranscribeFile(ctx, file, opts...)
	if err != nil {
		return errs.New(errs.Internal, fmt.Errorf("transcribe: %w", err))
	}

	duration := tr.Duration

	switch respFmt {
	case "text":
		return rawResponse{data: []byte(tr.Text), contentType: "text/plain; charset=utf-8"}
	case "srt":
		return rawResponse{data: []byte(formatSRT(tr)), contentType: "application/x-subrip; charset=utf-8"}
	case "vtt":
		return rawResponse{data: []byte(formatVTT(tr)), contentType: "text/vtt; charset=utf-8"}
	case "verbose_json":
		return jsonResponse(verboseJSON(tr, duration, wantWordTimes))
	default:
		return jsonResponse(map[string]any{"text": tr.Text})
	}
}

// =============================================================================

func parseBool(s string) bool {
	if s == "" {
		return false
	}
	b, err := strconv.ParseBool(s)
	if err != nil {
		return false
	}
	return b
}

func verboseJSON(tr model.Transcription, duration float64, wantWordTimes bool) map[string]any {
	segments := make([]map[string]any, 0, len(tr.Segments))
	for i, s := range tr.Segments {
		segments = append(segments, map[string]any{
			"id":                i,
			"seek":              0,
			"start":             float64(s.StartMs) / 1000.0,
			"end":               float64(s.EndMs) / 1000.0,
			"text":              s.Text,
			"tokens":            []int{},
			"temperature":       0.0,
			"avg_logprob":       0.0,
			"compression_ratio": 0.0,
			"no_speech_prob":    s.NoSpeechProb,
		})
	}

	out := map[string]any{
		"task":     "transcribe",
		"language": tr.Language,
		"duration": duration,
		"text":     tr.Text,
		"segments": segments,
	}

	// Word-level timestamps require per-word data from whisper.cpp,
	// which the bucky SDK does not yet surface. Emit an empty list
	// when the client asks for word granularity so the response shape
	// stays compatible.
	if wantWordTimes {
		out["words"] = []map[string]any{}
	}

	return out
}

func formatSRT(tr model.Transcription) string {
	var out []byte
	for i, s := range tr.Segments {
		out = append(out, []byte(strconv.Itoa(i+1))...)
		out = append(out, '\n')
		out = append(out, []byte(srtTimestamp(s.StartMs))...)
		out = append(out, ' ', '-', '-', '>', ' ')
		out = append(out, []byte(srtTimestamp(s.EndMs))...)
		out = append(out, '\n')
		out = append(out, []byte(s.Text)...)
		out = append(out, '\n', '\n')
	}
	return string(out)
}

func formatVTT(tr model.Transcription) string {
	out := []byte("WEBVTT\n\n")
	for _, s := range tr.Segments {
		out = append(out, []byte(vttTimestamp(s.StartMs))...)
		out = append(out, ' ', '-', '-', '>', ' ')
		out = append(out, []byte(vttTimestamp(s.EndMs))...)
		out = append(out, '\n')
		out = append(out, []byte(s.Text)...)
		out = append(out, '\n', '\n')
	}
	return string(out)
}

func srtTimestamp(ms int64) string {
	h := ms / 3600000
	ms -= h * 3600000
	m := ms / 60000
	ms -= m * 60000
	s := ms / 1000
	ms -= s * 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

func vttTimestamp(ms int64) string {
	h := ms / 3600000
	ms -= h * 3600000
	m := ms / 60000
	ms -= m * 60000
	s := ms / 1000
	ms -= s * 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}

// =============================================================================

// rawResponse implements web.Encoder for non-JSON response formats.
type rawResponse struct {
	data        []byte
	contentType string
}

// Encode implements the web.Encoder interface.
func (r rawResponse) Encode() ([]byte, string, error) {
	return r.data, r.contentType, nil
}

// jsonResponse implements web.Encoder for JSON response formats.
type jsonResponse map[string]any

// Encode implements the web.Encoder interface.
func (j jsonResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(map[string]any(j))
	if err != nil {
		return nil, "", err
	}
	return data, "application/json", nil
}
