// This example supplies an IMC store, saves a conversation, and continues it
// after a process restart. Replace the disk store with your own imc.Store.
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
	phase := flag.String("phase", "save", "save a new conversation or restore the latest saved conversation")
	modelPath := flag.String("model", "", "path to an installed text GGUF model")
	directory := flag.String("directory", "imc-sessions", "private storage directory for this example")
	libPath := flag.String("lib-path", "", "optional llama.cpp library directory")
	flag.Parse()

	if *modelPath == "" || (*phase != "save" && *phase != "restore") {
		return errors.New("provide -model and -phase save|restore")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Replace this constructor with your own imc.Store implementation.
	// The application owns storage: close it only after unloading the model.
	// WithLog is optional and belongs to the disk backend, not imc.Store.
	store, err := disk.New(*directory, disk.WithLog(kronk.FmtLogger))
	if err != nil {
		return err
	}

	defer func() {
		retErr = errors.Join(retErr, store.Close())
	}()

	page, err := store.List(ctx, imc.ListQuery{Limit: 1})
	if err != nil {
		return err
	}

	request := model.D{
		"messages": []model.D{
			{"role": "system", "content": "You are a concise assistant. Follow the user's instructions exactly."},
			{"role": "user", "content": "Remember this project code: 713. We are building a disk backed conversation cache."},
			{"role": "assistant", "content": "I will remember project code 713."},
			{"role": "user", "content": "What is our project code? Reply with only the number."},
		},
		"max_tokens": 128, "temperature": 0.0, "enable_thinking": false,
	}

	switch *phase {
	case "save":
		if len(page.Sessions) != 0 {
			return errors.New("save needs an empty storage directory; use restore to continue existing history")
		}

	case "restore":
		if len(page.Sessions) == 0 {
			return errors.New("no saved conversation; run the save phase first")
		}

		request, err = continueConversation(ctx, store, page.Sessions[0].Ref)
		if err != nil {
			return err
		}
	}

	if err := kronk.Init(kronk.WithLibPath(*libPath)); err != nil {
		return err
	}

	krn, err := kronk.NewWithContext(ctx,
		model.WithModelFiles([]string{*modelPath}),
		model.WithContextWindow(2048), model.WithNSeqMax(1),
		model.WithIncrementalCache(true), model.WithCacheMinTokens(1),
		model.WithSpeculationMode(model.SpeculationDisabled),
		model.WithStorage(model.StorageConfig{Store: store}))
	if err != nil {
		return err
	}

	defer func() { retErr = errors.Join(retErr, krn.Unload(context.Background())) }()

	// IMC discovers and restores matching state automatically. The application
	// supplies messages, not native checkpoints or a forced cache-session ID.
	response, err := krn.Chat(ctx, request)
	if err != nil {
		return err
	}

	if len(response.Choices) == 0 || response.Choices[0].Message == nil || response.Usage == nil {
		return errors.New("model returned no message or usage")
	}

	fmt.Printf("Assistant: %s\nCached prompt tokens: %d\n",
		response.Choices[0].Message.Content, response.Usage.PromptTokensDetails.CachedTokens)

	page, err = store.List(ctx, imc.ListQuery{Limit: 1})
	if err != nil {
		return err
	}

	if len(page.Sessions) == 0 {
		fmt.Println("No snapshot saved; persistence is best effort. Check the storage logs.")
		return nil
	}

	latest := page.Sessions[0].Ref

	fmt.Printf("Latest saved session: %s\nRevision: %d\n", latest.Key.ID, latest.Revision)

	return nil
}

// continueConversation reads history through the public contract. Reading
// messages does not require allocating or interpreting native state payloads.
func continueConversation(ctx context.Context, store imc.Store, ref imc.Ref) (request model.D, retErr error) {
	handle, err := store.Open(ctx, ref)
	if err != nil {
		return nil, err
	}

	defer func() {
		retErr = errors.Join(retErr, handle.Close())
	}()

	record := handle.Record()

	var conversation struct {
		Messages []model.D `json:"messages"`
	}

	if err := json.Unmarshal(record.Request, &request); err != nil {
		return nil, err
	}

	if err := json.Unmarshal(record.Request, &conversation); err != nil {
		return nil, err
	}

	request["messages"] = append(conversation.Messages,
		model.D{"role": "assistant", "content": record.AssistantText},
		model.D{"role": "user", "content": "What is our project code? Reply with only the number."})

	return request, nil
}
