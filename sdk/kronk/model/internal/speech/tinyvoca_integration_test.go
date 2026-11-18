//go:build speech_integration

package speech_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// TestTinyVocaSpeech is an opt-in native smoke test selected by make test-speech.
// Missing fixtures fail rather than skip; regular tests omit this build tag.
func TestTinyVocaSpeech(t *testing.T) {
	const id = "ggml-org/tiny-voca-F32"
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

	const text = "Hello! The meeting moved to 3:30 pm. Blorfle has 221 cats."
	var baseline []byte
	for _, tc := range []struct {
		name, voice, language string
	}{
		{name: "default M2"},
		{name: "F2 preset", voice: "F2", language: "english"},
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
				t.Error("M2 and F2 voices returned identical audio")
			}
		})
	}
	for _, request := range []model.SpeechRequest{
		{Input: "Hello", Language: "es"},
		{Input: "Hello", Voice: "unknown"},
		{Input: "Hello", Reference: model.SpeechReference{Audio: baseline}},
		{Input: strings.Repeat("cat ", 180)},
	} {
		response, err := krn.Speech(t.Context(), request)
		if !errors.Is(err, model.ErrInvalidRequest) || len(response.Audio) != 0 {
			t.Fatalf("unsupported input returned %v, bytes=%d", err, len(response.Audio))
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	response, err := krn.Speech(ctx, model.SpeechRequest{Input: text})
	if !errors.Is(err, context.Canceled) || len(response.Audio) != 0 {
		t.Fatalf("canceled speech: got %v, bytes=%d", err, len(response.Audio))
	}
	response, err = krn.Speech(t.Context(), model.SpeechRequest{Input: text, Voice: "M2", Language: "en-us"})
	if err != nil {
		t.Fatalf("reuse after rejected requests: %v", err)
	}
	checkSpeechWAV(t, response)
	if !bytes.Equal(baseline, response.Audio) {
		t.Error("default voice changed after F2, rejected requests and cancellation")
	}
}
