// This example shows you how to use an image with a vision-capable Clef decision model.
//
// The first time you run this program the system will download and install
// the model and libraries. The model source must include a compatible vision
// projector and an image-capable systemone template.
//
// Run the example from the root of the project:
// $ make example-decision-image

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

var modelSource = "ggml-org/Clef-Flash-Q4_K_M"

const imageFile = "samples/giraffe.jpg"

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

	if err := decision(krn); err != nil {
		return err
	}

	return nil
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
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create decision model: %w", err)
	}

	fmt.Print("- system info:\n\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow  :", krn.ModelConfig().ContextWindow())
	fmt.Printf("- k/v            : %s/%s\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
	fmt.Println("- flashAttention :", krn.ModelConfig().FlashAttention())
	fmt.Println("- prefill batch  :", krn.ModelConfig().PrefillBatchSize())
	fmt.Println("- decision       :", krn.ModelInfo().IsDecisionModel)
	fmt.Println("- modelType      :", krn.ModelInfo().Type)
	fmt.Println("- sequence type  :", krn.ModelInfo().SequenceProcessing)
	fmt.Println("- architecture   :", krn.ModelInfo().Architecture)
	fmt.Println("- template       :", krn.ModelInfo().Template.FileName)
	fmt.Println("- nSeqMax        :", krn.ModelConfig().NSeqMax())
	fmt.Println("- vramTotal      :", krn.ModelInfo().VRAMTotal/(1024*1024), "MiB")
	fmt.Println("- modelSize      :", krn.ModelInfo().Size/(1000*1000), "MB")

	return krn, nil
}

func decision(krn *kronk.Kronk) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	image, err := os.ReadFile(imageFile)
	if err != nil {
		return fmt.Errorf("read image: %w", err)
	}

	request := model.DecisionRequest{
		State:  "The attached photograph was taken during a wildlife survey.",
		Images: [][]byte{image},
		Questions: []model.DecisionQuestion{
			model.DecisionQuestionChoice("animal", "Which animal is pictured?",
				model.DecisionQuestionOpt("giraffe", "A giraffe"),
				model.DecisionQuestionOpt("deer", "A deer"),
				model.DecisionQuestionOpt("elephant", "An elephant"),
			),
			model.DecisionQuestionNoul("long_neck", "Does the pictured animal have a long neck?", nil),
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
