// This example shows the streaming transcription API surface for the
// bucky SDK. It demonstrates the Feed -> range Events -> Reset -> Close
// pattern for an indefinite session.
//
// NOTE: the streaming API is currently STUBBED. NewStream / Feed / Reset
// / Close return model.ErrNotImplemented. This example exists to show
// the intended call-site ergonomics, not to produce real transcripts.
//
// Run the example like this from the root of the project:
// $ make example-bucky-stream

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/bucky/pkg/audio"
	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/bucky/model"
	buckylibs "github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
)

// modelSource names the bucky whisper model to download.
const modelSource = "tiny.en"

// audioFile is a 16 kHz mono WAV sample of JFK's "ask not" speech.
const audioFile = "samples/jfk.wav"

// feedChunk is how many samples we push per Feed call to simulate audio
// arriving over time (≈200 ms at 16 kHz).
const feedChunk = 3200

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

	samples, err := loadSamples(audioFile)
	if err != nil {
		return fmt.Errorf("load samples: %w", err)
	}

	if err := streamTranscribe(b, samples); err != nil {
		return fmt.Errorf("stream transcribe: %w", err)
	}

	return nil
}

// =============================================================================

// streamTranscribe shows the intended streaming call site: open a stream,
// consume Events in a goroutine, Feed audio over time, Reset to start a
// fresh logical session without dropping the pool slot, then Close.
func streamTranscribe(b *bucky.Bucky, samples []float32) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fmt.Println("\nOpening stream...")

	stream, err := b.NewStream(ctx,
		model.WithStreamLanguage("en"),
		model.WithPartialEveryMs(1000),
		model.WithVAD(true),
		model.WithEmitResetEvent(true),
	)

	// The API is stubbed today; show the surface and stop gracefully.
	if errors.Is(err, model.ErrNotImplemented) {
		fmt.Println("- NewStream returned ErrNotImplemented (API is stubbed)")
		fmt.Println("- the code below shows how a real caller would drive it")
		return nil
	}
	if err != nil {
		return fmt.Errorf("new stream: %w", err)
	}
	defer stream.Close()

	// Consumer: range over Events. The channel closes after Close (or an
	// EventError). Partials are tentative; Finals are authoritative.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range stream.Events() {
			switch ev.Kind {
			case model.EventPartial:
				fmt.Printf("  ~ partial [%6dms] %s\n", ev.EndMs, ev.Text)
			case model.EventFinal:
				fmt.Printf("  = final   [%6dms] %s\n", ev.EndMs, ev.Text)
			case model.EventReset:
				fmt.Println("  -- reset --")
			case model.EventError:
				fmt.Printf("  ! error: %v\n", ev.Err)
			}
		}
	}()

	// Producer: Feed audio in chunks to simulate a live source. Feed
	// blocks (respecting ctx) when the internal buffer is full.
	for off := 0; off < len(samples); off += feedChunk {
		end := min(off+feedChunk, len(samples))

		if err := stream.Feed(ctx, samples[off:end]); err != nil {
			return fmt.Errorf("feed: %w", err)
		}

		time.Sleep(200 * time.Millisecond)
	}

	// Reset starts a fresh logical session (e.g. topic change, push-to-talk
	// release) WITHOUT releasing the pool slot or worker.
	fmt.Println("\nResetting session...")
	if err := stream.Reset(ctx); err != nil {
		return fmt.Errorf("reset: %w", err)
	}

	// ... feed the next session's audio here, reusing the same stream ...

	// Close performs a final flush, emits remaining Finals, and closes
	// Events. The consumer goroutine then returns.
	if err := stream.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	<-done

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
	fmt.Println("- multilingual    :", mi.IsMultilingual)
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
