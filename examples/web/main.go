// This example shows you a web service that provides a chat endpoint for asking
// questions to a model with a browser based chat UI.
//
// The first time you run this program the system will download and install
// the model and libraries.
//
// Run the example like this from the root of the project:
// $ make example-web
//
// Run the website by navigation to his URL:
// http://localhost:8080
//
// Use curl to see the raw output:
// $ make example-web-curl1
// $ make example-web-curl2

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/defaults"
	"github.com/ardanlabs/kronk/examples/web/website"
	"github.com/ardanlabs/kronk/model"
	"github.com/ardanlabs/kronk/tools"
	"github.com/hybridgroup/yzma/pkg/download"
)

const (
	modelChatURL       = "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf"
	modelInstances     = 1
	WebReadTimeout     = 10 * time.Second
	WebWriteTimeout    = 120 * time.Second
	WebIdleTimeout     = 120 * time.Second
	WebShutdownTimeout = 20 * time.Second
	WebAPIHost         = "0.0.0.0:8080"
)

var (
	libPath   = defaults.LibsDir()
	modelPath = defaults.ModelsDir()
)

func main() {
	log.Default().SetOutput(os.Stdout)

	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	_, err := tools.DownloadLibraries(context.Background(), tools.FmtLogger, libPath, download.CPU, true)
	if err != nil {
		return fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	info, err := tools.DownloadModel(context.Background(), tools.FmtLogger, modelChatURL, "", modelPath)
	if err != nil {
		return fmt.Errorf("unable to install chat model: %w", err)
	}

	// -------------------------------------------------------------------------

	if err := kronk.Init(libPath, kronk.LogSilent); err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	krnChat, err := kronk.New(modelInstances, model.Config{
		ModelFile: info.ModelFile,
		NBatch:    32 * 1024,
	})
	if err != nil {
		return fmt.Errorf("unable to create chat model: %w", err)
	}
	defer func() {
		fmt.Println("\nUnloading Kronk")
		if err := krnChat.Unload(context.Background()); err != nil {
			fmt.Printf("failed to unload chat model: %v", err)
		}
	}()

	fmt.Print("- system info:\n\t")
	for k, v := range krnChat.SystemInfo() {
		fmt.Printf("%s:%v, ", k, v)
	}
	fmt.Println()

	fmt.Println("- contextWindow:", krnChat.ModelConfig().ContextWindow)
	fmt.Println("- embeddings   :", krnChat.ModelConfig().Embeddings)
	fmt.Println("- isGPT        :", krnChat.ModelInfo().IsGPT)

	// -------------------------------------------------------------------------

	fmt.Println()
	fmt.Println("startup: status: initializing V1 API support")

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	cfg := website.Config{
		KRNChat:    krnChat,
		KRNTimeout: WebWriteTimeout,
	}

	api := http.Server{
		Addr:         WebAPIHost,
		Handler:      website.WebAPI(cfg),
		ReadTimeout:  WebReadTimeout,
		WriteTimeout: WebWriteTimeout,
		IdleTimeout:  WebIdleTimeout,
	}

	serverErrors := make(chan error, 1)

	go func() {
		fmt.Println("startup: status: api router and website started: host", api.Addr)
		serverErrors <- api.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		return fmt.Errorf("server error: %w", err)

	case sig := <-shutdown:
		fmt.Println("shutdown: status: shutdown started: signal", sig)
		defer fmt.Println("shutdown: status: shutdown complete: signal", sig)

		ctx, cancel := context.WithTimeout(context.Background(), WebShutdownTimeout)
		defer cancel()

		if err := api.Shutdown(ctx); err != nil {
			api.Close()
			return fmt.Errorf("could not stop server gracefully: %w", err)
		}
	}

	return nil
}
