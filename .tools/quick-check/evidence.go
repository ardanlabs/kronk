package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

func collectEvidence(file *os.File, initial os.FileInfo, path string, c *client, r *report) error {
	current, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !os.SameFile(initial, current) || current.Size() < initial.Size() {
		return fmt.Errorf("server log rotated or truncated; evidence incomplete")
	}
	requests := make(map[string]*result)
	if r.FirstRequest != nil {
		requests[r.FirstRequest.TraceID] = r.FirstRequest
	}
	for i := range r.Results {
		requests[r.Results[i].TraceID] = &r.Results[i]
	}
	var errorEvents []map[string]any
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 65536), 4*1024*1024)
	for scanner.Scan() {
		var event map[string]any
		if json.Unmarshal(scanner.Bytes(), &event) != nil {
			continue
		}
		trace, _ := event["trace_id"].(string)
		phase, ok := c.traces[trace]
		if !ok {
			continue
		}
		if err := c.events.Encode(map[string]any{"source": "server", "phase": phase, "event": event}); err != nil {
			return err
		}
		if event["level"] == "ERROR" {
			errorEvents = append(errorEvents, event)
		}
		msg, _ := event["msg"].(string)
		if phase == "load" && strings.HasPrefix(msg, "draft-model-mtp") && event["status"] == "loaded" {
			r.MTPBackend, _ = event["backend"].(string)
		}
		if res := requests[trace]; res != nil {
			recordExecution(res, event)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	for _, event := range errorEvents {
		trace, _ := event["trace_id"].(string)
		res := requests[trace]
		// The HTTP middleware logs a deliberate disconnect as ERROR. Accept only
		// its structured canceled outcome on a trace we actively cancelled and
		// whose engine also acknowledged cancellation. All other errors remain.
		if res != nil && res.Cancelled && res.Execution.TerminalStatus == "cancel" && event["msg"] == "handled error during request" {
			outer, _ := event["err"].(map[string]any)
			inner, _ := outer["error"].(map[string]any)
			text, _ := inner["message"].(string)
			if inner["code"] == "canceled" && strings.HasSuffix(text, "context canceled") {
				res.Execution.ExpectedErrors++
				continue
			}
		}
		r.Findings = append(r.Findings, fmt.Sprintf("%s: unexpected server ERROR: %v: %v", c.traces[trace], event["msg"], event["err"]))
	}
	validateExecution(r)
	measured := append([]result{}, r.Results...)
	if r.FirstRequest != nil {
		measured = append(measured, *r.FirstRequest)
	}
	for _, res := range measured {
		for _, key := range []string{"admission_seconds", "prepare_seconds", "queue_wait_seconds", "slot_execution_seconds", "slot_prefill_ttft_seconds", "generation_seconds"} {
			if _, ok := res.ServerTimings[key]; !ok {
				r.Findings = append(r.Findings, res.Phase+": missing or invalid server timing "+key)
			}
		}
	}
	r.evidenceValidated = true
	return nil
}

func recordDuration(r *result, event map[string]any, field, metric string) {
	text, _ := event[field].(string)
	duration, err := time.ParseDuration(text)
	if err != nil || duration < 0 {
		return
	}
	if r.ServerTimings == nil {
		r.ServerTimings = make(map[string]float64)
	}
	r.ServerTimings[metric] = duration.Seconds()
}

func eventInt(event map[string]any, key string) int {
	n, _ := event[key].(float64)
	return int(n)
}

func recordExecution(r *result, event map[string]any) {
	e := &r.Execution
	if slot, ok := event["slot"].(float64); ok && e.Slot != nil && int(slot) != *e.Slot {
		e.SlotConflict = true
	}
	msg, _ := event["msg"].(string)
	status, _ := event["status"].(string)
	switch msg {
	case "imc":
		if status == "plan-ready" {
			r.CacheMatch, _ = event["match_kind"].(string)
			r.CacheReason, _ = event["match_reason"].(string)
			r.ReusableTokens = eventInt(event, "reusable_tokens")
			r.StableTokens = eventInt(event, "stable_tokens")
			r.ExtensionTokens = eventInt(event, "extension_tokens")
			r.TailTokens = eventInt(event, "tail_tokens")
			r.SystemBoundary = eventInt(event, "system_boundary_tokens")
		}
		if status == "system-cache-published" {
			e.SystemPublished = eventInt(event, "tokens")
		}
	case "start-slot":
		switch status {
		case "imc-restore-done":
			e.TargetRestored = true
		case "imc-draft-restore-done":
			e.DraftRestored = true
		case "imc-snapshot-done":
			e.TargetSnapshotted = true
		case "imc-draft-snapshot-done":
			e.DraftSnapshotted = true
		}
	case "speculative":
		if status == "draft-kv-cleared" || status == "draft-kv-trimmed" {
			e.DraftCleaned = true
		}
	case "finish-slot":
		if status == "seq-cleared" {
			e.SequenceCleared = true
		}
	case "batch-engine":
		if status == "prefill-done" {
			recordDuration(r, event, "ttft", "slot_prefill_ttft_seconds")
		}
		if status == "slot-finished" {
			recordDuration(r, event, "elapsed", "generation_seconds")
			e.SlotFinished = true
			e.ReusedTokens = eventInt(event, "imc_actual_reused_tokens")
			e.OutputTokens = eventInt(event, "output_tokens")
			e.DraftTokens = eventInt(event, "draft_tokens")
			e.DraftAccepted = eventInt(event, "draft_accepted_tokens")
			e.DraftResume, _ = event["draft_resume_source"].(string)
		}
	case "request-lifecycle":
		stage := eventInt(event, "stage")
		if status == "complete" {
			switch stage {
			case 1:
				recordDuration(r, event, "elapsed", "admission_seconds")
			case 2:
				recordDuration(r, event, "elapsed", "prepare_seconds")
			case 3:
				recordDuration(r, event, "queue_wait", "queue_wait_seconds")
			}
		}
		if stage != 4 {
			return
		}
		timestamp, _ := event["time"].(string)
		at, _ := time.Parse(time.RFC3339Nano, timestamp)
		if status == "started" {
			if slot, ok := event["slot"].(float64); ok {
				id := int(slot)
				e.Slot = &id
			}
			e.Started = at
		} else {
			recordDuration(r, event, "elapsed", "slot_execution_seconds")
			e.TerminalStatus, e.Finished = status, at
		}
	}
}

func validateExecution(r *report) {
	fail := func(phase, text string) { r.Findings = append(r.Findings, phase+": "+text) }
	if r.MTPBackend == "" {
		fail("load", "MTP backend activation not attested")
	}
	if len(r.Results) != 6 {
		fail("workload", "incomplete: all six phases required")
	}
	var coldStable, freshStable, systemTokens int
	var cancelledAt time.Time
	for _, res := range r.Results {
		e := res.Execution
		if e.Slot == nil || *e.Slot != r.Slot || e.SlotConflict || e.Started.IsZero() || e.Finished.Before(e.Started) || !e.SlotFinished || !e.SequenceCleared || !e.DraftCleaned {
			fail(res.Phase, "execution on configured slot and target/draft cleanup not attested")
		}
		terminal := "complete"
		if res.Phase == "cancel" {
			terminal = "cancel"
			cancelledAt = e.Finished
			if !res.Cancelled || res.Content == "" {
				fail(res.Phase, "client did not cancel after content")
			}
		}
		if e.TerminalStatus != terminal {
			fail(res.Phase, "server terminal status must be "+terminal)
		}
		if !res.Cancelled && (e.OutputTokens != res.Usage.CompletionTokens || e.DraftTokens != res.Usage.DraftTokens || e.DraftAccepted != res.Usage.DraftAcceptedTokens || e.ReusedTokens != res.Usage.PromptTokensDetails.CachedTokens) {
			fail(res.Phase, "server completion/drafting/reuse counters disagree with client usage")
		}
		if res.StableTokens < r.SystemTokens || res.TailTokens <= 0 || res.ReusableTokens+res.ExtensionTokens != res.StableTokens || e.ReusedTokens != res.ReusableTokens {
			fail(res.Phase, "actual cache reuse and stable-prefix accounting not attested")
		}
		if !res.Cancelled && res.StableTokens+res.TailTokens != res.Usage.PromptTokens {
			fail(res.Phase, "rendered prompt accounting disagrees with client usage")
		}
		if res.Phase == "cold" {
			coldStable, systemTokens = res.StableTokens, e.SystemPublished
			if res.CacheMatch != "rebuild" || res.ReusableTokens != 0 || res.SystemBoundary != systemTokens || systemTokens < r.SystemTokens || e.DraftResume != "fresh-prefill" {
				fail(res.Phase, "uncached build and large System-prefix publication not attested")
			}
		} else {
			kind, reason, reused := "append", "complete-prefix-append", coldStable
			if res.Phase == "cancel" {
				reused = freshStable
			}
			if res.Phase == "replay" {
				kind, reason = "exact", "complete-prefix-equal"
			}
			if res.Phase == "fresh-session" || res.Phase == "recovery" {
				reason, reused = "system-cache-prefix", systemTokens
			}
			if res.CacheMatch != kind || res.CacheReason != reason || reused <= 0 || res.ReusableTokens != reused || (kind == "exact" && res.ExtensionTokens != 0) || (kind == "append" && res.ExtensionTokens <= 0) {
				fail(res.Phase, "required "+reason+" reuse extent not attested")
			}
			if !e.TargetRestored || !e.DraftRestored || e.DraftResume != "restored-draft-kv" {
				fail(res.Phase, "paired target/MTP restore and drafting continuation not attested")
			}
		}
		if res.Phase == "fresh-session" {
			freshStable = res.StableTokens
		}
		if res.Phase != "replay" && (!e.TargetSnapshotted || !e.DraftSnapshotted) {
			fail(res.Phase, "paired target/MTP snapshot publication not attested")
		}
		if res.Phase == "recovery" && (cancelledAt.IsZero() || e.Started.Before(cancelledAt)) {
			fail(res.Phase, "recovery must execute after cancelled request cleanup on the same slot")
		}
	}
}
