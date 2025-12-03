// Package website provides api.
package website

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/examples/install"
	"github.com/ardanlabs/kronk/model"
	"github.com/hybridgroup/yzma/pkg/download"
)

const (
	modelChatURL       = "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf"
	libPath            = "tests/libraries"
	modelPath          = "tests/models"
	modelInstances     = 1
	WebReadTimeout     = 10 * time.Second
	WebWriteTimeout    = 120 * time.Second
	WebIdleTimeout     = 120 * time.Second
	WebShutdownTimeout = 20 * time.Second
	WebAPIHost         = "0.0.0.0:8080"
)

func Run() error {
	if err := install.Libraries(libPath, download.CPU, true); err != nil {
		return fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	modelChatFile, err := install.Model(modelChatURL, modelPath)
	if err != nil {
		return fmt.Errorf("unable to install chat model: %w", err)
	}

	// -------------------------------------------------------------------------

	if err := kronk.Init(libPath, kronk.LogSilent); err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	krnChat, err := kronk.New(modelInstances, model.Config{
		ModelFile: modelChatFile,
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

	cfg := Config{
		KRNChat:    krnChat,
		KRNTimeout: WebWriteTimeout,
	}

	api := http.Server{
		Addr:         WebAPIHost,
		Handler:      WebAPI(cfg),
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
