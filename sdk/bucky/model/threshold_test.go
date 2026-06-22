package model

import "testing"

// The threshold overrides feed whisper.cpp's no-speech and log-probability
// acceptance gates. buildFullParams maps them into whisper_full_params, but
// that call dlopens the native library (see fakeDecoder for why the model
// package avoids it in unit tests), so these tests assert the option wiring
// and the sentinel/pointer semantics that decide whether buildFullParams
// applies an override at all.

func TestTranscribeThresholdOptions(t *testing.T) {
	// Zero value: no override requested on either path.
	var cfg TranscribeConfig
	if cfg.NoSpeechThreshold != 0 {
		t.Fatalf("default NoSpeechThreshold: got %v, want 0", cfg.NoSpeechThreshold)
	}
	if cfg.LogProbThreshold != nil {
		t.Fatalf("default LogProbThreshold: got %v, want nil", cfg.LogProbThreshold)
	}

	WithNoSpeechThreshold(0.7)(&cfg)
	if cfg.NoSpeechThreshold != 0.7 {
		t.Errorf("WithNoSpeechThreshold(0.7): got %v, want 0.7", cfg.NoSpeechThreshold)
	}

	// 0 must be expressible for LogProbThreshold (a maximally strict
	// threshold); that is the whole reason the field is a pointer rather
	// than a !=0 sentinel.
	WithLogProbThreshold(0)(&cfg)
	if cfg.LogProbThreshold == nil {
		t.Fatal("WithLogProbThreshold(0): got nil, want a settable 0")
	}
	if *cfg.LogProbThreshold != 0 {
		t.Errorf("WithLogProbThreshold(0): got %v, want 0", *cfg.LogProbThreshold)
	}

	WithLogProbThreshold(-2)(&cfg)
	if cfg.LogProbThreshold == nil || *cfg.LogProbThreshold != -2 {
		t.Errorf("WithLogProbThreshold(-2): got %v, want -2", cfg.LogProbThreshold)
	}
}

func TestStreamThresholdOptions(t *testing.T) {
	var cfg StreamConfig
	if cfg.NoSpeechThreshold != 0 {
		t.Fatalf("default NoSpeechThreshold: got %v, want 0", cfg.NoSpeechThreshold)
	}
	if cfg.LogProbThreshold != nil {
		t.Fatalf("default LogProbThreshold: got %v, want nil", cfg.LogProbThreshold)
	}

	WithStreamNoSpeechThreshold(0.7)(&cfg)
	if cfg.NoSpeechThreshold != 0.7 {
		t.Errorf("WithStreamNoSpeechThreshold(0.7): got %v, want 0.7", cfg.NoSpeechThreshold)
	}

	WithStreamLogProbThreshold(0)(&cfg)
	if cfg.LogProbThreshold == nil {
		t.Fatal("WithStreamLogProbThreshold(0): got nil, want a settable 0")
	}
	if *cfg.LogProbThreshold != 0 {
		t.Errorf("WithStreamLogProbThreshold(0): got %v, want 0", *cfg.LogProbThreshold)
	}
}
