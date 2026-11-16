// This example verifies disk-backed IMC across two separate program runs.
// Save writes a conversation, its final assistant text, and an input checkpoint.
// Restore reads the messages, appends the saved assistant text and a new user
// message, and lets normal IMC discovery import the checkpoint into a fresh model.
// This is application-managed text replay, not a manual native-state restore API.
//
// Install libraries and a model first (from the repository root):
//
//	kronk libs --local
//	kronk model pull unsloth/Qwen3-0.6B-Q8_0 --local
//
// Run with the same model, directory, configuration, and library version:
//
//	make example-imc-storage ARGS='-phase save -model /path/to/model.gguf -directory /tmp/imc-demo'
//	make example-imc-storage ARGS='-phase restore -model /path/to/model.gguf -directory /tmp/imc-demo'
//
// Use a new directory for the save phase. Restore exits with an error if no
// checkpoint tokens were reused or the saved conversation ID was not preserved.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/imc"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/storage/disk"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func run() (retErr error) {
	phase := flag.String("phase", "save", "save or restore (run in separate processes)")
	modelPath := flag.String("model", "", "path to an installed text GGUF model")
	directory := flag.String("directory", "imc-sessions", "private IMC storage directory")
	libPath := flag.String("lib-path", "", "optional llama.cpp library directory")
	flag.Parse()

	if *modelPath == "" || (*phase != "save" && *phase != "restore") {
		return errors.New("provide -model and -phase save|restore")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	store, err := disk.New(*directory)
	if err != nil {
		return err
	}

	defer func() { retErr = errors.Join(retErr, store.Close()) }()

	page, err := store.List(ctx, imc.ListQuery{Limit: 1})
	if err != nil {
		return err
	}

	request := model.D{
		"messages": []model.D{
			{"role": "system", "content": "You are a concise assistant. Follow the user's instructions and answer in one short sentence."},
			{"role": "user", "content": "Remember this project code: 713. We are building a disk backed conversation cache."},
			{"role": "assistant", "content": "I will remember project code 713."},
			{"role": "user", "content": "What is our project code? Reply with only the number."},
		},
		"max_tokens": 64, "temperature": 0.0, "enable_thinking": false,
	}

	var previous imc.Summary

	switch *phase {
	case "save":
		if len(page.Sessions) != 0 {
			return errors.New("save phase needs a new directory; existing history is not overwritten by this example")
		}

	case "restore":
		if len(page.Sessions) == 0 {
			return errors.New("run the save phase first")
		}

		previous = page.Sessions[0]

		handle, err := store.Open(ctx, previous.Ref)
		if err != nil {
			return err
		}

		record := handle.Record()

		if err := handle.Close(); err != nil {
			return err
		}

		if err := json.Unmarshal(record.Request, &request); err != nil {
			return err
		}

		var conversation struct {
			Messages []model.D `json:"messages"`
		}
		if err := json.Unmarshal(record.Request, &conversation); err != nil {
			return err
		}

		if record.AssistantText == "" {
			return errors.New("saved response has no final text to replay")
		}

		request["messages"] = append(conversation.Messages,
			model.D{"role": "assistant", "content": record.AssistantText},
			model.D{"role": "user", "content": "Repeat our project code. Reply with only the number."})
	}

	if err := kronk.Init(kronk.WithLibPath(*libPath)); err != nil {
		return err
	}

	krn, err := kronk.NewWithContext(ctx,
		model.WithModelFiles([]string{*modelPath}),
		model.WithContextWindow(2048), model.WithNSeqMax(1),
		model.WithIncrementalCache(true), model.WithCacheMinTokens(1),
		model.WithStorage(model.StorageConfig{Store: store}),
		model.WithLog(kronk.FmtLogger),
	)
	if err != nil {
		return err
	}

	defer func() { retErr = errors.Join(retErr, krn.Unload(context.Background())) }()

	response, err := krn.Chat(ctx, request)
	if err != nil {
		return err
	}

	if len(response.Choices) == 0 || response.Choices[0].Message == nil || response.Choices[0].Message.Content == "" || response.Usage == nil {
		return errors.New("model returned no final text or usage")
	}

	fmt.Printf("Assistant: %s\nCached prompt tokens: %d\n", response.Choices[0].Message.Content, response.Usage.PromptTokensDetails.CachedTokens)

	page, err = store.List(ctx, imc.ListQuery{Limit: 2})
	if err != nil {
		return err
	}

	if len(page.Sessions) != 1 {
		return fmt.Errorf("expected one saved conversation, got %d", len(page.Sessions))
	}

	current := page.Sessions[0]
	if *phase == "restore" && (response.Usage.PromptTokensDetails.CachedTokens == 0 || current.Ref.Key != previous.Ref.Key || current.Ref.Revision <= previous.Ref.Revision) {
		return errors.New("disk checkpoint was not reused and advanced under the same conversation ID")
	}

	handle, err := store.Open(ctx, current.Ref)
	if err != nil {
		return err
	}

	record := handle.Record()
	target, draft, media := handle.Sizes()
	if err := handle.Close(); err != nil {
		return err
	}

	if record.AssistantText != response.Choices[0].Message.Content {
		return errors.New("stored assistant text differs from the delivered response")
	}

	fmt.Printf("Saved conversation: %s\nRevision: %d\nUncompressed target/draft: %d/%d bytes; media items: %d\n", current.Ref.Key.ID, current.Ref.Revision, target, draft, len(media))
	fmt.Printf("PASS: %s phase\n", *phase)

	return nil
}
