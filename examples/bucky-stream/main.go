// This example is a LIVE MICROPHONE transcription demo for the bucky
// streaming API. It captures the default input device, streams the audio
// through a *model.Stream, and renders the transcript live: partial
// hypotheses are re-rendered in place (words appear, then get revised as
// you keep talking), and finals are committed on their own line — the same
// effect as whisper.cpp's stream example. Say "STOP" to end.
//
// The streaming SDK itself is pure Go (no CGO). This example adds CGO only
// for microphone capture via github.com/gen2brain/malgo (miniaudio), which
// lives entirely in the examples module.
//
// Run the example like this from the root of the project:
// $ make example-bucky-stream
// $ make example-bucky-stream-vad

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/bucky/model"
	buckylibs "github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	"github.com/gen2brain/malgo"
)

// modelSource names the bucky whisper model to download.
const modelSource = "ggml-tiny.bin"

// vadModelSource names the optional Silero VAD model to download.
const vadModelSource = "silero-vad"

// micRate / micChannels are the format we ask the capture device for.
// miniaudio converts the hardware's native format to this for us, so we
// hand the stream exactly the 16 kHz mono int16 it wants with no resample.
const (
	micRate     = 16000
	micChannels = 1
)

// ANSI helpers for the live-rewrite UX. eraseLine clears the current line
// and returns the cursor to column 0 so the next print overwrites it —
// this is what produces the "words change as you talk" effect.
const (
	eraseLine = "\033[2K\r"
	colYellow = "\033[33m"
	colGreen  = "\033[32m"
	colRed    = "\033[31m"
	colReset  = "\033[0m"
)

type installedModels struct {
	whisper buckymodels.Path
	vadPath string
}

func main() {
	sileroVAD := flag.Bool("silero-vad", false, "use the Silero model instead of energy-based VAD")
	vadThreshold := flag.Float64("vad-threshold", 0.5, "Silero speech probability threshold (0, 1]")
	flag.Parse()

	if err := run(*sileroVAD, float32(*vadThreshold)); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run(sileroVAD bool, vadThreshold float32) error {
	if sileroVAD && (vadThreshold <= 0 || vadThreshold > 1) {
		return fmt.Errorf("vad threshold %v is outside (0, 1]", vadThreshold)
	}

	models, err := installSystem(sileroVAD)
	if err != nil {
		return fmt.Errorf("install system: %w", err)
	}

	b, err := newBucky(models.whisper)
	if err != nil {
		return fmt.Errorf("new bucky: %w", err)
	}
	defer func() {
		fmt.Println("Unloading whisper")
		if err := b.Unload(context.Background()); err != nil {
			fmt.Printf("unload: %v\n", err)
		}
	}()

	if err := liveTranscribe(b, models.vadPath, vadThreshold); err != nil {
		return fmt.Errorf("live transcribe: %w", err)
	}

	return nil
}

// =============================================================================

// liveTranscribe opens a streaming session, wires the default microphone
// into it, and renders the transcript live until the speaker says "STOP"
// (or presses Ctrl-C).
func liveTranscribe(b *bucky.Bucky, vadPath string, vadThreshold float32) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Energy-based VAD remains the zero-configuration default. When
	// --silero-vad is set, these options replace it with learned speech
	// probabilities from the downloaded Silero model.
	streamOpts := []model.StreamOption{
		model.WithStreamLanguage("en"),
		model.WithPartialEveryMs(700),
	}
	if vadPath != "" {
		streamOpts = append(streamOpts,
			model.WithVADModel(vadPath),
			model.WithVADSpeechThreshold(vadThreshold),
		)
	}

	stream, err := b.NewStream(ctx, streamOpts...)
	if err != nil {
		return fmt.Errorf("new stream: %w", err)
	}
	defer stream.Close()

	// Consumer: render partials in place, commit finals, and watch for the
	// spoken "STOP" command. Runs until Events closes (after stream.Close).
	saidStop := make(chan struct{})
	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		consume(stream, saidStop)
	}()

	// The audio callback runs on miniaudio's realtime thread; it must never
	// block. It copies the captured bytes and hands them to a buffered
	// channel that a pump goroutine drains into the stream.
	pcmC := make(chan []byte, 64)
	onFrames := func(_, in []byte, _ uint32) {
		select {
		case pcmC <- append([]byte(nil), in...):
		default: // pump is behind; drop rather than stall the audio thread
		}
	}

	device, mctx, err := openMic(onFrames)
	if err != nil {
		return fmt.Errorf("open mic: %w", err)
	}
	defer func() {
		device.Uninit()
		_ = mctx.Uninit()
		mctx.Free()
	}()

	// Pump: convert + feed raw mic PCM into the stream. FeedPCM does the
	// pure-Go int16 -> float32 conversion (and would downmix/resample if
	// the format differed from the engine's 16 kHz mono).
	micFormat := model.AudioFormat{SampleRate: micRate, Channels: micChannels, Sample: model.Int16LE}
	go func() {
		for buf := range pcmC {
			if err := stream.FeedPCM(ctx, buf, micFormat); err != nil {
				return
			}
		}
	}()

	if err := device.Start(); err != nil {
		return fmt.Errorf("start mic: %w", err)
	}

	fmt.Printf("\n%s🎤 Mic is live — say something. Say \"STOP\" to end.%s\n\n", colGreen, colReset)

	// Wait for the spoken stop word or Ctrl-C.
	select {
	case <-saidStop:
	case <-ctx.Done():
	}

	fmt.Printf("\n\nStopping…\n")
	_ = device.Stop()
	close(pcmC)        // pump exits
	_ = stream.Close() // final flush + closes Events
	<-consumerDone     // let the consumer print the closing Final

	return nil
}

// consume renders transcript events. Partials overwrite the live line;
// Finals commit on their own line. The moment the word "stop" appears in
// either a partial or a final, it signals saidStop once so the program
// ends immediately rather than waiting for the next pause.
func consume(stream *model.Stream, saidStop chan struct{}) {
	signaled := false
	signalStop := func(text string) {
		if !signaled && containsWord(text, "stop") {
			signaled = true
			close(saidStop)
		}
	}

	for ev := range stream.Events() {
		switch ev.Kind {
		case model.EventPartial:
			fmt.Printf("%s%s%s%s", eraseLine, colYellow, ev.Text, colReset)
			signalStop(ev.Text)

		case model.EventFinal:
			fmt.Printf("%s%s%s%s\n", eraseLine, colGreen, ev.Text, colReset)
			signalStop(ev.Text)

		case model.EventError:
			fmt.Printf("\n%serror: %v%s\n", colRed, ev.Err, colReset)
		}
	}
}

// containsWord reports whether text contains word as a whole word,
// case-insensitively and ignoring surrounding punctuation (so "Stop.",
// "STOP!" and "stop" all match).
func containsWord(text, word string) bool {
	for f := range strings.FieldsSeq(strings.ToLower(text)) {
		if strings.Trim(f, ".,!?;:\"'`-") == word {
			return true
		}
	}
	return false
}

// =============================================================================

// openMic initializes the miniaudio context and the default capture device
// configured for 16 kHz mono int16, wiring onFrames as the data callback.
func openMic(onFrames malgo.DataProc) (*malgo.Device, *malgo.AllocatedContext, error) {
	mctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, func(string) {})
	if err != nil {
		return nil, nil, fmt.Errorf("init audio context: %w", err)
	}

	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatS16
	cfg.Capture.Channels = micChannels
	cfg.SampleRate = micRate
	cfg.Alsa.NoMMap = 1

	device, err := malgo.InitDevice(mctx.Context, cfg, malgo.DeviceCallbacks{Data: onFrames})
	if err != nil {
		_ = mctx.Uninit()
		mctx.Free()
		return nil, nil, fmt.Errorf("init capture device: %w", err)
	}

	return device, mctx, nil
}

// =============================================================================

func installSystem(sileroVAD bool) (installedModels, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	lib, err := buckylibs.New(buckylibs.WithDetect(ctx, bucky.FmtLogger))
	if err != nil {
		return installedModels{}, fmt.Errorf("libs new: %w", err)
	}

	if _, err := lib.Download(ctx, bucky.FmtLogger); err != nil {
		return installedModels{}, fmt.Errorf("download whisper.cpp libs: %w", err)
	}
	if err := bucky.Init(bucky.WithLibPath(lib.LibsPath())); err != nil {
		return installedModels{}, fmt.Errorf("bucky init: %w", err)
	}

	mdls, err := buckymodels.New()
	if err != nil {
		return installedModels{}, fmt.Errorf("models new: %w", err)
	}

	fmt.Println("Downloading whisper model:", modelSource)

	mp, err := mdls.Download(ctx, bucky.FmtLogger, modelSource)
	if err != nil {
		return installedModels{}, fmt.Errorf("download model: %w", err)
	}

	models := installedModels{whisper: mp}
	if !sileroVAD {
		return models, nil
	}

	fmt.Println("Downloading VAD model:", vadModelSource)

	vad, err := mdls.Download(ctx, bucky.FmtLogger, vadModelSource)
	if err != nil {
		return installedModels{}, fmt.Errorf("download VAD model: %w", err)
	}
	if len(vad.ModelFiles) == 0 {
		return installedModels{}, fmt.Errorf("download VAD model: no model files on disk")
	}
	models.vadPath = vad.ModelFiles[0]

	return models, nil
}

func newBucky(mp buckymodels.Path) (*bucky.Bucky, error) {
	fmt.Println("Initializing bucky / whisper.cpp")

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
