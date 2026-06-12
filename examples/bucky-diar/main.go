// This example shows you how to perform channel-separated speaker
// diarization with the bucky SDK (whisper.cpp under the hood). When each
// speaker is recorded on a dedicated channel — common in call-center and
// meeting captures — TranscribeChannelsFile decodes the audio preserving
// its channel layout, transcribes every channel on its own, and merges
// the results into a single transcript where each segment is tagged with
// the channel (speaker) it came from.
//
// The bundled sample is a 16 kHz stereo WAV with one speaker on each
// channel (speaker 0 on the left, speaker 1 on the right), so the merged,
// time-sorted transcript reads as a back-and-forth conversation.
//
// The first time you run this program the system will download and
// install the whisper.cpp libraries and a small whisper model.
//
// Run the example like this from the root of the project:
// $ make example-bucky-diar

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/bucky/model"
	buckylibs "github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
)

// modelSource names the bucky whisper model to download. Valid short
// names are listed by models.SupportedModels(). A multilingual model is
// used so each channel's language is auto-detected (the sample has an
// English speaker on the left and a Spanish speaker on the right).
const modelSource = "tiny"

// audioFile is a 16 kHz stereo WAV sample with one speaker per channel.
const audioFile = "samples/stereo-speakers.wav"

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

	b, err := newBucky(mp)
	if err != nil {
		return fmt.Errorf("new bucky: %w", err)
	}
	defer func() {
		fmt.Println("\nUnloading whisper")
		if err := b.Unload(context.Background()); err != nil {
			fmt.Printf("unload: %v\n", err)
		}
	}()

	if err := diarize(b, audioFile); err != nil {
		return fmt.Errorf("diarize: %w", err)
	}

	return nil
}

// =============================================================================

// diarize decodes the audio file preserving its channel layout, transcribes
// each channel as its own speaker, and prints both the per-speaker
// transcripts and the merged, time-sorted segment stream.
func diarize(b *bucky.Bucky, path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	fmt.Printf("\nDiarizing %s (one speaker per channel)...\n", path)
	start := time.Now()

	// No language is set so whisper auto-detects each channel's language
	// independently (English on the left, Spanish on the right).
	d, err := b.TranscribeChannelsFile(ctx, f)
	if err != nil {
		return err
	}

	// d.Channels holds one Transcription per source channel.
	fmt.Println("\nPer-Speaker Transcripts")
	for _, ct := range d.Channels {
		fmt.Printf("- speaker %d: %s\n", ct.Channel, ct.Text)
	}

	// d.Segments merges every channel's segments sorted by start time,
	// each tagged with the channel (speaker) it came from.
	fmt.Println("\nMerged Timeline")
	for _, s := range d.Segments {
		fmt.Printf("  [%6dms → %6dms] speaker %d: %s\n",
			s.StartMs, s.EndMs, s.Channel, s.Text)
	}

	fmt.Println("\n- speakers   :", len(d.Channels))
	fmt.Println("- segments   :", len(d.Segments))
	fmt.Println("- elapsed    :", time.Since(start).Round(time.Millisecond))

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
	fmt.Println("- active-streams  :", b.ActiveStreams())

	return b, nil
}
