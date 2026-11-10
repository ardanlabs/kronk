// This example shows you how to generate speech with Kronk.
//
// The first time you run this program the system will download and install
// the model, its matching audio decoder, and the native libraries.
// It writes a complete PCM16 mono WAV to speech.wav in the working directory.
// Speech detection and decoder discovery are automatic; WithSpeech is optional.
// The verified bundles are Chatterbox Turbo and Qwen3-TTS 1.7B Base on Metal.
// Change modelSource below to switch between them, without a decoder URL.
// Streaming audio, voice cloning, and a language selector are not exposed.
//
// Run the example like this from the root of the project:
// $ make example-speech
// Play the result on macOS:
// $ afplay examples/speech.wav

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

const (
	// modelSource = "ggml-org/chatterbox-turbo-Q8_0"
	modelSource = "ggml-org/Qwen3-TTS-12Hz-1.7B-Base-Q8_0"
	outputFile  = "speech.wav"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run() (err error) {
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("unable to install system: %w", err)
	}

	krn, err := newKronk(mp)
	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	defer func() {
		fmt.Println("\nUnloading Kronk")
		if unloadErr := krn.Unload(context.Background()); unloadErr != nil {
			err = errors.Join(err, fmt.Errorf("unloading model: %w", unloadErr))
		}
	}()

	return speech(krn)
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithDetect(ctx, kronk.FmtLogger),
		libs.WithValidation(true),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	if err := kronk.Init(kronk.WithLibPath(libs.LibsPath())); err != nil {
		return models.Path{}, fmt.Errorf("unable to init kronk: %w", err)
	}

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to init models: %w", err)
	}

	mp, err := mdls.Download(ctx, kronk.FmtLogger, modelSource)
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install speech model: %w", err)
	}

	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	fmt.Println("loading model...")

	krn, err := kronk.New(
		model.WithModelFiles(mp.ModelFiles),
		model.WithProjFile(mp.ProjFile),
		// Limits default to 512 frames; optionally lower them with
		// model.WithSpeech(&model.SpeechConfig{MaxFrames: 256}).
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create speech model: %w", err)
	}

	fmt.Print("- system info:\n\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	cfg, info := krn.ModelConfig(), krn.ModelInfo()
	fmt.Println("- speech         :", info.IsSpeechModel)
	fmt.Println("- architecture   :", info.Architecture)
	fmt.Println("- contextWindow  :", cfg.ContextWindow())
	fmt.Printf("- k/v            : %s/%s\n", cfg.CacheTypeK, cfg.CacheTypeV)
	fmt.Println("- prefill batch  :", cfg.PrefillBatchSize())
	fmt.Println("- nSeqMax        :", cfg.NSeqMax())
	fmt.Println("- maxFrames      :", cfg.SpeechMaxFrames())

	return krn, nil
}

func speech(krn *kronk.Kronk) error {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	request := model.SpeechRequest{
		Input: "Mary had a little lamb, Its fleece was white as snow; And everywhere that Mary went, The lamb was sure to go.",
		Voice: "", // Empty uses default conditioning, even without named presets.
	}
	fmt.Println("\nTEXT:", request.Input)

	started := time.Now()
	response, err := krn.Speech(ctx, request)
	if err != nil {
		return fmt.Errorf("speech: %w", err)
	}
	elapsed := time.Since(started)

	if err := os.WriteFile(outputFile, response.Audio, 0644); err != nil {
		return fmt.Errorf("writing speech WAV: %w", err)
	}

	fmt.Printf("\nWrote %s: PCM16 mono WAV, %d Hz, %d samples, %.2f seconds, %d bytes\n",
		outputFile, response.SampleRate, response.Samples, float64(response.Samples)/float64(response.SampleRate), len(response.Audio))
	fmt.Printf("Generated in %s\n", elapsed)
	fmt.Println("Play from this directory on macOS: afplay", outputFile)

	return nil
}
