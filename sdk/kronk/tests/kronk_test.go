package kronk_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/defaults"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
)

var (
	mpThinkToolChat models.Path
	mpGPTChat       models.Path
	mpSimpleVision  models.Path
	mpEmbed         models.Path
)

func init() {
	models, err := models.New()
	if err != nil {
		fmt.Printf("creating models system: %s\n", err)
		os.Exit(1)
	}

	mpThinkToolChat = models.MustRetrieveModel("Qwen3-8B-Q8_0")
	mpSimpleVision = models.MustRetrieveModel("Qwen2.5-VL-3B-Instruct-Q8_0")
	mpEmbed = models.MustRetrieveModel("embeddinggemma-300m-qat-Q8_0")

	if os.Getenv("GITHUB_ACTIONS") != "true" {
		mpGPTChat = models.MustRetrieveModel("gpt-oss-20b-Q8_0")
	}
}

var (
	gw             = os.Getenv("GITHUB_WORKSPACE")
	imageFile      = filepath.Join(gw, "examples/samples/giraffe.jpg")
	goroutines     = 1
	modelInstances = 1
	runInParallel  = false
	testDuration   = 60 * 5 * time.Second
)

func TestMain(m *testing.M) {
	printInfo()

	ctx := context.Background()

	catalog, err := catalog.New()
	if err != nil {
		fmt.Printf("unable to create template system: %s", err)
		os.Exit(1)
	}

	if err := catalog.Download(ctx); err != nil {
		fmt.Printf("unable to download catalog: %s", err)
		os.Exit(1)
	}

	templates, err := templates.New()
	if err != nil {
		fmt.Printf("unable to create template system: %s", err)
		os.Exit(1)
	}

	if err := templates.Download(ctx); err != nil {
		fmt.Printf("unable to download templates: %s", err)
		os.Exit(1)
	}

	if err := kronk.Init(); err != nil {
		fmt.Printf("Failed to init the llama.cpp library: error: %s\n", err)
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

	fmt.Println("libpath        :", defaults.LibsDir(""))
	fmt.Println("modelPath      :", defaults.ModelsDir(""))
	fmt.Println("imageFile      :", imageFile)
	fmt.Println("processor      :", "cpu")
	fmt.Println("testDuration   :", testDuration)
	fmt.Println("MODEL INSTANCES:", modelInstances)
	fmt.Println("GOROUTINES     :", goroutines)
	fmt.Println("RUN_IN_PARALLEL:", runInParallel)

	libs, err := libs.New()
	if err != nil {
		fmt.Printf("Failed to construct the libs api: %v\n", err)
		os.Exit(1)
	}

	currentVersion, err := libs.InstalledVersion()
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
	if len(resp.Choice[0].Delta.ToolCalls) > 0 {
		resp.Choice[0].Delta.ToolCalls[0].Name = strings.ToLower(resp.Choice[0].Delta.ToolCalls[0].Name)
	}

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
