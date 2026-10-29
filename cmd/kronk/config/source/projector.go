package source

import (
	"context"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// Inputs are what a model's projector (mmproj) turns into tokens besides
// text. They come from the projector's own GGUF header, where llama.cpp
// writes clip.has_vision_encoder and clip.has_audio_encoder. The catalog's
// capabilities guess them from model names and get some wrong: an
// audio-only projector is listed with images.
type Inputs struct {
	// Known is false when the projector's header couldn't be read, for
	// example offline. Its questions are then asked anyway.
	Known  bool
	Images bool
	Audio  bool
}

// projectorInputs reads the inputs of the model c's projector: from the
// local file when the model is installed, otherwise from the header of the
// file the download would fetch.
func (s *Source) projectorInputs(ctx context.Context, c Choice) Inputs {
	var data []byte

	if c.Installed {
		mp, err := s.mdls.FullPath(c.ID)
		if err != nil || mp.ProjFile == "" {
			return Inputs{}
		}
		if data, err = gguf.ReadHeaderBytes(mp.ProjFile); err != nil {
			return Inputs{}
		}
	} else {
		rfile, err := defaults.CatalogFile("", s.basePath)
		if err != nil {
			return Inputs{}
		}

		ctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()

		res, err := models.NewResolver(s.mdls, rfile).Resolve(ctx, c.ID)
		if err != nil || res.DownloadProj == "" {
			return Inputs{}
		}
		if data, _, err = gguf.FetchHeaderBytes(ctx, res.DownloadProj); err != nil {
			return Inputs{}
		}
	}

	md, err := gguf.ParseMetadata(data)
	if err != nil {
		return Inputs{}
	}
	return inputsFromMetadata(md)
}

// inputsFromMetadata reads the encoder flags of a projector's metadata. A
// projector with neither flag is too old to say, so its inputs are unknown.
func inputsFromMetadata(md map[string]string) Inputs {
	in := Inputs{Images: md["clip.has_vision_encoder"] == "true", Audio: md["clip.has_audio_encoder"] == "true"}
	in.Known = in.Images || in.Audio
	return in
}
