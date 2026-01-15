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
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
)

func init() {
	fmt.Println("kronk_test init starting...")
}

var (
	mpThinkToolChat models.Path
	mpGPTChat       models.Path
	mpSimpleVision  models.Path
	mpEmbed         models.Path
)

var (
	gw             = os.Getenv("GITHUB_WORKSPACE")
	imageFile      = filepath.Join(gw, "examples/samples/giraffe.jpg")
	goroutines     = 1
	modelInstances = 1
	runInParallel  = false
	testDuration   = 60 * 5 * time.Second
)

func TestMain(m *testing.M) {
	fmt.Println("Initializing models system...")
	models, err := models.New()
	if err != nil {
		fmt.Printf("creating models system: %s\n", err)
		os.Exit(1)
	}

	fmt.Println("MustRetrieveModel Qwen3-8B-Q8_0...")
	mpThinkToolChat = models.MustRetrieveModel("Qwen3-8B-Q8_0")

	fmt.Println("MustRetrieveModel Qwen2.5-VL-3B-Instruct-Q8_0...")
	mpSimpleVision = models.MustRetrieveModel("Qwen2.5-VL-3B-Instruct-Q8_0")

	fmt.Println("MustRetrieveModel embeddinggemma-300m-qat-Q8_0...")
	mpEmbed = models.MustRetrieveModel("embeddinggemma-300m-qat-Q8_0")

	if os.Getenv("GITHUB_ACTIONS") != "true" {
		mpGPTChat = models.MustRetrieveModel("gpt-oss-20b-Q8_0")
	}

	// -------------------------------------------------------------------------

	printInfo(models)

	ctx := context.Background()

	templates, err := templates.New()
	if err != nil {
		fmt.Printf("unable to create template system: %s", err)
		os.Exit(1)
	}

	fmt.Println("Downloading Templates...")
	if err := templates.Download(ctx); err != nil {
		fmt.Printf("unable to download templates: %s", err)
		os.Exit(1)
	}

	fmt.Println("Downloading Catalog...")
	if err := templates.Catalog().Download(ctx); err != nil {
		fmt.Printf("unable to download catalog: %s", err)
		os.Exit(1)
	}

	fmt.Println("Init Kronk...")
	if err := kronk.Init(); err != nil {
		fmt.Printf("Failed to init the llama.cpp library: error: %s\n", err)
		os.Exit(1)
	}

	fmt.Println("Loading chat models...")
	if err := initChatModels(); err != nil {
		fmt.Printf("Failed to init chat models: %s\n", err)
		os.Exit(1)
	}

	fmt.Println("Loading media models...")
	if err := initMediaModels(); err != nil {
		fmt.Printf("Failed to init media models: %s\n", err)
		os.Exit(1)
	}

	fmt.Println("Initializing response inputs...")
	initResponseInputs()

	code := m.Run()

	fmt.Println("Unloading media models...")
	unloadMediaModels()

	fmt.Println("Unloading chat models...")
	unloadChatModels()

	os.Exit(code)
}

func printInfo(models *models.Models) {
	if os.Getenv("GOROUTINES") != "" {
		var err error
		goroutines, err = strconv.Atoi(os.Getenv("GOROUTINES"))
		if err != nil {
			goroutines = 1
		}
	}

	if os.Getenv("RUN_IN_PARALLEL") == "yes" {
		runInParallel = true
	}

	fmt.Println("libpath          :", libs.Path(""))
	fmt.Println("useLibVersion    :", defaults.LibVersion(""))
	fmt.Println("modelPath        :", models.Path())
	fmt.Println("imageFile        :", imageFile)
	fmt.Println("processor        :", "cpu")
	fmt.Println("testDuration     :", testDuration)
	fmt.Println("MODEL INSTANCES. :", modelInstances)
	fmt.Println("GOROUTINES       :", goroutines)
	fmt.Println("RUN_IN_PARALLEL  :", runInParallel)

	libs, err := libs.New(libs.WithVersion(defaults.LibVersion("")))
	if err != nil {
		fmt.Printf("Failed to construct the libs api: %v\n", err)
		os.Exit(1)
	}

	currentVersion, err := libs.InstalledVersion()
	if err != nil {
		fmt.Printf("Failed to retrieve version info: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Installed version: %s\n", currentVersion)
}

func getMsg(choice model.Choice, streaming bool) model.ResponseMessage {
	if streaming && choice.FinishReason == "" {
		return choice.Delta
	}
	return choice.Message
}

func testChatBasics(resp model.ChatResponse, modelName string, object string, reasoning bool, streaming bool) error {
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

	msg := getMsg(resp.Choice[0], streaming)

	if resp.Choice[0].FinishReason == "" && msg.Content == "" && msg.Reasoning == "" {
		return fmt.Errorf("basics: expected delta content and reasoning to be non-empty")
	}

	if resp.Choice[0].FinishReason == "" && msg.Role != "assistant" {
		return fmt.Errorf("basics: expected delta role to be assistant, got %s", msg.Role)
	}

	if resp.Choice[0].FinishReason == "stop" && msg.Content == "" {
		return fmt.Errorf("basics: expected final content to be non-empty")
	}

	if resp.Choice[0].FinishReason == "tool" && len(msg.ToolCalls) == 0 {
		return fmt.Errorf("basics: expected tool calls to be non-empty")
	}

	if streaming && resp.Choice[0].FinishReason == "tool" && len(resp.Choice[0].Delta.ToolCalls) == 0 {
		return fmt.Errorf("basics: expected tool calls in Delta for OpenAI streaming compatibility")
	}

	if reasoning {
		if resp.Choice[0].FinishReason == "stop" && msg.Reasoning == "" {
			return fmt.Errorf("basics: expected final reasoning")
		}
	}

	return nil
}

type testResult struct {
	Err      error
	Warnings []string
}

func testChatResponse(resp model.ChatResponse, modelName string, object string, find string, funct string, arg string, streaming bool) testResult {
	if err := testChatBasics(resp, modelName, object, object == model.ObjectChatText, streaming); err != nil {
		return testResult{Err: err}
	}

	var result testResult

	msg := getMsg(resp.Choice[0], streaming)

	find = strings.ToLower(find)
	funct = strings.ToLower(funct)
	msg.Reasoning = strings.ToLower(msg.Reasoning)
	msg.Content = strings.ToLower(msg.Content)

	if len(msg.ToolCalls) > 0 {
		msg.ToolCalls[0].Function.Name = strings.ToLower(msg.ToolCalls[0].Function.Name)
	}

	// Reasoning checks are warnings (LLM output is non-deterministic).
	if object == model.ObjectChatText {
		if len(msg.Reasoning) == 0 {
			result.Err = fmt.Errorf("content: expected some reasoning")
		}

		switch {
		case funct == "":
			if !strings.Contains(msg.Reasoning, find) {
				result.Warnings = append(result.Warnings, fmt.Sprintf("reasoning: expected %q, got %q", find, msg.Reasoning))
			}

		case funct != "":
			if !strings.Contains(msg.Reasoning, funct) {
				result.Warnings = append(result.Warnings, fmt.Sprintf("reasoning: expected %q, got %q", funct, msg.Reasoning))
			}
		}
	}

	if resp.Choice[0].FinishReason == "stop" {
		if len(msg.Content) == 0 {
			result.Err = fmt.Errorf("content: expected some content")
		}

		if !strings.Contains(msg.Content, find) {
			result.Warnings = append(result.Warnings, fmt.Sprintf("content: expected %q, got %q", find, msg.Content))
			return result
		}
	}

	if resp.Choice[0].FinishReason == "tool" {
		if !strings.Contains(msg.ToolCalls[0].Function.Name, funct) {
			result.Warnings = append(result.Warnings, fmt.Sprintf("tooling: expected %q, got %q", funct, msg.ToolCalls[0].Function.Name))
			return result
		}

		if len(msg.ToolCalls[0].Function.Arguments) == 0 {
			result.Err = fmt.Errorf("tooling: expected arguments to be non-empty, got %v", msg.ToolCalls[0].Function.Arguments)
			return result
		}

		location, exists := msg.ToolCalls[0].Function.Arguments[arg]
		if !exists {
			result.Err = fmt.Errorf("tooling: expected an argument named %s", arg)
			return result
		}

		if !strings.Contains(strings.ToLower(location.(string)), find) {
			result.Err = fmt.Errorf("tooling: expected %q, got %q", find, location.(string))
			return result
		}
	}

	return result
}
