// Package audioapp provides the audio (speech-to-text) api endpoints.
package audioapp

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/bucky/model"
	"github.com/ardanlabs/kronk/sdk/pool"
)

// maxUploadBytes matches OpenAI's documented 25 MB file cap for the audio
// transcriptions endpoint. The request limit allows a small amount of space
// for multipart headers and form fields.
const (
	maxUploadBytes       = 25 << 20
	maxMultipartOverhead = 1 << 20
)

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
	r.Body = http.MaxBytesReader(nil, r.Body, maxUploadBytes+maxMultipartOverhead)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		return errs.New(errs.InvalidArgument, fmt.Errorf("parse multipart form: %w", err))
	}
	defer r.MultipartForm.RemoveAll()

	modelID := r.FormValue("model")
	if modelID == "" {
		return errs.Errorf(errs.InvalidArgument, "missing model field")
	}

	file, hdr, err := r.FormFile("file")
	if err != nil {
		return errs.New(errs.InvalidArgument, fmt.Errorf("file form field: %w", err))
	}
	defer file.Close()
	if hdr.Size > maxUploadBytes {
		return errs.Errorf(errs.InvalidArgument, "file exceeds 25 MB limit")
	}

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
	if wantWordTimes {
		opts = append(opts, model.WithWordTimestamps(true))
	}
	whisperOpts, err := parseWhisperOptions(r.Form)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	opts = append(opts, whisperOpts...)

	a.log.Info(ctx, "transcribe", "model", modelID, "filename", hdr.Filename, "size", hdr.Size, "language", language, "response-format", respFmt)

	b, err := a.pool.Bucky.AquireModel(ctx, modelID)
	if err != nil {
		return errs.FromSDK(err)
	}

	if !b.ModelInfo().IsMultilingual && language != "" && language != "en" {
		return errs.Errorf(errs.InvalidArgument, "model[%s] is english-only but language[%s] was requested", modelID, language)
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	tr, err := b.TranscribeFile(ctx, file, opts...)
	if err != nil {
		return errs.FromSDK(fmt.Errorf("transcribe: %w", err))
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

func parseWhisperOptions(form url.Values) ([]model.TranscribeOption, error) {
	opts := make([]model.TranscribeOption, 0, 9)

	floatFields := []struct {
		name   string
		option func(float32) model.TranscribeOption
	}{
		{name: "temperature", option: model.WithTemperature},
		{name: "temperature_inc", option: model.WithTemperatureInc},
		{name: "entropy_threshold", option: model.WithEntropyThreshold},
		{name: "logprob_threshold", option: model.WithLogProbThreshold},
		{name: "no_speech_threshold", option: model.WithNoSpeechThreshold},
		{name: "beam_search_patience", option: model.WithBeamSearchPatience},
		{name: "length_penalty", option: model.WithLengthPenalty},
	}
	for _, field := range floatFields {
		value, exists, err := parseOptionalFloat32(form, field.name)
		if err != nil {
			return nil, err
		}
		if exists {
			opts = append(opts, field.option(value))
		}
	}

	intFields := []struct {
		name   string
		option func(int32) model.TranscribeOption
	}{
		{name: "greedy_best_of", option: model.WithGreedyBestOf},
		{name: "beam_size", option: model.WithBeamSize},
	}
	for _, field := range intFields {
		value, exists, err := parseOptionalInt32(form, field.name)
		if err != nil {
			return nil, err
		}
		if exists {
			opts = append(opts, field.option(value))
		}
	}

	return opts, nil
}

func parseOptionalFloat32(form url.Values, field string) (float32, bool, error) {
	values, exists := form[field]
	if !exists {
		return 0, false, nil
	}
	if len(values) == 0 || values[0] == "" {
		return 0, false, fmt.Errorf("field[%s] must be a number", field)
	}

	value, err := strconv.ParseFloat(values[0], 32)
	if err != nil {
		return 0, false, fmt.Errorf("field[%s] must be a number: %w", field, err)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false, fmt.Errorf("field[%s] must be a finite number", field)
	}

	return float32(value), true, nil
}

func parseOptionalInt32(form url.Values, field string) (int32, bool, error) {
	values, exists := form[field]
	if !exists {
		return 0, false, nil
	}
	if len(values) == 0 || values[0] == "" {
		return 0, false, fmt.Errorf("field[%s] must be an integer", field)
	}

	value, err := strconv.ParseInt(values[0], 10, 32)
	if err != nil {
		return 0, false, fmt.Errorf("field[%s] must be an integer: %w", field, err)
	}

	return int32(value), true, nil
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

	if wantWordTimes {
		words := make([]map[string]any, 0, len(tr.Words))
		for _, w := range tr.Words {
			words = append(words, map[string]any{
				"word":  w.Text,
				"start": float64(w.StartMs) / 1000.0,
				"end":   float64(w.EndMs) / 1000.0,
			})
		}
		out["words"] = words
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
