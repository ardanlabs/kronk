package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Two short conversations exercise overlap and cancellation ownership, not
// sustained load. Record results canonically; request events retain arrival order.
func runConcurrent(c *client, r *report, system string, log func(string, ...any)) error {
	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()
	base := *c
	base.ctx = ctx
	start := time.Now()
	defer func() { r.WorkloadSeconds = time.Since(start).Seconds() }()
	perform := func(client *client, phase string, messages []message, interrupted bool) result {
		res, err := client.chat(phase, messages, 768, interrupted)
		if err == nil && !interrupted {
			err = checkResponse(res, r.TargetOnly)
		}
		if err != nil {
			res.Error = err.Error()
		}
		return res
	}
	save := func(results ...result) error {
		var err error
		for _, res := range results {
			r.Results = append(r.Results, res)
			log("phase=%s elapsed=%.3fs first_content=%.3fs prompt=%d cached=%d output=%d drafts=%d error=%s", res.Phase, res.Elapsed, res.FirstContent, res.Usage.PromptTokens, res.Usage.PromptTokensDetails.CachedTokens, res.Usage.CompletionTokens, res.Usage.DraftTokens, res.Error)
			if res.Error != "" {
				err = errors.Join(err, fmt.Errorf("%s: %s", res.Phase, res.Error))
			}
		}
		return err
	}
	continueChat := func(history []message, answer result, question message) []message {
		return append(append([]message{}, history...), message{Role: "assistant", Content: answer.Content}, question)
	}
	a := conversation(system, "River", "Alice", "Apple")
	b := conversation(system, "Mountain", "Bob", "Lemon")
	if r.Images {
		var err error
		b, err = picturedConversation(b, "giraffe.jpg")
		if err != nil {
			return err
		}
	}
	cold := perform(&base, "cold", a, false)
	if err := save(cold); err != nil {
		return err
	}
	if err := save(perform(&base, "replay", a, false)); err != nil {
		return err
	}
	fresh := perform(&base, "fresh-session", b, false)
	if err := save(fresh); err != nil {
		return err
	}
	aQuestion := message{Role: "user", Content: "The destination is now Orange; keep the identifier and owner unchanged. Write a review in four short paragraphs totaling 60–80 words. Begin with the current identifier, owner and destination. Explain queue checks, safe interruption, cache reuse and recovery."}
	bQuestion := message{Role: "user", Content: "The destination is now Cherry; keep the identifier and owner unchanged. Write a review in four short paragraphs totaling 60–80 words. Begin with the current identifier, owner and destination. Explain queue checks, safe interruption, cache reuse and recovery."}
	if r.Images {
		var err error
		aQuestion, err = picturedMessage(strings.Replace(aQuestion.Content.(string), "Begin with the current identifier, owner and destination.", "In the opening paragraph, include the current identifier, owner and destination, and name the animal shown in the image attached to this message.", 1), "giraffe.jpg")
		if err != nil {
			return err
		}
		bQuestion, err = picturedMessage(strings.Replace(bQuestion.Content.(string), "Begin with the current identifier, owner and destination.", "In the opening paragraph, include the current identifier, owner and destination, and name only the animal shown in the image attached to this message. Omit the animal from earlier images.", 1), "deer/image10.jpg")
		if err != nil {
			return err
		}
	}
	a = continueChat(a, cold, aQuestion)
	b = continueChat(b, fresh, bQuestion)
	var appended, peer result
	var pair sync.WaitGroup
	pair.Go(func() {
		appended = perform(&base, "append", a, false)
		if appended.Error != "" {
			cancel()
		}
	})
	pair.Go(func() {
		peer = perform(&base, "peer-append", b, false)
		if peer.Error != "" {
			cancel()
		}
	})
	pair.Wait()
	if err := save(appended, peer); err != nil {
		return err
	}

	// Delay B's disconnect until A has delivered content. Server timestamps
	// below must still prove overlap and A's progress after that disconnect.
	survivorReady, survivorDone := make(chan struct{}), make(chan result, 1)
	survivorClient, cancelClient := base, base
	var ready sync.Once
	survivorClient.onContent = func(context.Context) error {
		ready.Do(func() { close(survivorReady) })
		return nil
	}
	cancelClient.onContent = func(ctx context.Context) error {
		select {
		case <-survivorReady:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	survivorQuestion := "Write a review of the current handover in four paragraphs totaling about 180 words. Begin with its current identifier, owner and destination. Use only current values. Do not mention earlier destinations or explain how the handover changed. Explain queue checks, safe interruption, cache reuse and recovery."
	if r.Images {
		survivorQuestion = strings.Replace(survivorQuestion, "Begin with its current identifier, owner and destination.", "In the opening paragraph, include its current identifier, owner and destination, and name the animal shown in the most recently supplied image in this conversation. Omit animals from earlier images.", 1)
	}
	a = continueChat(a, appended, message{Role: "user", Content: survivorQuestion})
	b = continueChat(b, peer, message{Role: "user", Content: "For interrupted handover Forest, write a long recovery checklist, repeating its identifier in every paragraph."})
	go func() {
		res := perform(&survivorClient, "survivor", a, false)
		if res.Error != "" {
			cancel()
		}
		survivorDone <- res
	}()
	interrupted := perform(&cancelClient, "cancel", b, true)
	var recovered result
	var err error
	if interrupted.Error != "" {
		err = fmt.Errorf("cancel: %s", interrupted.Error)
	} else if !interrupted.Cancelled {
		err = fmt.Errorf("cancel: active cancellation was not observed")
	} else {
		released := time.Now()
		err = base.waitPeerIdle()
		r.CancelReleaseSeconds = time.Since(released).Seconds()
		if err == nil {
			recovered = perform(&base, "recovery", conversation(system, "River", "Alice", "Apple"), false)
		}
	}
	if err != nil || recovered.Error != "" {
		cancel()
	}
	survivor := <-survivorDone // Always join before evidence extraction or unload.
	err = errors.Join(err, save(survivor, interrupted))
	if recovered.Phase != "" {
		err = errors.Join(err, save(recovered))
	}
	if err == nil {
		err = base.waitIdle()
	}
	return err
}

func (c *client) waitPeerIdle() error {
	deadline := time.Now().Add(15 * time.Second)
	for {
		poll := *c
		poll.timeout = min(c.timeout, time.Until(deadline))
		if poll.timeout <= 0 {
			return fmt.Errorf("cancel: peer capacity not released after 15s")
		}
		e, err := poll.snapshot()
		if err != nil {
			return err
		}
		idle := 0
		for _, slot := range e.Slots {
			if slot.Phase == "idle" {
				idle++
			}
		}
		if len(e.Slots) != 2 || idle == 2 {
			return fmt.Errorf("survivor: expected one active slot during recovery admission")
		}
		if idle == 1 {
			return nil
		}
		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func validateConcurrentExecution(r *report) {
	fail := func(phase, text string) { r.Findings = append(r.Findings, phase+": "+text) }
	byPhase := make(map[string]result)
	for _, res := range r.Results {
		byPhase[res.Phase] = res
	}
	if len(byPhase) != 8 || len(r.ConfiguredSlots) != 2 || r.ConfiguredSlots[0] == r.ConfiguredSlots[1] {
		fail("workload", "eight phases and two distinct configured slots required")
		return
	}
	overlaps := func(a, b result) bool {
		x, y := a.Execution, b.Execution
		return x.Slot != nil && y.Slot != nil && *x.Slot != *y.Slot &&
			!x.FirstToken.IsZero() && !y.FirstToken.IsZero() &&
			!x.FirstToken.Before(x.Started) && x.FirstToken.Before(x.Finished) &&
			!y.FirstToken.Before(y.Started) && y.FirstToken.Before(y.Finished) &&
			x.FirstToken.Before(y.Finished) && y.FirstToken.Before(x.Finished)
	}
	if !overlaps(byPhase["append"], byPhase["peer-append"]) {
		fail("append", "generation overlap on distinct slots not attested")
	}
	survivor, interrupted, recovery := byPhase["survivor"], byPhase["cancel"], byPhase["recovery"]
	if !overlaps(survivor, interrupted) || !survivor.LastContentAt.After(interrupted.Started.Add(time.Duration(interrupted.Elapsed*float64(time.Second)))) {
		fail("survivor", "overlap and continued content after peer cancellation not attested")
	}
	x, y, z := survivor.Execution, interrupted.Execution, recovery.Execution
	if y.Slot == nil || z.Slot == nil || x.Slot == nil || *y.Slot != *z.Slot || *x.Slot == *z.Slot || z.Started.Before(y.Finished) || !z.Started.Before(x.Finished) {
		fail("recovery", "must start on cancelled slot after cleanup while survivor remains active")
	}
	if r.NoCache {
		return // Common validation checks full prefill and zero saved/reused state.
	}
	system := byPhase["cold"].Execution.SystemPublished
	for _, res := range r.Results {
		e := res.Execution
		kind, reason := "append", "complete-prefix-append"
		media := r.Images && res.Phase != "cold" && res.Phase != "replay" && res.Phase != "recovery"
		parent := ""
		switch res.Phase {
		case "cold":
			kind, reason = "rebuild", ""
			if system < r.SystemTokens || system <= 0 || e.ReusedTokens != 0 || (!r.TargetOnly && e.DraftResume != "fresh-prefill") {
				fail(res.Phase, "uncached text build and System publication not attested")
			}
		case "replay":
			kind, reason = "exact", "complete-prefix-equal"
			parent = "cold"
		case "fresh-session", "recovery":
			reason = "system-cache-prefix"
			if res.ReusableTokens != system || system <= 0 {
				fail(res.Phase, "independent conversation must reuse System prefix")
			}
		case "append":
			parent = "cold"
			if media {
				kind, reason = "media-append", "text-prefix-media-append"
			}
		case "peer-append":
			parent = "fresh-session"
			if media {
				kind, reason = "media-append", "media-prefix-append"
			}
		case "survivor", "cancel":
			parent = "append"
			if res.Phase == "cancel" {
				parent = "peer-append"
			}
			if media {
				kind, reason = "anchor", "media-prefix-text-replay"
			}
		}
		if res.CacheMatch != kind || (reason != "" && res.CacheReason != reason) || e.ReusedTokens != res.ReusableTokens {
			fail(res.Phase, "required "+kind+"/"+reason+" cache path not attested")
		}
		if parent != "" {
			previous := byPhase[parent]
			reused := previous.StableTokens
			if previous.MediaPlan {
				reused = previous.MediaPosition
			}
			if reused <= 0 || res.ReusableTokens != reused {
				fail(res.Phase, "required conversation-prefix reuse extent not attested")
			}
		}
		if res.Phase != "cold" && (res.ReusableTokens <= 0 || !e.TargetRestored || (!r.TargetOnly && (!e.DraftRestored || e.DraftResume != "restored-draft-kv"))) {
			fail(res.Phase, "target restore and any required draft continuation not attested")
		}
		if kind != "exact" && (!e.TargetSnapshotted || (!r.TargetOnly && !e.DraftSnapshotted)) {
			fail(res.Phase, "target snapshot and any required draft snapshot not attested")
		}
		if media {
			count := 1
			if res.Phase == "peer-append" || res.Phase == "cancel" {
				count = 2
			}
			newImage := res.Phase == "fresh-session" || res.Phase == "append" || res.Phase == "peer-append"
			if !res.MediaPlan || res.MediaCount != count || res.TailTokens <= 0 || (newImage && (res.EncodedTokens <= 0 || res.DecodedTokens <= 0)) || (!newImage && (res.EncodedTokens != 0 || res.DecodedTokens != 0)) {
				fail(res.Phase, "media history and new-image-only encoding/decoding not attested")
			}
		} else if res.StableTokens < r.SystemTokens || res.TailTokens <= 0 || res.ReusableTokens+res.ExtensionTokens != res.StableTokens || (!res.Cancelled && res.StableTokens+res.TailTokens != res.Usage.PromptTokens) {
			fail(res.Phase, "text stable-prefix and prompt accounting not attested")
		}
	}
}
