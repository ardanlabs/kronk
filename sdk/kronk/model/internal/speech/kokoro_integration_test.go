//go:build speech_integration

package speech_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// TestKokoroSpeech is an opt-in native smoke test selected by make test-speech.
// Missing fixtures fail rather than skip; regular tests omit this build tag.
func TestKokoroSpeech(t *testing.T) {
	const id = "ggml-org/Kokoro-82M-F32"
	mdls, err := models.New()
	if err != nil {
		t.Fatal(err)
	}
	mp, err := mdls.FullPath(id)
	if err != nil || len(mp.ModelFiles) == 0 || mp.ProjFile == "" {
		t.Fatalf("missing speech fixture %s: %v; run make install-speak-models", id, err)
	}
	if err := kronk.Init(); err != nil {
		t.Fatal(err)
	}
	krn, err := kronk.NewWithContext(t.Context(), model.WithModelFiles(mp.ModelFiles), model.WithProjFile(mp.ProjFile))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := krn.Unload(context.Background()); err != nil {
			t.Error(err)
		}
	})

	const text = "There are 221 cats. The year is 2024."
	var baseline []byte
	for _, tc := range []struct {
		name, voice, language string
	}{
		{name: "default US", language: "en-us"},
		{name: "British preset", voice: "bf_emma", language: "en-gb"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response, err := krn.Speech(t.Context(), model.SpeechRequest{Input: text, Voice: tc.voice, Language: tc.language})
			if err != nil {
				t.Fatal(err)
			}
			checkSpeechWAV(t, response)
			if baseline == nil {
				baseline = bytes.Clone(response.Audio)
				return
			}
			if bytes.Equal(baseline, response.Audio) {
				t.Error("US and British voices returned identical audio")
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	response, err := krn.Speech(ctx, model.SpeechRequest{Input: text})
	if !errors.Is(err, context.Canceled) || len(response.Audio) != 0 {
		t.Fatalf("canceled speech: got %v, bytes=%d", err, len(response.Audio))
	}
	response, err = krn.Speech(t.Context(), model.SpeechRequest{Input: text, Language: "en-us"})
	if err != nil {
		t.Fatalf("reuse after cancellation: %v", err)
	}
	checkSpeechWAV(t, response)
	if !bytes.Equal(baseline, response.Audio) {
		t.Error("default voice changed after British voice and cancellation")
	}
}

func checkSpeechWAV(t *testing.T, response model.SpeechResponse) {
	t.Helper()
	wav := response.Audio
	if response.SampleRate != 24000 || response.Samples <= 0 || response.Stereo || len(wav) < 44 || len(wav) != 44+2*response.Samples {
		t.Fatalf("invalid speech output: rate=%d samples=%d stereo=%t bytes=%d", response.SampleRate, response.Samples, response.Stereo, len(wav))
	}
	if string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" ||
		binary.LittleEndian.Uint32(wav[4:8]) != uint32(len(wav)-8) ||
		binary.LittleEndian.Uint16(wav[20:22]) != 1 || binary.LittleEndian.Uint16(wav[22:24]) != 1 ||
		binary.LittleEndian.Uint32(wav[24:28]) != uint32(response.SampleRate) ||
		binary.LittleEndian.Uint16(wav[34:36]) != 16 || string(wav[36:40]) != "data" ||
		binary.LittleEndian.Uint32(wav[40:44]) != uint32(len(wav)-44) {
		t.Fatal("speech is not a complete PCM16 mono WAV")
	}
	for _, b := range wav[44:] {
		if b != 0 {
			t.Logf("WAV: rate=%d samples=%d bytes=%d", response.SampleRate, response.Samples, len(wav))
			return
		}
	}
	t.Fatal("speech output is silent")
}
