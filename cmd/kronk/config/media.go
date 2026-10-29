package config

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/ardanlabs/kronk/sdk/applog"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	malinamodels "github.com/ardanlabs/kronk/sdk/tools/malina/models"
	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
	yamlv2 "go.yaml.in/yaml/v2"
	"go.yaml.in/yaml/v3"
)

// Audio (Bucky, whisper.cpp) and image (Malina, stable-diffusion.cpp) models
// have their own entries under bucky-models: and malina-models:, and their own
// questions. They have no GGUF analysis.

// Model kinds in the picker.
const (
	kindChatModel  = ""
	kindAudioModel = "audio"
	kindImageModel = "image"
)

// sectionOfKind is the config file section a model kind is written to.
func sectionOfKind(k string) string {
	switch k {
	case kindAudioModel:
		return sectionBucky
	case kindImageModel:
		return sectionMalina
	}
	return sectionModels
}

func audioQuestions() []question {
	return []question{
		{
			key: "nseq-max", kind: kindInt, min: 1,
			title: "How many requests should run at the same time?",
			def:   func(facts) string { return "1" },
			show:  always,
		},
		{
			key: "queue-depth", kind: kindInt, min: 0,
			title: "How many requests can wait in line?",
			def:   func(facts) string { return "0" },
			show:  always,
		},
		{
			key: "admission-timeout", kind: kindDuration, min: 1,
			title: "How long can a request wait for a slot?",
			def:   func(facts) string { return "3m" },
			show:  always,
		},
		{
			key: "nthreads", kind: kindInt, min: 1,
			title: "How many CPU threads?",
			def:   func(facts) string { return "whisper.cpp's choice" },
			show:  always,
		},
	}
}

func imageQuestions() []question {
	return []question{
		{
			key: "concurrency", kind: kindInt, min: 1,
			title: "How many images can be generated at the same time?",
			def:   func(facts) string { return "1" },
			show:  always,
		},
		{
			key: "queue-depth", kind: kindInt, min: 0,
			title: "How many requests can wait in line?",
			def:   func(facts) string { return "0" },
			show:  always,
		},
		{
			key: "admission-timeout", kind: kindDuration, min: 1,
			title: "How long can a request wait for a slot?",
			def:   func(facts) string { return "3m" },
			show:  always,
		},
		{
			key: "cpu-threads", kind: kindInt, min: 1,
			title: "How many CPU threads?",
			def:   func(facts) string { return "stable-diffusion.cpp's choice" },
			show:  always,
		},
	}
}

// mediaChoices lists the installed audio and image models.
func (s *source) mediaChoices(doc modelconfig.Document) []choice {
	var out []choice

	if mdls, err := buckymodels.NewWithPaths(s.basePath); err == nil && mdls.BuildIndex(applog.DiscardLogger, false) == nil {
		files, _ := mdls.Files()
		for _, f := range files {
			_, inFile := doc.BuckyModels[f.ID]
			out = append(out, choice{ID: f.ID, Kind: kindAudioModel, Installed: true, Validated: true, InFile: inFile, Size: f.Size})
		}
	}

	if mdls, err := malinamodels.NewWithPaths(s.basePath); err == nil && mdls.BuildIndex(applog.DiscardLogger, false) == nil {
		for _, name := range malinamodels.SupportedBundles() {
			mp, err := mdls.FullPath(name.String())
			if err != nil {
				continue
			}
			var size int64
			for _, n := range mp.FileSizes {
				size += n
			}
			_, inFile := doc.MalinaModels[name.String()]
			out = append(out, choice{ID: name.String(), Kind: kindImageModel, Installed: true, Validated: true, InFile: inFile, Size: size})
		}
	}

	slices.SortFunc(out, func(a, b choice) int {
		return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.ID, b.ID))
	})

	return out
}

// mediaExisting flattens the entry for id in the audio or image section.
func mediaExisting(doc modelconfig.Document, kind string, id string) (map[string]any, error) {
	var entry any
	switch kind {
	case kindAudioModel:
		mc, ok := doc.BuckyModels[id]
		if !ok {
			return nil, nil
		}
		entry = mc
	case kindImageModel:
		mc, ok := doc.MalinaModels[id]
		if !ok {
			return nil, nil
		}
		entry = mc
	}

	data, err := yamlv2.Marshal(entry)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	out := map[string]any{}
	flatten("", m, out)
	return out, nil
}

// checkMedia decodes an audio or image entry with the SDK's own types and
// validation, so a bad value is an error before the file is written.
func checkMedia(kind string, flat map[string]any) error {
	data, err := yaml.Marshal(unflatten(flat))
	if err != nil {
		return err
	}

	switch kind {
	case kindAudioModel:
		var mc modelconfig.BuckyModelConfig
		if err := yamlv2.UnmarshalStrict(data, &mc); err != nil {
			return fmt.Errorf("invalid audio model config: %w", err)
		}
		return mc.Validate()
	case kindImageModel:
		var mc modelconfig.MalinaModelConfig
		if err := yamlv2.UnmarshalStrict(data, &mc); err != nil {
			return fmt.Errorf("invalid image model config: %w", err)
		}
		return mc.Validate()
	}

	return errors.New("not an audio or image model")
}
