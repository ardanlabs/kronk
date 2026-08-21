package model

import (
	"testing"

	"github.com/ardanlabs/bucky/pkg/whisper"
)

func TestTranscribeSamplingOptions(t *testing.T) {
	var cfg TranscribeConfig
	options := []TranscribeOption{
		WithTemperature(0),
		WithTemperatureInc(0.3),
		WithEntropyThreshold(2.1),
		WithLogProbThreshold(-0.8),
		WithNoSpeechThreshold(0.5),
		WithGreedyBestOf(3),
		WithBeamSize(5),
		WithBeamSearchPatience(1.2),
		WithLengthPenalty(-0.5),
		WithWordTimestamps(true),
	}
	for _, option := range options {
		option(&cfg)
	}

	params := whisper.WhisperFullParams{
		Temperature:    0.7,
		TemperatureInc: 0.2,
		EntropyThold:   2.4,
		LogprobThold:   -1,
		NoSpeechThold:  0.6,
		GreedyBestOf:   2,
		LengthPenalty:  -1,
	}
	applyFullParamOverrides(&params, cfg)

	if params.Temperature != 0 {
		t.Errorf("Temperature: got %v, want 0", params.Temperature)
	}
	if params.TemperatureInc != 0.3 {
		t.Errorf("TemperatureInc: got %v, want 0.3", params.TemperatureInc)
	}
	if params.EntropyThold != 2.1 {
		t.Errorf("EntropyThold: got %v, want 2.1", params.EntropyThold)
	}
	if params.LogprobThold != -0.8 {
		t.Errorf("LogprobThold: got %v, want -0.8", params.LogprobThold)
	}
	if params.NoSpeechThold != 0.5 {
		t.Errorf("NoSpeechThold: got %v, want 0.5", params.NoSpeechThold)
	}
	if params.GreedyBestOf != 3 {
		t.Errorf("GreedyBestOf: got %d, want 3", params.GreedyBestOf)
	}
	if params.BeamSearchBeamSize != 5 {
		t.Errorf("BeamSearchBeamSize: got %d, want 5", params.BeamSearchBeamSize)
	}
	if params.BeamSearchPatience != 1.2 {
		t.Errorf("BeamSearchPatience: got %v, want 1.2", params.BeamSearchPatience)
	}
	if params.LengthPenalty != -0.5 {
		t.Errorf("LengthPenalty: got %v, want -0.5", params.LengthPenalty)
	}
	if params.TokenTimestamps != 1 {
		t.Errorf("TokenTimestamps: got %d, want 1", params.TokenTimestamps)
	}
	if params.SplitOnWord != 1 {
		t.Errorf("SplitOnWord: got %d, want 1", params.SplitOnWord)
	}
	if got := transcribeSamplingStrategy(cfg); got != whisper.SamplingBeamSearch {
		t.Errorf("sampling strategy: got %v, want beam search", got)
	}
}

func TestAppendWordPiece(t *testing.T) {
	var words []Word
	words = appendWordPiece(words, " Amer", 100, 200)
	words = appendWordPiece(words, "icans", 200, 300)
	words = appendWordPiece(words, ",", 300, 320)
	words = appendWordPiece(words, " hello", 400, 500)
	words = appendWordPiece(words, " ", 500, 510)

	want := []Word{
		{Text: "Americans,", StartMs: 100, EndMs: 320},
		{Text: "hello", StartMs: 400, EndMs: 500},
	}
	if len(words) != len(want) {
		t.Fatalf("words: got %d, want %d", len(words), len(want))
	}
	for i := range want {
		if words[i] != want[i] {
			t.Errorf("words[%d]: got %+v, want %+v", i, words[i], want[i])
		}
	}
}

func TestTranscribeSamplingOptionsPreserveDefaults(t *testing.T) {
	want := whisper.WhisperFullParams{
		Temperature:        0.7,
		TemperatureInc:     0.2,
		EntropyThold:       2.4,
		LogprobThold:       -1,
		NoSpeechThold:      0.6,
		GreedyBestOf:       2,
		BeamSearchBeamSize: 4,
		BeamSearchPatience: -1,
		LengthPenalty:      -1,
		PrintProgress:      0,
		PrintRealtime:      0,
		PrintTimestamps:    0,
	}
	got := want

	applyFullParamOverrides(&got, TranscribeConfig{})

	if got != want {
		t.Errorf("params changed without overrides: got %+v, want %+v", got, want)
	}
	if strategy := transcribeSamplingStrategy(TranscribeConfig{}); strategy != whisper.SamplingGreedy {
		t.Errorf("sampling strategy: got %v, want greedy", strategy)
	}
}

func TestStreamSamplingOptions(t *testing.T) {
	var cfg StreamConfig
	options := []StreamOption{
		WithStreamTemperature(0),
		WithStreamTemperatureInc(0.3),
		WithStreamEntropyThreshold(2.1),
		WithStreamLogProbThreshold(-0.8),
		WithStreamNoSpeechThreshold(0.5),
		WithStreamGreedyBestOf(3),
		WithStreamBeamSize(5),
		WithStreamBeamSearchPatience(1.2),
		WithStreamLengthPenalty(-0.5),
	}
	for _, option := range options {
		option(&cfg)
	}

	got := streamTranscribeConfig(cfg, []whisper.Token{1, 2})
	if got.Temperature == nil || *got.Temperature != 0 {
		t.Errorf("Temperature: got %v, want pointer to 0", got.Temperature)
	}
	if got.TemperatureInc == nil || *got.TemperatureInc != 0.3 {
		t.Errorf("TemperatureInc: got %v, want pointer to 0.3", got.TemperatureInc)
	}
	if got.EntropyThreshold == nil || *got.EntropyThreshold != 2.1 {
		t.Errorf("EntropyThreshold: got %v, want pointer to 2.1", got.EntropyThreshold)
	}
	if got.LogProbThreshold == nil || *got.LogProbThreshold != -0.8 {
		t.Errorf("LogProbThreshold: got %v, want pointer to -0.8", got.LogProbThreshold)
	}
	if got.NoSpeechThreshold != 0.5 {
		t.Errorf("NoSpeechThreshold: got %v, want 0.5", got.NoSpeechThreshold)
	}
	if got.GreedyBestOf == nil || *got.GreedyBestOf != 3 {
		t.Errorf("GreedyBestOf: got %v, want pointer to 3", got.GreedyBestOf)
	}
	if got.BeamSize != 5 {
		t.Errorf("BeamSize: got %d, want 5", got.BeamSize)
	}
	if got.BeamSearchPatience == nil || *got.BeamSearchPatience != 1.2 {
		t.Errorf("BeamSearchPatience: got %v, want pointer to 1.2", got.BeamSearchPatience)
	}
	if got.LengthPenalty == nil || *got.LengthPenalty != -0.5 {
		t.Errorf("LengthPenalty: got %v, want pointer to -0.5", got.LengthPenalty)
	}
}
