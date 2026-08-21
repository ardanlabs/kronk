package audioapp

import (
	"net/url"
	"testing"

	"github.com/ardanlabs/kronk/sdk/bucky/model"
)

func TestParseWhisperOptions(t *testing.T) {
	form := url.Values{
		"temperature":          {"0"},
		"temperature_inc":      {"0.3"},
		"entropy_threshold":    {"2.1"},
		"logprob_threshold":    {"-0.8"},
		"no_speech_threshold":  {"0.5"},
		"greedy_best_of":       {"3"},
		"beam_size":            {"5"},
		"beam_search_patience": {"1.2"},
		"length_penalty":       {"-0.5"},
	}

	options, err := parseWhisperOptions(form)
	if err != nil {
		t.Fatalf("parseWhisperOptions: %v", err)
	}

	var cfg model.TranscribeConfig
	for _, option := range options {
		option(&cfg)
	}

	if cfg.Temperature == nil || *cfg.Temperature != 0 {
		t.Errorf("Temperature: got %v, want pointer to 0", cfg.Temperature)
	}
	if cfg.TemperatureInc == nil || *cfg.TemperatureInc != 0.3 {
		t.Errorf("TemperatureInc: got %v, want pointer to 0.3", cfg.TemperatureInc)
	}
	if cfg.EntropyThreshold == nil || *cfg.EntropyThreshold != 2.1 {
		t.Errorf("EntropyThreshold: got %v, want pointer to 2.1", cfg.EntropyThreshold)
	}
	if cfg.LogProbThreshold == nil || *cfg.LogProbThreshold != -0.8 {
		t.Errorf("LogProbThreshold: got %v, want pointer to -0.8", cfg.LogProbThreshold)
	}
	if cfg.NoSpeechThreshold != 0.5 {
		t.Errorf("NoSpeechThreshold: got %v, want 0.5", cfg.NoSpeechThreshold)
	}
	if cfg.GreedyBestOf == nil || *cfg.GreedyBestOf != 3 {
		t.Errorf("GreedyBestOf: got %v, want pointer to 3", cfg.GreedyBestOf)
	}
	if cfg.BeamSize != 5 {
		t.Errorf("BeamSize: got %d, want 5", cfg.BeamSize)
	}
	if cfg.BeamSearchPatience == nil || *cfg.BeamSearchPatience != 1.2 {
		t.Errorf("BeamSearchPatience: got %v, want pointer to 1.2", cfg.BeamSearchPatience)
	}
	if cfg.LengthPenalty == nil || *cfg.LengthPenalty != -0.5 {
		t.Errorf("LengthPenalty: got %v, want pointer to -0.5", cfg.LengthPenalty)
	}
}

func TestParseWhisperOptionsOmitted(t *testing.T) {
	options, err := parseWhisperOptions(nil)
	if err != nil {
		t.Fatalf("parseWhisperOptions: %v", err)
	}
	if len(options) != 0 {
		t.Errorf("options: got %d, want 0", len(options))
	}
}

func TestParseWhisperOptionsRejectsInvalidNumbers(t *testing.T) {
	tests := []struct {
		name  string
		field string
		value string
	}{
		{name: "empty float", field: "temperature"},
		{name: "invalid float", field: "length_penalty", value: "invalid"},
		{name: "not a number", field: "temperature", value: "NaN"},
		{name: "infinite", field: "temperature_inc", value: "Inf"},
		{name: "empty integer", field: "beam_size"},
		{name: "fractional integer", field: "greedy_best_of", value: "2.5"},
		{name: "integer overflow", field: "beam_size", value: "2147483648"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseWhisperOptions(url.Values{tt.field: {tt.value}})
			if err == nil {
				t.Fatalf("parseWhisperOptions(%s=%q): got nil error", tt.field, tt.value)
			}
		})
	}
}

func TestVerboseJSONWordTimestamps(t *testing.T) {
	tr := model.Transcription{
		Text:     "hello world",
		Language: "en",
		Words: []model.Word{
			{Text: "hello", StartMs: 120, EndMs: 450},
			{Text: "world", StartMs: 500, EndMs: 900},
		},
	}

	got := verboseJSON(tr, 1, true)
	words, ok := got["words"].([]map[string]any)
	if !ok {
		t.Fatalf("words: got %T, want []map[string]any", got["words"])
	}
	if len(words) != 2 {
		t.Fatalf("words: got %d, want 2", len(words))
	}
	if words[0]["word"] != "hello" || words[0]["start"] != 0.12 || words[0]["end"] != 0.45 {
		t.Errorf("words[0]: got %+v, want hello from 0.12 to 0.45", words[0])
	}
	if words[1]["word"] != "world" || words[1]["start"] != 0.5 || words[1]["end"] != 0.9 {
		t.Errorf("words[1]: got %+v, want world from 0.5 to 0.9", words[1])
	}
}
