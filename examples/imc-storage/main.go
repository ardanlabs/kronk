// This example verifies disk-backed IMC across separate program runs for text,
// vision, own-KV MTP, and shared-target-KV MTP. It also provides cold and changed
// image controls. See README.md for models, commands, and pass criteria.
// Application-managed text replay is not a manual native-state restore API.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
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
	phase := flag.String("phase", "save", "save, restore, cold, or changed-image (separate processes)")
	scenario := flag.String("scenario", "text", "text, vision, mtp-own, or mtp-shared")
	modelPath := flag.String("model", "", "path to an installed GGUF model")
	projPath := flag.String("proj", "", "vision projector GGUF (required for vision)")
	drafterPath := flag.String("mtp-drafter", "", "optional companion MTP GGUF; omit for embedded MTP")
	directory := flag.String("directory", "imc-sessions", "private IMC storage directory")
	libPath := flag.String("lib-path", "", "optional llama.cpp library directory")
	flag.Parse()

	if *modelPath == "" || (*phase != "save" && *phase != "restore" && *phase != "cold" && *phase != "changed-image") {
		return errors.New("provide -model and -phase save|restore|cold|changed-image")
	}
	if (*scenario == "vision" && *projPath == "") || (*phase == "changed-image" && *scenario != "vision") {
		return errors.New("vision needs -proj; changed-image is only available for vision")
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

	request, err := initialRequest(*scenario)
	if err != nil {
		return err
	}

	var previous imc.Summary

	switch *phase {
	case "save":
		if len(page.Sessions) != 0 {
			return errors.New("save phase needs a new directory; existing history is not overwritten by this example")
		}

	case "restore", "cold", "changed-image":
		if len(page.Sessions) == 0 {
			return errors.New("run the save phase first")
		}

		previous = page.Sessions[0]

		record, err := inspectSnapshot(ctx, store, previous.Ref, *scenario, false)
		if err != nil {
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

		if *phase == "changed-image" {
			if len(conversation.Messages) != 1 {
				return errors.New("changed-image needs the original save; use a separate directory before running restore")
			}

			image, err := colorImage(true)
			if err != nil {
				return err
			}

			// Keep the original input text unchanged; do not replay the saved
			// final answer, which would reveal the original image's color.
			conversation.Messages[0] = model.ImageMessage(visionQuestion, image, "png")[0]
			request["messages"] = conversation.Messages
		} else {
			request["messages"] = append(conversation.Messages,
				model.D{"role": "assistant", "content": record.AssistantText},
				model.D{"role": "user", "content": followUp(*scenario)})
		}
	}

	if err := kronk.Init(kronk.WithLibPath(*libPath)); err != nil {
		return err
	}

	var observed evidence
	opts := []model.Option{
		model.WithModelFiles([]string{*modelPath}),
		model.WithProjFile(*projPath), model.WithMTPDrafterFile(*drafterPath),
		model.WithContextWindow(2048), model.WithNSeqMax(1),
		model.WithIncrementalCache(true), model.WithCacheMinTokens(1),
		model.WithLog(observed.log),
	}

	if *phase != "cold" {
		opts = append(opts, model.WithStorage(model.StorageConfig{Store: store}))
	}

	speculation := model.SpeculationDisabled
	if strings.HasPrefix(*scenario, "mtp-") {
		speculation = model.SpeculationMTP // Never silently select target-only.
	}

	opts = append(opts, model.WithSpeculationMode(speculation))
	krn, err := kronk.NewWithContext(ctx, opts...)
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
	if err := observed.check(*scenario, *phase, response); err != nil {
		return err
	}

	page, err = store.List(ctx, imc.ListQuery{Limit: 2})
	if err != nil {
		return err
	}

	if *phase == "cold" {
		if len(page.Sessions) != 1 || page.Sessions[0] != previous {
			return errors.New("cold control changed saved history")
		}
		fmt.Printf("PASS: %s cold control; history unchanged\n", *scenario)
		return nil
	}

	wantSessions := 1
	if *phase == "changed-image" {
		wantSessions = 2
	}

	if len(page.Sessions) != wantSessions {
		return fmt.Errorf("expected %d saved conversations, got %d", wantSessions, len(page.Sessions))
	}

	current := page.Sessions[0]
	if *phase == "restore" && (current.Ref.Key != previous.Ref.Key || current.Ref.Revision <= previous.Ref.Revision) {
		return errors.New("restored checkpoint did not advance under the same conversation ID")
	}

	if *phase == "changed-image" && (current.Ref.Key == previous.Ref.Key || page.Sessions[1] != previous) {
		return errors.New("changed image reused or overwrote the original conversation")
	}

	record, err := inspectSnapshot(ctx, store, current.Ref, *scenario, *phase == "changed-image")
	if err != nil {
		return err
	}

	if record.AssistantText != response.Choices[0].Message.Content {
		return errors.New("stored assistant text differs from the delivered response")
	}

	fmt.Printf("Saved conversation: %s\nRevision: %d\n", current.Ref.Key.ID, current.Ref.Revision)
	fmt.Printf("PASS: %s %s phase\n", *scenario, *phase)

	return nil
}
