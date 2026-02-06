package kronk_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	mpAudio         models.Path
	mpEmbed         models.Path
	mpRerank        models.Path
)

var (
	gw            = os.Getenv("GITHUB_WORKSPACE")
	imageFile     = filepath.Join(gw, "examples/samples/giraffe.jpg")
	audioFile     = filepath.Join(gw, "examples/samples/jfk.wav")
	goroutines    = 2
	runInParallel = false
	testDuration  = 60 * 5 * time.Second
)

func TestMain(m *testing.M) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		goroutines = 1
	}

	fmt.Println("Initializing models system...")
	models, err := models.New()
	if err != nil {
		fmt.Printf("creating models system: %s\n", err)
		os.Exit(1)
	}

	fmt.Println("MustRetrieveModel Qwen3-8B-Q8_0...")
	mpThinkToolChat = models.MustFullPath("Qwen3-8B-Q8_0")

	fmt.Println("MustRetrieveModel Qwen2.5-VL-3B-Instruct-Q8_0...")
	mpSimpleVision = models.MustFullPath("Qwen2.5-VL-3B-Instruct-Q8_0")

	fmt.Println("MustRetrieveModel embeddinggemma-300m-qat-Q8_0...")
	mpEmbed = models.MustFullPath("embeddinggemma-300m-qat-Q8_0")

	fmt.Println("MustRetrieveModel bge-reranker-v2-m3-Q8_0...")
	mpRerank = models.MustFullPath("bge-reranker-v2-m3-Q8_0")

	if os.Getenv("GITHUB_ACTIONS") != "true" {
		fmt.Println("MustRetrieveModel gpt-oss-20b-Q8_0...")
		mpGPTChat = models.MustFullPath("gpt-oss-20b-Q8_0")

		fmt.Println("MustRetrieveModel Qwen2-Audio-7B.Q8_0...")
		mpAudio = models.MustFullPath("Qwen2-Audio-7B.Q8_0")
	}

	// -------------------------------------------------------------------------

	if os.Getenv("RUN_IN_PARALLEL") == "yes" {
		runInParallel = true
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

	fmt.Println("Initializing test inputs...")
	if err := initChatTestInputs(); err != nil {
		fmt.Printf("Failed to init test inputs: %s\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func printInfo(models *models.Models) {
	fmt.Println("libpath          :", libs.Path(""))
	fmt.Println("useLibVersion    :", defaults.LibVersion(""))
	fmt.Println("modelPath        :", models.Path())
	fmt.Println("imageFile        :", imageFile)
	fmt.Println("processor        :", "cpu")
	fmt.Println("goroutines       :", goroutines)
	fmt.Println("testDuration     :", testDuration)
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
	if streaming && choice.FinishReason() == "" && choice.Delta != nil {
		return *choice.Delta
	}
	if choice.Message != nil {
		return *choice.Message
	}
	return model.ResponseMessage{}
}

// streamAccumulator collects content from streaming delta chunks.
type streamAccumulator struct {
	Content   strings.Builder
	Reasoning strings.Builder
	ToolCalls []model.ResponseToolCall
}

// accumulate adds delta content from a streaming response chunk.
func (sa *streamAccumulator) accumulate(resp model.ChatResponse) {
	if len(resp.Choice) == 0 {
		return
	}

	choice := resp.Choice[0]

	// Only accumulate from Delta when FinishReason is not set (intermediate chunks).
	if choice.FinishReason() == "" && choice.Delta != nil {
		sa.Content.WriteString(choice.Delta.Content)
		sa.Reasoning.WriteString(choice.Delta.Reasoning)
		if len(choice.Delta.ToolCalls) > 0 {
			sa.ToolCalls = append(sa.ToolCalls, choice.Delta.ToolCalls...)
		}
		return
	}

	// For final chunk, use Message which has accumulated content.
	if choice.Message != nil {
		sa.Content.WriteString(choice.Message.Content)
		sa.Reasoning.WriteString(choice.Message.Reasoning)
		if len(choice.Message.ToolCalls) > 0 {
			sa.ToolCalls = append(sa.ToolCalls, choice.Message.ToolCalls...)
		}
	}
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

	if resp.Choice[0].FinishReason() == "" && msg.Content == "" && msg.Reasoning == "" {
		return fmt.Errorf("basics: expected delta content and reasoning to be non-empty")
	}

	if resp.Choice[0].FinishReason() == "" && msg.Role != "assistant" {
		return fmt.Errorf("basics: expected delta role to be assistant, got %s", msg.Role)
	}

	if resp.Choice[0].FinishReason() == "stop" && msg.Content == "" {
		return fmt.Errorf("basics: expected final content to be non-empty")
	}

	if resp.Choice[0].FinishReason() == "tool_calls" && len(msg.ToolCalls) == 0 {
		return fmt.Errorf("basics: expected tool calls to be non-empty")
	}

	if resp.Choice[0].FinishReason() == "tool_calls" && streaming {
		if resp.Choice[0].Delta == nil || len(resp.Choice[0].Delta.ToolCalls) == 0 {
			return fmt.Errorf("basics: expected tool calls in Delta for streaming compatibility")
		}
	}

	if resp.Choice[0].FinishReason() == "tool_calls" && !streaming {
		if resp.Choice[0].Message == nil || len(resp.Choice[0].Message.ToolCalls) == 0 {
			return fmt.Errorf("basics: expected tool calls in Message for non-streaming")
		}
	}

	if reasoning {
		if resp.Choice[0].FinishReason() == "stop" && msg.Reasoning == "" {
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
	if err := testChatBasics(resp, modelName, object, object == model.ObjectChatText || object == model.ObjectChatTextFinal, streaming); err != nil {
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
	if object == model.ObjectChatText || object == model.ObjectChatTextFinal {
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

	if resp.Choice[0].FinishReason() == "stop" {
		if len(msg.Content) == 0 {
			result.Err = fmt.Errorf("content: expected some content")
		}

		if !strings.Contains(msg.Content, find) {
			result.Warnings = append(result.Warnings, fmt.Sprintf("content: expected %q, got %q", find, msg.Content))
			return result
		}
	}

	if resp.Choice[0].FinishReason() == "tool" {
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

// testStreamingContent validates accumulated streaming content.
// Use this for streaming tests instead of testChatResponse when the final
// chunk's Message.Content may not contain all accumulated content.
func testStreamingContent(acc *streamAccumulator, lastResp model.ChatResponse, find string) testResult {
	var result testResult

	content := strings.ToLower(acc.Content.String())
	reasoning := strings.ToLower(acc.Reasoning.String())
	find = strings.ToLower(find)

	// Check that we got some content or reasoning.
	if len(content) == 0 && len(reasoning) == 0 {
		result.Err = fmt.Errorf("streaming: expected some content or reasoning, got neither")
		return result
	}

	// Check for expected content.
	if !strings.Contains(content, find) && !strings.Contains(reasoning, find) {
		result.Warnings = append(result.Warnings, fmt.Sprintf("streaming: expected %q in content or reasoning, content=%q, reasoning=%q", find, content, reasoning))
	}

	return result
}

// testStreamingToolCall validates accumulated streaming tool call content.
func testStreamingToolCall(acc *streamAccumulator, lastResp model.ChatResponse, find string, funct string, arg string) testResult {
	var result testResult

	find = strings.ToLower(find)
	funct = strings.ToLower(funct)

	// For tool calls, we need to check the accumulated tool calls or the final response.
	var toolCalls []model.ResponseToolCall
	if len(acc.ToolCalls) > 0 {
		toolCalls = acc.ToolCalls
	} else if len(lastResp.Choice) > 0 && lastResp.Choice[0].Message != nil {
		toolCalls = lastResp.Choice[0].Message.ToolCalls
	}

	if len(toolCalls) == 0 {
		result.Err = fmt.Errorf("streaming: expected tool calls, got none")
		return result
	}

	funcName := strings.ToLower(toolCalls[0].Function.Name)
	if !strings.Contains(funcName, funct) {
		result.Warnings = append(result.Warnings, fmt.Sprintf("streaming: expected function %q, got %q", funct, funcName))
		return result
	}

	if len(toolCalls[0].Function.Arguments) == 0 {
		result.Err = fmt.Errorf("streaming: expected arguments to be non-empty")
		return result
	}

	location, exists := toolCalls[0].Function.Arguments[arg]
	if !exists {
		result.Err = fmt.Errorf("streaming: expected an argument named %s", arg)
		return result
	}

	if !strings.Contains(strings.ToLower(location.(string)), find) {
		result.Err = fmt.Errorf("streaming: expected %q in %s, got %q", find, arg, location.(string))
		return result
	}

	return result
}
