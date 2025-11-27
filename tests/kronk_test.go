package kronk_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/install"
	"github.com/ardanlabs/kronk/model"
	"github.com/hybridgroup/yzma/pkg/download"
)

var (
	modelSimpleChatFile   = "models/qwen2.5-0.5b-instruct-q8_0.gguf"
	modelThinkChatFile    = "models/Qwen3-8B-Q8_0.gguf"
	modelGPTChatFile      = "models/gpt-oss-20b-Q8_0.gguf"
	modelSimpleVisionFile = "models/Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
	projSimpleVisionFile  = "models/mmproj-Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
	modelEmbedFile        = "models/embeddinggemma-300m-qat-Q8_0.gguf"
)

var (
	gw             = os.Getenv("GITHUB_WORKSPACE")
	libPath        = filepath.Join(gw, "tests/libraries")
	modelPath      = filepath.Join(gw, "tests/models")
	imageFile      = filepath.Join(gw, "images/samples", "giraffe.jpg")
	goroutines     = 1
	modelInstances = 1
	runInParallel  = false
)

func TestMain(m *testing.M) {
	installer()

	if err := kronk.Init(libPath, kronk.LogSilent); err != nil {
		fmt.Printf("Failed to init the llamacpp library: %s: error: %s\n", libPath, err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func installer() {
	if os.Getenv("MODEL_INSTANCES") != "" {
		var err error
		modelInstances, err = strconv.Atoi(os.Getenv("MODEL_INSTANCES"))
		if err != nil {
			modelInstances = 1
		}
	}

	if os.Getenv("GOROUTINES") != "" {
		var err error
		goroutines, err = strconv.Atoi(os.Getenv("GOROUTINES"))
		if err != nil {
			goroutines = 1
		}
	}

	if os.Getenv("RUN_IN_PARALLEL") == "1" {
		runInParallel = true
	}

	fmt.Println("libpath        :", libPath)
	fmt.Println("modelPath      :", modelPath)
	fmt.Println("imageFile      :", imageFile)
	fmt.Println("LD_LIBRARY_PATH:", os.Getenv("LD_LIBRARY_PATH"))
	fmt.Println("MODEL INSTANCES:", modelInstances)
	fmt.Println("GOROUTINES     :", goroutines)
	fmt.Println("INSTALL_LLAMA  :", os.Getenv("INSTALL_LLAMA"))
	fmt.Println("RUN_IN_PARALLEL:", runInParallel)

	if os.Getenv("INSTALL_LLAMA") == "1" {
		vi, err := install.VersionInformation(libPath)
		if err != nil {
			fmt.Printf("Failed to retrieve version info: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Latest version : %s, Current version: %s\n", vi.Latest, vi.Current)

		if vi.Current != vi.Latest {
			fmt.Printf("LIBRARIES      : Installing at %s\n", libPath)
			_, err := install.Llama(libPath, download.CPU, true)
			if err != nil {
				fmt.Printf("Failed to install llama: %s: error: %s\n", libPath, err)
				os.Exit(1)
			}

			if err := filepath.Walk(libPath, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				fmt.Println("lib:", path)
				return nil
			}); err != nil {
				fmt.Printf("error walking model path: %v\n", err)
			}
		}
	}

	fmt.Println("MODELS:")
	if err := filepath.Walk(modelPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		fmt.Println("Model:", path)
		return nil
	}); err != nil {
		fmt.Printf("error walking model path: %v\n", err)
	}
}

func testChatResponse(msg model.ChatResponse, modelFile string, object string, reasoning bool) error {
	if msg.ID == "" {
		return fmt.Errorf("expected id")
	}

	if msg.Object != object {
		return fmt.Errorf("expected object type to be %s, got %s", object, msg.Object)
	}

	if msg.Created == 0 {
		return fmt.Errorf("expected created time")
	}

	modelName := filepath.Base(modelFile)
	modelName = strings.TrimSuffix(modelName, filepath.Ext(modelName))

	if msg.Model != modelName {
		return fmt.Errorf("expected model to be %s, got %s", modelName, msg.Model)
	}

	if len(msg.Choice) == 0 {
		return fmt.Errorf("expected choice, got %d", len(msg.Choice))
	}

	if msg.Choice[0].FinishReason == "" && msg.Choice[0].Delta.Content == "" && msg.Choice[0].Delta.Reasoning == "" {
		return fmt.Errorf("expected delta content and reasoning empty")
	}

	if msg.Choice[0].FinishReason == "" && msg.Choice[0].Delta.Role != "assistant" {
		return fmt.Errorf("expected delta role to be assistant, got %s", msg.Choice[0].Delta.Role)
	}

	if msg.Choice[0].FinishReason == "stop" && msg.Choice[0].Delta.Content == "" {
		return fmt.Errorf("expected final content")
	}

	if reasoning {
		if msg.Choice[0].FinishReason == "stop" && msg.Choice[0].Delta.Reasoning == "" {
			return fmt.Errorf("expected final reasoning")
		}
	}

	return nil
}
