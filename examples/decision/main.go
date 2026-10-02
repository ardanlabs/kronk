// This example shows you how to use a decision model.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ cd examples && go run ./decision

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

// modelSource is the model to download. It may be a HuggingFace URL,
// a canonical "provider/modelID", or a bare model id.
var modelSource = "chaoliangUNSW/Jev-Style-0.8B-Decision-v3-Q8_0"

// The ggml-org models require a llama.cpp build containing PR #29818.
// var modelSource = "ggml-org/Julia-1-Q8_0"
// var modelSource = "ggml-org/Laya-Q8_0"
// var modelSource = "ggml-org/lev-Q4_K_M"
// var modelSource = "ggml-org/Kev-4B-Q4_K_M"
// var modelSource = "ggml-org/OpenJev-Q4_K_M"
// var modelSource = "openjev/OpenJev-Q4_K_M"

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
	fmt.Println("- template       :", krn.ModelInfo().Template.FileName)
	fmt.Println("- nSeqMax        :", krn.ModelConfig().NSeqMax())
	fmt.Println("- vramTotal      :", krn.ModelInfo().VRAMTotal/(1024*1024), "MiB")
	fmt.Println("- modelSize      :", krn.ModelInfo().Size/(1000*1000), "MB")

	return krn, nil
}

func decision(krn *kronk.Kronk) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	request := model.DecisionRequest{
		State: model.DecisionState(
			model.DecisionStateData("customer_message", "I was charged twice and need this fixed today."),
			model.DecisionStateData("account_tier", "business"),
		),
		Questions: []model.DecisionQuestion{
			model.DecisionQuestionChoice("route", "Which team should handle this request?",
				model.DecisionQuestionOpt("billing", "Payments, invoices, refunds, and duplicate charges"),
				model.DecisionQuestionOpt("technical_support", "Product bugs and technical problems"),
				model.DecisionQuestionOpt("sales", "Plans, pricing, and new purchases"),
			),
			model.DecisionQuestionScore("urgency", "How urgent is this request?",
				"not urgent",
				"normal",
				"urgent",
				"critical",
			),
			model.DecisionQuestionNoul("requires_human", "Should a human review this request?", nil),
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
