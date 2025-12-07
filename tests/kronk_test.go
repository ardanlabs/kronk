package kronk_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/defaults"
	"github.com/ardanlabs/kronk/model"
	"github.com/ardanlabs/kronk/tools"
)

var (
	mpThinkToolChat tools.ModelPath
	mpGPTChat       tools.ModelPath
	mpSimpleVision  tools.ModelPath
	mpEmbed         tools.ModelPath
)

func init() {
	mpThinkToolChat = tools.MustFindModel(defaults.ModelsDir(""), "Qwen3-8B-Q8_0.gguf")
	mpSimpleVision = tools.MustFindModel(defaults.ModelsDir(""), "Qwen2.5-VL-3B-Instruct-Q8_0.gguf")
	mpEmbed = tools.MustFindModel(defaults.ModelsDir(""), "embeddinggemma-300m-qat-Q8_0.gguf")

	if os.Getenv("GITHUB_ACTIONS") != "true" {
		mpGPTChat = tools.MustFindModel(defaults.ModelsDir(""), "gpt-oss-20b-Q8_0.gguf")
	}
}

var (
	gw             = os.Getenv("GITHUB_WORKSPACE")
	libPath        = defaults.LibsDir("")
	modelPath      = defaults.ModelsDir("")
	imageFile      = filepath.Join(gw, "examples/samples/giraffe.jpg")
	goroutines     = 1
	modelInstances = 1
	runInParallel  = false
	testDuration   = 60 * 5 * time.Second
)

func TestMain(m *testing.M) {
	printInfo()

	err := kronk.Init(libPath, kronk.LogSilent)
	if err != nil {
		fmt.Printf("Failed to init the llama.cpp library: %s: error: %s\n", libPath, err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func printInfo() {
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
	fmt.Println("processor      :", "cpu")
	fmt.Println("testDuration   :", testDuration)
	fmt.Println("MODEL INSTANCES:", modelInstances)
	fmt.Println("GOROUTINES     :", goroutines)
	fmt.Println("RUN_IN_PARALLEL:", runInParallel)

	currentVersion, err := tools.InstalledVersion(libPath)
	if err != nil {
		fmt.Printf("Failed to retrieve version info: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Current version: %s\n", currentVersion)
}

func testChatBasics(resp model.ChatResponse, modelName string, object string, reasoning bool) error {
	if resp.ID == "" {
		return fmt.Errorf("expected id")
	}

	if resp.Object != object {
		return fmt.Errorf("expected object type to be %s, got %s", object, resp.Object)
	}

	if resp.Created == 0 {
		return fmt.Errorf("expected created time")
	}

	if resp.Model != modelName {
		return fmt.Errorf("basics: expected model to be %s, got %s", modelName, resp.Model)
	}

	if len(resp.Choice) == 0 {
		return fmt.Errorf("basics: expected choice, got %d", len(resp.Choice))
	}

	if resp.Choice[0].FinishReason == "" && resp.Choice[0].Delta.Content == "" && resp.Choice[0].Delta.Reasoning == "" {
		return fmt.Errorf("basics: expected delta content and reasoning to be non-empty")
	}

	if resp.Choice[0].FinishReason == "" && resp.Choice[0].Delta.Role != "assistant" {
		return fmt.Errorf("basics: expected delta role to be assistant, got %s", resp.Choice[0].Delta.Role)
	}

	if resp.Choice[0].FinishReason == "stop" && resp.Choice[0].Delta.Content == "" {
		return fmt.Errorf("basics: expected final content to be non-empty")
	}

	if resp.Choice[0].FinishReason == "tool" && len(resp.Choice[0].Delta.ToolCalls) == 0 {
		return fmt.Errorf("basics: expected tool calls to be non-empty")
	}

	if reasoning {
		if resp.Choice[0].FinishReason == "stop" && resp.Choice[0].Delta.Reasoning == "" {
			return fmt.Errorf("basics: expected final reasoning")
		}
	}

	return nil
}

func testChatResponse(resp model.ChatResponse, modelName string, object string, find string, funct string, arg string) error {
	if err := testChatBasics(resp, modelName, object, object == model.ObjectChatText); err != nil {
		return err
	}

	find = strings.ToLower(find)
	funct = strings.ToLower(funct)
	resp.Choice[0].Delta.Reasoning = strings.ToLower(resp.Choice[0].Delta.Reasoning)
	resp.Choice[0].Delta.Content = strings.ToLower(resp.Choice[0].Delta.Content)
	resp.Choice[0].Delta.ToolCalls[0].Name = strings.ToLower(resp.Choice[0].Delta.ToolCalls[0].Name)

	if object == model.ObjectChatText {
		switch {
		case funct == "":
			if !strings.Contains(resp.Choice[0].Delta.Reasoning, find) {
				return fmt.Errorf("reasoning: expected %q, got %q", find, resp.Choice[0].Delta.Reasoning)
			}

		case funct != "":
			if !strings.Contains(resp.Choice[0].Delta.Reasoning, funct) {
				return fmt.Errorf("reasoning: expected %q, got %q", funct, resp.Choice[0].Delta.Reasoning)
			}
		}
	}

	if resp.Choice[0].FinishReason == "stop" {
		if !strings.Contains(resp.Choice[0].Delta.Content, find) {
			return fmt.Errorf("content: expected %q, got %q", find, resp.Choice[0].Delta.Content)
		}
	}

	if resp.Choice[0].FinishReason == "tool" {
		if !strings.Contains(resp.Choice[0].Delta.ToolCalls[0].Name, funct) {
			return fmt.Errorf("tooling: expected %q, got %q", funct, resp.Choice[0].Delta.ToolCalls[0].Name)
		}

		if len(resp.Choice[0].Delta.ToolCalls[0].Arguments) == 0 {
			return fmt.Errorf("tooling: expected arguments to be non-empty, got %v", resp.Choice[0].Delta.ToolCalls[0].Arguments)
		}

		location, exists := resp.Choice[0].Delta.ToolCalls[0].Arguments[arg]
		if !exists {
			return fmt.Errorf("tooling: expected an argument named %s", arg)
		}

		if !strings.Contains(strings.ToLower(location.(string)), find) {
			return fmt.Errorf("tooling: expected %q, got %q", find, location.(string))
		}
	}

	return nil
}
