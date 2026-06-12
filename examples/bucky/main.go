// This example shows you how to transcribe an audio file with the
// bucky SDK (whisper.cpp under the hood).
//
// The first time you run this program the system will download and
// install the whisper.cpp libraries and a small whisper model.
//
// Run the example like this from the root of the project:
// $ make example-bucky

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/bucky/pkg/audio"
	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/bucky/model"
	buckylibs "github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
)

// modelSource names the bucky whisper model to download. Valid short
// names are listed by models.SupportedModels().
const modelSource = "ggml-tiny.bin"

// audioFile is a 16 kHz mono WAV sample of JFK's "ask not" speech.
const audioFile = "samples/jfk.wav"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("install system: %w", err)
	}

	w, err := newBucky(mp)
	if err != nil {
		return fmt.Errorf("new whisper: %w", err)
	}
	defer func() {
		fmt.Println("\nUnloading whisper")
		if err := w.Unload(context.Background()); err != nil {
			fmt.Printf("unload: %v\n", err)
		}
	}()

	samples, err := loadSamples(audioFile)
	if err != nil {
		return fmt.Errorf("load samples: %w", err)
	}

	if err := transcribe(w, samples); err != nil {
		return fmt.Errorf("transcribe: %w", err)
	}

	if err := detectLanguage(w, samples); err != nil {
		return fmt.Errorf("detect language: %w", err)
	}

	return nil
}

// =============================================================================

func installSystem() (buckymodels.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	lib, err := buckylibs.New()
	if err != nil {
		return buckymodels.Path{}, fmt.Errorf("libs new: %w", err)
	}

	if _, err := lib.Download(ctx, bucky.FmtLogger); err != nil {
		return buckymodels.Path{}, fmt.Errorf("download whisper.cpp libs: %w", err)
	}

	mdls, err := buckymodels.New()
	if err != nil {
		return buckymodels.Path{}, fmt.Errorf("models new: %w", err)
	}

	fmt.Println("Downloading whisper model:", modelSource)

	mp, err := mdls.Download(ctx, bucky.FmtLogger, modelSource)
	if err != nil {
		return buckymodels.Path{}, fmt.Errorf("download model: %w", err)
	}

	return mp, nil
}

func newBucky(mp buckymodels.Path) (*bucky.Bucky, error) {
	fmt.Println("Initializing bucky / whisper.cpp")

	if err := bucky.Init(); err != nil {
		return nil, fmt.Errorf("bucky init: %w", err)
	}

	if len(mp.ModelFiles) == 0 {
		return nil, fmt.Errorf("no model files on disk")
	}

	b, err := bucky.New(
		model.WithModelPath(mp.ModelFiles[0]),
		model.WithUseGPU(true),
		model.WithLog(bucky.FmtLogger),
	)
	if err != nil {
		return nil, fmt.Errorf("create whisper handle: %w", err)
	}

	mi := b.ModelInfo()
	fmt.Println("- model           :", mi.ID)
	fmt.Println("- model type      :", mi.Type)
	fmt.Println("- multilingual    :", mi.IsMultilingual)
	fmt.Println("- text-ctx        :", mi.NTextCtx)
	fmt.Println("- audio-ctx       :", mi.NAudioCtx)
	fmt.Println("- mels            :", mi.NMels)
	fmt.Println("- vocab           :", mi.NVocab)
	fmt.Println("- active-streams  :", b.ActiveStreams())

	return b, nil
}

func loadSamples(path string) ([]float32, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	samples, err := audio.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode %q: %w", path, err)
	}

	return samples, nil
}

// =============================================================================

func transcribe(b *bucky.Bucky, samples []float32) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fmt.Println("\nTranscribing...")
	start := time.Now()

	tr, err := b.Transcribe(ctx, samples,
		model.WithLanguage("en"),
		model.WithOnSegment(func(seg model.Segment) {
			fmt.Printf("  segment %2d [%6dms → %6dms] %s\n",
				seg.Index, seg.StartMs, seg.EndMs, seg.Text)
		}),
	)
	if err != nil {
		return err
	}

	fmt.Println("\nFinal Transcription")
	fmt.Println("- language   :", tr.Language)
	fmt.Println("- segments   :", len(tr.Segments))
	fmt.Println("- text       :", tr.Text)
	fmt.Println("- elapsed    :", time.Since(start).Round(time.Millisecond))

	return nil
}

func detectLanguage(w *bucky.Bucky, samples []float32) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("\nDetecting language...")

	lang, probs, err := w.DetectLanguage(ctx, samples, true)
	if err != nil {
		return err
	}

	fmt.Println("- detected   :", lang)

	// Print top 5 candidates by probability.
	type cand struct {
		code string
		prob float32
	}
	tops := make([]cand, 0, 5)
	for id, p := range probs {
		c := cand{code: bucky.LangStr(int32(id)), prob: p}
		switch {
		case len(tops) < cap(tops):
			tops = append(tops, c)
		default:
			worstIdx := 0
			for i, t := range tops {
				if t.prob < tops[worstIdx].prob {
					worstIdx = i
				}
			}
			if c.prob > tops[worstIdx].prob {
				tops[worstIdx] = c
			}
		}
	}

	fmt.Println("- top 5      :")
	for _, c := range tops {
		fmt.Printf("    %-6s %.4f\n", c.code, c.prob)
	}

	return nil
}
