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

	"github.com/ardanlabs/kronk/examples/web/website"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
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

func main() {
	log.Default().SetOutput(os.Stdout)

	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	libs, err := libs.New()
	if err != nil {
		return err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	// -------------------------------------------------------------------------

	modelTool, err := models.New()
	if err != nil {
		return fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	mp, err := modelTool.Download(ctx, kronk.FmtLogger, modelChatURL, "")
	if err != nil {
		return fmt.Errorf("unable to install model: %w", err)
	}

	// -------------------------------------------------------------------------

	catalog, err := catalog.New()
	if err != nil {
		return fmt.Errorf("unable to create catalog system: %w", err)
	}

	if err := catalog.Download(ctx); err != nil {
		return fmt.Errorf("unable to download catalog: %w", err)
	}

	// -------------------------------------------------------------------------

	templates, err := templates.New()
	if err != nil {
		return fmt.Errorf("unable to create template system: %w", err)
	}

	if err := templates.Download(ctx); err != nil {
		return fmt.Errorf("unable to download templates: %w", err)
	}

	// -------------------------------------------------------------------------

	if err := kronk.Init(); err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	krnChat, err := kronk.New(modelInstances, model.Config{
		Log:       kronk.FmtLogger,
		ModelFile: mp.ModelFile,
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
	fmt.Println("- embeddings   :", krnChat.ModelInfo().IsEmbedModel)
	fmt.Println("- isGPT        :", krnChat.ModelInfo().IsGPTModel)

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
