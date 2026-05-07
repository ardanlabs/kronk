// This example shows you how to create a concurrent chat application against an
// inference model using kronk. Thanks to Kronk and yzma, reasoning and tool
// calling is enabled.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-concurrency

package main

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/google/uuid"
)

var modelSource = "unsloth/Qwen3.5-0.8B-Q8_0"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	info, err := installSystem()
	if err != nil {
		return fmt.Errorf("unable to install system: %w", err)
	}

	krn, err := newKronk(info)
	if err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	defer func() {
		fmt.Println("\nUnloading Kronk")
		if err := krn.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload model: %v", err)
		}
	}()

	return runVisionTest(krn)
}

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithVersion(defaults.LibVersion("")),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install llama.cpp: %w", err)
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

	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(
		model.WithModelFiles(mp.ModelFiles),
		model.WithProjFile(mp.ProjFile),
		//model.WithLog(kronk.FmtLogger),
		model.WithIncrementalCache(false),
		model.WithContextWindow(8*1024),
		model.WithNSeqMax(2),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	fmt.Print("- system info:\n\t")
	for k, v := range krn.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow  :", krn.ModelConfig().ContextWindow())
	fmt.Printf("- k/v            : %s/%s\n", krn.ModelConfig().CacheTypeK, krn.ModelConfig().CacheTypeV)
	fmt.Println("- flashAttention :", krn.ModelConfig().FlashAttention)
	fmt.Println("- nBatch         :", krn.ModelConfig().NBatch())
	fmt.Println("- nuBatch        :", krn.ModelConfig().NUBatch())
	fmt.Println("- modelType      :", krn.ModelInfo().Type)
	fmt.Println("- isGPT          :", krn.ModelInfo().IsGPTModel)
	fmt.Println("- template       :", krn.ModelInfo().Template.FileName)
	fmt.Println("- grammar        :", krn.ModelConfig().DefaultParams.Grammar != "")
	fmt.Println("- nSeqMax        :", krn.ModelConfig().NSeqMax())
	fmt.Println("- vramTotal      :", krn.ModelInfo().VRAMTotal/(1024*1024), "MiB")
	fmt.Println("- slotMemory     :", krn.ModelInfo().SlotMemory/(1024*1024), "MiB")
	fmt.Println("- modelSize      :", krn.ModelInfo().Size/(1000*1000), "MB")
	fmt.Println("- imc            :", krn.ModelConfig().IncrementalCache())
	if n := krn.ModelConfig().PtrNGpuLayers; n != nil {
		fmt.Println("- nGPULayers     :", *n)
	} else {
		fmt.Println("- nGPULayers     : all")
	}

	return krn, nil
}

const prompt = `Analyze the attached trail cam picture and determine if there
are any deer in that picture. If there are deer determine if any of the deer
have antlers. If there is a deer with antlers return: Buck. If there is deer but
none with antlers return: Doe. If there are no deer in the picture return: None.
Analyze carefully, because the deer can be behind some grasses or trees.
Sometimes the deer antlers can be obstructed by trees or grasses. You can only
respond with 1 of 3 possible values, value 1: Buck, value 2: Doe or value 3:
None. Do not return any other characters.
 `

const systemPrompt = `You are a helpful AI assistant. You are designed to help
users indetify images and provide information in a helpful and accurate manner.
Always follow the user's instructions carefully.`

func runVisionTest(krn *kronk.Kronk) error {
	const imageLocation = "samples/deer"

	imageFiles, err := listImages(imageLocation)
	if err != nil {
		return fmt.Errorf("listImages: %w", err)
	}

	fmt.Printf("\n- Number of images: %d\n", len(imageFiles))

	if len(imageFiles) == 0 {
		return fmt.Errorf("no images to processing")
	}

	// -------------------------------------------------------------------------

	const g = 4

	ch := make(chan string, g)
	var wg sync.WaitGroup
	wg.Add(g)

	for gc := range g {
		go func(gc int) {
			defer func() {
				fmt.Printf("g[%d]: SHUTTING DOWN G\n", gc)
				wg.Done()
			}()

			for imageFile := range ch {
				traceID := uuid.NewString()

				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				ctx = applog.SetTraceID(ctx, traceID)

				imageData, err := readImage(imageFile)
				if err != nil {
					fmt.Printf("g[%d]: traceID %s: image %s: ERROR: read image: %s\n", gc, traceID, imageFile, err)
					cancel()
					continue
				}

				d := model.D{
					"messages": model.Messages(
						model.TextMessage(model.RoleSystem, systemPrompt),
						model.RawMediaMessage(prompt, imageData),
					),
					"enable_thinking": false,
					"temperature":     1.0,
					"top_p":           0.95,
					"top_k":           64,
					"max_tokens":      2048,
				}

				mdlResp, err := krn.Chat(ctx, d)
				if err != nil {
					fmt.Printf("g[%d]: traceID %s: image %s: ERROR: chat streaming: %s\n", gc, traceID, imageFile, err)
					cancel()
					continue
				}

				cancel()

				fmt.Printf("g[%d]: traceID %s: image %s: Resp: %s\n", gc, traceID, imageFile, strings.Trim(mdlResp.Choices[0].Message.Content, "\n"))
			}
		}(gc)
	}

	// -------------------------------------------------------------------------

	for range 20 {
		i := rand.IntN(len(imageFiles))
		ch <- imageFiles[i]
	}

	close(ch)
	wg.Wait()

	return nil
}

func listImages(imageLocation string) ([]string, error) {
	entries, err := os.ReadDir(imageLocation)
	if err != nil {
		return nil, fmt.Errorf("unable to read directory %q: %w", imageLocation, err)
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == ".DS_Store" {
			continue
		}

		files = append(files, filepath.Join(imageLocation, entry.Name()))
	}

	return files, nil
}

func readImage(imageFile string) ([]byte, error) {
	if _, err := os.Stat(imageFile); err != nil {
		return nil, fmt.Errorf("error accessing file %q: %w", imageFile, err)
	}

	image, err := os.ReadFile(imageFile)
	if err != nil {
		return nil, fmt.Errorf("error reading file %q: %w", imageFile, err)
	}

	return image, nil
}
