package main

import (
	"context"
	"errors"
	"fmt"
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
	perform := func(client *client, phase string, messages []message, expected, forbidden []string, interrupted bool) result {
		res, err := client.chat(phase, messages, 768, interrupted)
		if err == nil && !interrupted {
			err = checkAnswer(res, expected, forbidden, r.TargetOnly)
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
	a := conversation(system, "ORCHID-47", "Maren", "Northport")
	b := conversation(system, "COBALT-83", "Tobin", "Southbank")
	bFacts := []string{"COBALT-83", "Tobin", "Southbank"}
	if r.Images {
		var err error
		b, err = picturedConversation(b, "giraffe.jpg")
		if err != nil {
			return err
		}
		bFacts = append(bFacts, "giraffe")
	}
	cold := perform(&base, "cold", a, []string{"ORCHID-47", "Maren", "Northport"}, []string{"COBALT-83", "Tobin"}, false)
	if err := save(cold); err != nil {
		return err
	}
	if err := save(perform(&base, "replay", a, []string{"ORCHID-47", "Maren", "Northport"}, []string{"COBALT-83", "Tobin"}, false)); err != nil {
		return err
	}
	fresh := perform(&base, "fresh-session", b, bFacts, []string{"ORCHID-47", "Maren", "Northport"}, false)
	if err := save(fresh); err != nil {
		return err
	}
	aQuestion := message{Role: "user", Content: "Change the destination to Eastmere; keep the identifier and owner unchanged. Review the updated handover in about 80 words, including its current facts. Do not repeat the previous destination."}
	bQuestion := message{Role: "user", Content: "Change the destination to Westhaven; keep the identifier and owner unchanged. Review the updated handover in about 80 words, including its current facts. Do not repeat the previous destination."}
	aFacts, bFacts := []string{"ORCHID-47", "Maren", "Eastmere"}, []string{"COBALT-83", "Tobin", "Westhaven"}
	aForeign, bForeign := []string{"Northport", "COBALT-83", "Tobin", "Westhaven"}, []string{"Southbank", "ORCHID-47", "Maren", "Eastmere"}
	if r.Images {
		var err error
		aQuestion, err = picturedMessage(aQuestion.Content.(string)+" Also identify the animal in this first attached picture.", "giraffe.jpg")
		if err != nil {
			return err
		}
		bQuestion, err = picturedMessage(bQuestion.Content.(string)+" Identify only the animal in the newest attached picture; do not name the earlier picture's animal.", "deer/image10.jpg")
		if err != nil {
			return err
		}
		aFacts, bFacts = append(aFacts, "giraffe"), append(bFacts, "deer")
		aForeign, bForeign = append(aForeign, "deer"), append(bForeign, "giraffe")
	}
	a = continueChat(a, cold, aQuestion)
	b = continueChat(b, fresh, bQuestion)
	var appended, peer result
	var pair sync.WaitGroup
	pair.Go(func() {
		appended = perform(&base, "append", a, aFacts, aForeign, false)
		if appended.Error != "" {
			cancel()
		}
	})
	pair.Go(func() {
		peer = perform(&base, "peer-append", b, bFacts, bForeign, false)
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
	a = continueChat(a, appended, message{Role: "user", Content: "Review this updated handover in about 180 words. Preserve its current identifier, owner and destination, and any pictured animal. Explain queue checks, safe interruption, cache reuse and recovery."})
	b = continueChat(b, peer, message{Role: "user", Content: "For interrupted handover INTERRUPTED-91, write a long recovery checklist, repeating its identifier in every paragraph."})
	go func() {
		res := perform(&survivorClient, "survivor", a, aFacts, aForeign, false)
		if res.Error != "" {
			cancel()
		}
		survivorDone <- res
	}()
	interrupted := perform(&cancelClient, "cancel", b, nil, nil, true)
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
			recovered = perform(&base, "recovery", conversation(system, "ORCHID-47", "Maren", "Northport"), []string{"ORCHID-47", "Maren", "Northport"}, []string{"COBALT-83", "Tobin", "Westhaven", "Eastmere", "INTERRUPTED-91"}, false)
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
