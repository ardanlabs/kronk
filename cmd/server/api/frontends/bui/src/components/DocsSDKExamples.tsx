import { useEffect, useRef } from 'react';
import Prism from 'prismjs';
import 'prismjs/components/prism-go';
import 'prismjs/themes/prism-tomorrow.css';

function GoCode({ children }: { children: string }) {
  const codeRef = useRef<HTMLElement>(null);

  useEffect(() => {
    if (codeRef.current) {
      Prism.highlightElement(codeRef.current);
    }
  }, [children]);

  return (
    <pre className="code-go">
      <code ref={codeRef} className="language-go">
        {children}
      </code>
    </pre>
  );
}

const questionExample = `// Run: make example-question

package main

import (
    "context"
    "fmt"
    "os"
    "time"

    "github.com/ardanlabs/kronk/sdk/kronk"
    "github.com/ardanlabs/kronk/sdk/kronk/model"
    "github.com/ardanlabs/kronk/sdk/tools/libs"
    "github.com/ardanlabs/kronk/sdk/tools/models"
)

const (
    modelURL       = "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf"
    modelInstances = 1
)

func main() {
    if err := run(); err != nil {
        fmt.Printf("\\nERROR: %s\\n", err)
        os.Exit(1)
    }
}

func run() error {
    info, _ := installSystem()

    kronk.Init()

    krn, err := kronk.New(modelInstances, model.Config{
        ModelFile: info.ModelFile,
    })
    if err != nil {
        return err
    }
    defer krn.Unload(context.Background())

    ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
    defer cancel()

    question := "Hello model"

    fmt.Println()
    fmt.Println("QUESTION:", question)
    fmt.Println()

    d := model.D{
        "messages": model.DocumentArray(
            model.TextMessage("user", question),
        ),
        "temperature": 0.7,
        "top_p":       0.9,
        "top_k":       40,
        "max_tokens":  2048,
    }

    ch, err := krn.ChatStreaming(ctx, d)
    if err != nil {
        return err
    }

    for resp := range ch {
        switch resp.Choice[0].FinishReason {
        case model.FinishReasonError:
            return fmt.Errorf("error: %s", resp.Choice[0].Delta.Content)
        case model.FinishReasonStop:
            return nil
        default:
            if resp.Choice[0].Delta.Reasoning != "" {
                fmt.Printf("\\033[91m%s\\033[0m", resp.Choice[0].Delta.Reasoning)
            } else {
                fmt.Print(resp.Choice[0].Delta.Content)
            }
        }
    }

    return nil
}`;

const chatExample = `// Run: make example-chat

package main

import (
    "bufio"
    "context"
    "fmt"
    "io"
    "os"
    "time"

    "github.com/ardanlabs/kronk/sdk/kronk"
    "github.com/ardanlabs/kronk/sdk/kronk/model"
    "github.com/ardanlabs/kronk/sdk/tools/libs"
    "github.com/ardanlabs/kronk/sdk/tools/models"
    "github.com/ardanlabs/kronk/sdk/tools/templates"
)

const (
    modelURL       = "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf"
    modelInstances = 1
)

func main() {
    if err := run(); err != nil {
        fmt.Printf("\\nERROR: %s\\n", err)
        os.Exit(1)
    }
}

func run() error {
    info, _ := installSystem()

    krn, err := kronk.New(modelInstances, model.Config{
        ModelFile: info.ModelFile,
    })
    if err != nil {
        return err
    }
    defer krn.Unload(context.Background())

    messages := model.DocumentArray()

    for {
        fmt.Print("\\nUSER> ")
        reader := bufio.NewReader(os.Stdin)
        userInput, err := reader.ReadString('\\n')
        if err != nil || userInput == "quit\\n" {
            return nil
        }

        messages = append(messages, model.TextMessage("user", userInput))

        ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)

        d := model.D{
            "messages":    messages,
            "tools":       toolDocuments(),
            "max_tokens":  2048,
            "temperature": 0.7,
        }

        ch, _ := krn.ChatStreaming(ctx, d)

        fmt.Print("\\nMODEL> ")
        for resp := range ch {
            switch resp.Choice[0].FinishReason {
            case model.FinishReasonStop:
                messages = append(messages, model.TextMessage("assistant", resp.Choice[0].Delta.Content))
            case model.FinishReasonTool:
                for _, tool := range resp.Choice[0].Delta.ToolCalls {
                    fmt.Printf("Tool: %s(%s)\\n", tool.Name, tool.Arguments)
                }
            default:
                fmt.Print(resp.Choice[0].Delta.Content)
            }
        }
        cancel()
    }
}

func toolDocuments() []model.D {
    return model.DocumentArray(
        model.D{
            "type": "function",
            "function": model.D{
                "name":        "get_weather",
                "description": "Get the current weather for a location",
                "parameters": model.D{
                    "type": "object",
                    "properties": model.D{
                        "location": model.D{
                            "type":        "string",
                            "description": "The location to get the weather for",
                        },
                    },
                    "required": []any{"location"},
                },
            },
        },
    )
}`;

const embeddingExample = `// Run: make example-embedding

package main

import (
    "context"
    "fmt"
    "os"
    "time"

    "github.com/ardanlabs/kronk/sdk/kronk"
    "github.com/ardanlabs/kronk/sdk/kronk/model"
    "github.com/ardanlabs/kronk/sdk/tools/libs"
    "github.com/ardanlabs/kronk/sdk/tools/models"
)

const (
    modelURL       = "https://huggingface.co/ggml-org/embeddinggemma-300m-qat-q8_0-GGUF/resolve/main/embeddinggemma-300m-qat-Q8_0.gguf"
    modelInstances = 1
)

func main() {
    if err := run(); err != nil {
        fmt.Printf("\\nERROR: %s\\n", err)
        os.Exit(1)
    }
}

func run() error {
    info, _ := installSystem()

    kronk.Init()

    krn, err := kronk.New(modelInstances, model.Config{
        ModelFile: info.ModelFile,
    })
    if err != nil {
        return err
    }
    defer krn.Unload(context.Background())

    ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
    defer cancel()

    question := "Why is the sky blue?"

    resp, err := krn.Embeddings(ctx, question)
    if err != nil {
        return err
    }

    fmt.Println()
    fmt.Println("Model  :", resp.Model)
    fmt.Println("Object :", resp.Object)
    fmt.Println("Created:", time.UnixMilli(resp.Created))
    fmt.Println("  Index    :", resp.Data[0].Index)
    fmt.Println("  Object   :", resp.Data[0].Object)
    fmt.Printf("  Embedding: [%v...%v]\\n", 
        resp.Data[0].Embedding[:3], 
        resp.Data[0].Embedding[len(resp.Data[0].Embedding)-3:])

    return nil
}`;

const audioExample = `// Run: make example-audio

package main

import (
    "context"
    "fmt"
    "os"
    "time"

    "github.com/ardanlabs/kronk/sdk/kronk"
    "github.com/ardanlabs/kronk/sdk/kronk/model"
    "github.com/ardanlabs/kronk/sdk/tools/libs"
    "github.com/ardanlabs/kronk/sdk/tools/models"
)

const (
    modelURL       = "https://huggingface.co/mradermacher/Qwen2-Audio-7B-GGUF/resolve/main/Qwen2-Audio-7B.Q8_0.gguf"
    projURL        = "https://huggingface.co/mradermacher/Qwen2-Audio-7B-GGUF/resolve/main/Qwen2-Audio-7B.mmproj-Q8_0.gguf"
    audioFile      = "examples/samples/jfk.wav"
    modelInstances = 1
)

func main() {
    if err := run(); err != nil {
        fmt.Printf("\\nERROR: %s\\n", err)
        os.Exit(1)
    }
}

func run() error {
    info, _ := installSystem()

    krn, err := newKronk(info)
    if err != nil {
        return fmt.Errorf("unable to init kronk: %w", err)
    }
    defer krn.Unload(context.Background())

    question := "Please describe what you hear in the following audio clip."

    ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
    defer cancel()

    audio, _ := os.ReadFile(audioFile)

    d := model.D{
        "messages":    model.MediaMessage(question, audio),
        "max_tokens":  2048,
        "temperature": 0.7,
    }

    ch, err := krn.ChatStreaming(ctx, d)
    if err != nil {
        return err
    }

    fmt.Print("\\nMODEL> ")
    for resp := range ch {
        switch resp.Choice[0].FinishReason {
        case model.FinishReasonStop:
            return nil
        case model.FinishReasonError:
            return fmt.Errorf("error: %s", resp.Choice[0].Delta.Content)
        default:
            fmt.Print(resp.Choice[0].Delta.Content)
        }
    }

    return nil
}`;

const visionExample = `// Run: make example-vision

package main

import (
    "context"
    "fmt"
    "os"
    "time"

    "github.com/ardanlabs/kronk/sdk/kronk"
    "github.com/ardanlabs/kronk/sdk/kronk/model"
    "github.com/ardanlabs/kronk/sdk/tools/libs"
    "github.com/ardanlabs/kronk/sdk/tools/models"
)

const (
    modelURL       = "https://huggingface.co/ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/resolve/main/Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
    projURL        = "https://huggingface.co/ggml-org/Qwen2.5-VL-3B-Instruct-GGUF/resolve/main/mmproj-Qwen2.5-VL-3B-Instruct-Q8_0.gguf"
    imageFile      = "examples/samples/giraffe.jpg"
    modelInstances = 1
)

func main() {
    if err := run(); err != nil {
        fmt.Printf("\\nERROR: %s\\n", err)
        os.Exit(1)
    }
}

func run() error {
    info, _ := installSystem()

    kronk.Init()

    krn, err := kronk.New(modelInstances, model.Config{
        ModelFile: info.ModelFile,
        ProjFile:  info.ProjFile,
    })
    if err != nil {
        return err
    }
    defer krn.Unload(context.Background())

    question := "What is in this picture?"

    ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
    defer cancel()

    image, _ := os.ReadFile(imageFile)

    fmt.Printf("\\nQuestion: %s\\n", question)

    d := model.D{
        "messages":    model.MediaMessage(question, image),
        "temperature": 0.7,
        "top_p":       0.9,
        "top_k":       40,
        "max_tokens":  2048,
    }

    ch, err := krn.ChatStreaming(ctx, d)
    if err != nil {
        return err
    }

    fmt.Print("\\nMODEL> ")
    for resp := range ch {
        switch resp.Choice[0].FinishReason {
        case model.FinishReasonStop:
            return nil
        case model.FinishReasonError:
            return fmt.Errorf("error: %s", resp.Choice[0].Delta.Content)
        default:
            if resp.Choice[0].Delta.Reasoning != "" {
                fmt.Printf("\\033[91m%s\\033[0m", resp.Choice[0].Delta.Reasoning)
            } else {
                fmt.Print(resp.Choice[0].Delta.Content)
            }
        }
    }

    return nil
}`;

export default function DocsSDKExamples() {
  return (
    <div>
      <div className="page-header">
        <h2>SDK Examples</h2>
        <p>Complete working examples demonstrating how to use the Kronk SDK</p>
      </div>

      <div className="card">
        <h3>Question</h3>
        <p className="doc-description">
          Basic program demonstrating how to ask a model a question with streaming response. The simplest way to get started.
        </p>
        <GoCode>{questionExample}</GoCode>
      </div>

      <div className="card">
        <h3>Chat</h3>
        <p className="doc-description">
          Create a simple chat application with tool calling support. Demonstrates multi-turn conversation and function calling.
        </p>
        <GoCode>{chatExample}</GoCode>
      </div>

      <div className="card">
        <h3>Embedding</h3>
        <p className="doc-description">
          Generate embeddings using an embedding model. Useful for semantic search and similarity comparisons.
        </p>
        <GoCode>{embeddingExample}</GoCode>
      </div>

      <div className="card">
        <h3>Audio</h3>
        <p className="doc-description">
          Execute a prompt against an audio model. Uses Qwen2-Audio-7B for audio understanding.
        </p>
        <GoCode>{audioExample}</GoCode>
      </div>

      <div className="card">
        <h3>Vision</h3>
        <p className="doc-description">
          Execute a prompt against a vision model to analyze images. Uses Qwen2.5-VL for image understanding.
        </p>
        <GoCode>{visionExample}</GoCode>
      </div>
    </div>
  );
}
