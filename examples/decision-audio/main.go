// This example shows you how to evaluate audio with an LFM3-d1-omni decision model.
// It asks choice, noul, and score questions about the bundled JFK speech sample
// directly from audio, without first generating a transcript.
//
// The first time you run this program the system will download and install
// the model, its audio projector, and libraries into the normal Kronk store.
//
// Run the example from the root of the project:
// $ make example-decision-audio

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

var modelSource = "LiquidAI/d1-Omni-2B-Q8_0"

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
		return fmt.Errorf("unable to install system: %w", err)
	}

	krn, err := newKronk(mp)
	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}
	defer func() {
		fmt.Println("\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload model: %v", err)
		}
	}()

	return decision(krn)
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
	fmt.Println("Downloading model:", modelSource)
	mp, err := mdls.Download(ctx, kronk.FmtLogger, modelSource)
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install model: %w", err)
	}
	return mp, nil
}

func newKronk(mp models.Path) (*kronk.Kronk, error) {
	fmt.Println("loading model...")
	krn, err := kronk.New(
		model.WithModelFiles(mp.ModelFiles),
		model.WithProjFile(mp.ProjFile),
		model.WithAutoTune(true),
		// Decision models evaluate each complete prompt in one physical batch.
		// Bound the context for this short recording to avoid a large workspace.
		model.WithContextWindow(4096),
		model.WithNSeqMax(1),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create decision model: %w", err)
	}

	fmt.Println("- model          :", modelSource)
	fmt.Println("- audio          :", audioFile)
	fmt.Println("- decision       :", krn.ModelConfig().DecisionProtocol)
	fmt.Println("- architecture   :", krn.ModelInfo().Architecture)
	fmt.Println("- contextWindow  :", krn.ModelConfig().ContextWindow())
	fmt.Println("- nSeqMax        :", krn.ModelConfig().NSeqMax())
	fmt.Println("- modelSize      :", krn.ModelInfo().Size/(1000*1000), "MB")
	return krn, nil
}

func decision(krn *kronk.Kronk) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	audio, err := os.ReadFile(audioFile)
	if err != nil {
		return fmt.Errorf("read audio: %w", err)
	}
	request := model.DecisionRequest{
		State: "A recording of a public speech.",
		// Files supplies the encoded audio directly to the model's projector.
		Files: [][]byte{audio},
		Questions: []model.DecisionQuestion{
			model.DecisionQuestionChoice("subject", "What is the subject of this speech?",
				model.DecisionQuestionOpt("service", "Serving one's country"),
				model.DecisionQuestionOpt("weather", "A weather forecast"),
				model.DecisionQuestionOpt("cooking", "A recipe for dinner"),
			),
			model.DecisionQuestionNoul("mentions_country", "Does the speaker mention their country?", nil),
			model.DecisionQuestionScore("speech", "Does this clip contain human speech?",
				"No human speech", "Unclear speech", "Clear human speech",
			),
		},
	}
	response, err := krn.Decision(ctx, request)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
