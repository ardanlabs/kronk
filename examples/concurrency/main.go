// This example shows you how to leverage Kronk's batch processing by running
// multiple inference requests concurrently against a single loaded model. It
// classifies trail-cam images using a small vision model.
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
	"sort"
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

const (
	modelSource    = "unsloth/Qwen3.5-0.8B-Q8_0"
	imageLocation  = "samples/deer"
	numWorkers     = 2
	numRequests    = 1500
	requestTimeout = 60 * time.Second
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
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
			fmt.Printf("failed to unload model: %v\n", err)
		}
	}()

	return classifyImages(krn)
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
	fmt.Println("Loading model...")

	if err := kronk.Init(); err != nil {
		return nil, fmt.Errorf("unable to init kronk: %w", err)
	}

	krn, err := kronk.New(
		model.WithModelFiles(mp.ModelFiles),
		model.WithProjFile(mp.ProjFile),
		model.WithAutoTune(true),
		model.WithIncrementalCache(false),
		model.WithContextWindow(8*1024),
		model.WithNSeqMax(2),
		model.WithLog(kronk.FmtLogger),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create inference model: %w", err)
	}

	printModelInfo(krn)

	return krn, nil
}

func printModelInfo(krn *kronk.Kronk) {
	info := krn.SystemInfo()
	keys := make([]string, 0, len(info))
	for k := range info {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Print("- system info:\n\t")
	for _, k := range keys {
		fmt.Printf("%s:%v, ", k, info[k])
	}
	fmt.Println()

	cfg := krn.ModelConfig()
	mi := krn.ModelInfo()

	fmt.Println("- contextWindow  :", cfg.ContextWindow())
	fmt.Printf("- k/v            : %s/%s\n", cfg.CacheTypeK, cfg.CacheTypeV)
	fmt.Println("- flashAttention :", cfg.FlashAttention)
	fmt.Println("- nBatch         :", cfg.NBatch())
	fmt.Println("- nuBatch        :", cfg.NUBatch())
	fmt.Println("- modelType      :", mi.Type)
	fmt.Println("- isGPT          :", mi.IsGPTModel)
	fmt.Println("- template       :", mi.Template.FileName)
	fmt.Println("- grammar        :", cfg.DefaultParams.Grammar != "")
	fmt.Println("- nSeqMax        :", cfg.NSeqMax())
	fmt.Println("- vramTotal      :", mi.VRAMTotal/(1024*1024), "MiB")
	fmt.Println("- slotMemory     :", mi.SlotMemory/(1024*1024), "MiB")
	fmt.Println("- modelSize      :", mi.Size/(1000*1000), "MB")
	fmt.Println("- imc            :", cfg.IncrementalCache())
	if n := cfg.PtrNGpuLayers; n != nil {
		fmt.Println("- nGPULayers     :", *n)
	} else {
		fmt.Println("- nGPULayers     : all")
	}
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
users identify images and provide information in a helpful and accurate manner.
Always follow the user's instructions carefully.`

func classifyImages(krn *kronk.Kronk) error {
	imageFiles, err := listImages(imageLocation)
	if err != nil {
		return fmt.Errorf("listImages: %w", err)
	}

	fmt.Printf("\n- Number of images: %d\n", len(imageFiles))

	if len(imageFiles) == 0 {
		return fmt.Errorf("no images to process")
	}

	// -------------------------------------------------------------------------
	// Start a pool of workers. Each worker pulls image paths off the channel,
	// runs inference, and prints the result.

	ch := make(chan string)
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for id := range numWorkers {
		go func() {
			defer func() {
				fmt.Printf("g[%d]: done\n", id)
				wg.Done()
			}()

			for imageFile := range ch {
				processImage(krn, id, imageFile)
			}
		}()
	}

	// -------------------------------------------------------------------------
	// Send numRequests randomly chosen images through the pool, then close the
	// channel and wait for the workers to drain.

	for range numRequests {
		ch <- imageFiles[rand.IntN(len(imageFiles))]
	}

	close(ch)
	wg.Wait()

	return nil
}

func processImage(krn *kronk.Kronk, workerID int, imageFile string) {
	traceID := uuid.NewString()

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	ctx = applog.SetTraceID(ctx, traceID)

	imageData, err := os.ReadFile(imageFile)
	if err != nil {
		fmt.Printf("g[%d]: traceID %s: image %s: ERROR: read image: %s\n", workerID, traceID, imageFile, err)
		return
	}

	imageType := "jpeg"
	if strings.EqualFold(filepath.Ext(imageFile), ".png") {
		imageType = "png"
	}

	params := model.D{
		"messages": model.Messages(
			model.TextMessage(model.RoleSystem, systemPrompt),
			model.ImageMessage(prompt, imageData, imageType),
		),
		"enable_thinking": false,
		"temperature":     1.0,
		"top_p":           0.95,
		"top_k":           64,
		"max_tokens":      2048,
	}

	resp, err := krn.Chat(ctx, params)
	if err != nil {
		fmt.Printf("g[%d]: traceID %s: image %s: ERROR: chat streaming: %s\n", workerID, traceID, imageFile, err)
		return
	}

	fmt.Printf("g[%d]: traceID %s: image %s: Resp: %s\n", workerID, traceID, imageFile, strings.Trim(resp.Choices[0].Message.Content, "\n"))
}

func listImages(imageLocation string) ([]string, error) {
	entries, err := os.ReadDir(imageLocation)
	if err != nil {
		return nil, fmt.Errorf("unable to read directory %q: %w", imageLocation, err)
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		files = append(files, filepath.Join(imageLocation, entry.Name()))
	}

	return files, nil
}
