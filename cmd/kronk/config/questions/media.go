package questions

import (
	"errors"
	"fmt"
	"slices"

	"github.com/ardanlabs/kronk/cmd/kronk/config/configfile"
	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
)

// Audio returns the audio model questions, written under bucky-models:.
func Audio() []Question {
	return slices.Concat([]Question{
		{
			Key: "nseq-max", Kind: kindInt, min: 1,
			Title: "How many requests should run at the same time?",
			About: "Each transcription at the same time keeps its own state and memory.",
			Def:   text("1"),
			show:  always,
		},
	}, mediaQueueQuestions(), []Question{
		{
			Key: "nthreads", Kind: kindInt, min: 1,
			Title: "How many CPU threads?",
			Def:   text("whisper.cpp's choice"),
			show:  always,
		},
	})
}

// Image returns the image model questions, written under malina-models:.
func Image() []Question {
	return slices.Concat([]Question{
		{
			Key: "concurrency", Kind: kindInt, min: 1,
			Title: "How many images can be generated at the same time?",
			About: "Each one loads its own model context, so memory grows with it.",
			Def:   text("1"),
			show:  always,
		},
	}, mediaQueueQuestions(), []Question{
		{
			Key: "cpu-threads", Kind: kindInt, min: 1,
			Title: "How many CPU threads?",
			Def:   text("stable-diffusion.cpp's choice"),
			show:  always,
		},
	})
}

// mediaQueueQuestions are the queue questions audio and image models share.
func mediaQueueQuestions() []Question {
	return []Question{
		{
			Key: "queue-depth", Kind: kindInt, min: 0,
			Title: "How many requests can wait in line?",
			About: "Requests admitted to wait when every slot is busy.",
			Def:   text("0"),
			show:  always,
		},
		{
			Key: "admission-timeout", Kind: kindDuration, min: 1,
			Title: "How long can a request wait for a slot?",
			About: "After that the request fails. It doesn't limit how long the work takes.",
			Def:   text("3m"),
			show:  always,
		},
	}
}

// MediaExisting flattens the entry for id in the audio or image section.
func MediaExisting(doc modelconfig.Document, kind source.ModelKind, id string) (map[string]any, error) {
	switch kind {
	case source.KindAudioModel:
		if mc, ok := doc.BuckyModels[id]; ok {
			return configfile.FlattenYAML(mc)
		}
	case source.KindImageModel:
		if mc, ok := doc.MalinaModels[id]; ok {
			return configfile.FlattenYAML(mc)
		}
	}
	return nil, nil
}

// CheckMedia decodes an audio or image entry with the SDK's own types and
// validation, so a bad value is an error before the file is written.
func CheckMedia(kind source.ModelKind, flat map[string]any) error {
	switch kind {
	case source.KindAudioModel:
		var mc modelconfig.BuckyModelConfig
		if err := configfile.DecodeStrict(flat, &mc); err != nil {
			return fmt.Errorf("invalid audio model config: %w", err)
		}
		return mc.Validate()
	case source.KindImageModel:
		var mc modelconfig.MalinaModelConfig
		if err := configfile.DecodeStrict(flat, &mc); err != nil {
			return fmt.Errorf("invalid image model config: %w", err)
		}
		return mc.Validate()
	}

	return errors.New("not an audio or image model")
}
