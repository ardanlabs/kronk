package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"slices"
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
	if r.WarmupRequest != nil {
		requests[r.WarmupRequest.TraceID] = r.WarmupRequest
	}
	for i := range r.Results {
		requests[r.Results[i].TraceID] = &r.Results[i]
	}
	startup := make(map[string]time.Time)
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
		if err := c.events.Encode(map[string]any{"source": "server", "cell_id": c.cell, "repetition": c.repetition, "phase": phase, "event": event}); err != nil {
			return err
		}
		if event["level"] == "ERROR" {
			errorEvents = append(errorEvents, event)
		}
		msg, _ := event["msg"].(string)
		if phase == "warmup" {
			status, _ := event["status"].(string)
			stamp, _ := event["time"].(string)
			if at, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
				startup[msg+"/"+status] = at
			}
		}
		if phase == "warmup" && strings.HasPrefix(msg, "draft-model-mtp") && event["status"] == "loaded" {
			r.MTPBackend, _ = event["backend"].(string)
		}
		if res := requests[trace]; res != nil {
			recordExecution(res, event)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if r.WarmupRequest != nil {
		r.WarmupRequest.StartupTimings = startupTimings(startup)
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
	if r.WarmupRequest != nil {
		measured = append(measured, *r.WarmupRequest)
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

// Same-server timestamps, not client/server clock subtraction. The offline
// comparator verifies the saved metrics against these recorded timestamps.
func startupTimings(bounds map[string]time.Time) map[string]float64 {
	timings := make(map[string]float64)
	for _, interval := range []struct{ metric, start, end string }{
		{"server_request_seconds", "request started/", "request completed/"},
		{"pre_chat_seconds", "request started/", "chat-completions/"},
		{"native_model_load_seconds", "loading model from file/started", "loading model from file/completed"},
		{"projection_load_seconds", "loading-prof-file/started", "loading-prof-file/completed"},
		{"mtp_load_seconds", "draft-model-mtp/loading", "draft-model-mtp/loaded"},
	} {
		start, end := bounds[interval.start], bounds[interval.end]
		if !start.IsZero() && !end.IsZero() && !end.Before(start) {
			timings[interval.metric] = end.Sub(start).Seconds()
		}
	}
	return timings
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
	case "imc-media-cache":
		switch status {
		case "plan-ready":
			r.MediaPlan = true
			r.MediaCount = eventInt(event, "media_count")
			r.ReusableTokens = eventInt(event, "reusable_logical_position")
			r.MediaAnchorKV = eventInt(event, "anchor_physical_kv")
			r.CacheMatch, _ = event["match_kind"].(string)
			r.CacheReason, _ = event["match_reason"].(string)
			r.TailTokens = eventInt(event, "replay_text_tokens")
		case "encoded-media-chunk":
			r.EncodedTokens += eventInt(event, "tokens")
		case "decoded-media-chunk":
			r.DecodedTokens += eventInt(event, "decoded_tokens")
		}
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
		case "imc-cache-ready":
			if event["media"] == true {
				r.MediaPosition = eventInt(event, "next_logical_position")
			}
		case "imc-restore-done":
			e.TargetRestored = true
		case "imc-draft-restore-done":
			e.DraftRestored = true
		case "imc-snapshot-done":
			e.TargetSnapshotted = true
		case "imc-draft-snapshot-done":
			e.DraftSnapshotted = true
		}
	case "start-slot-media":
		if status == "tokenized" {
			r.MediaPrefill = true
			r.ReusableTokens = eventInt(event, "cached_tokens")
		}
	case "prefill-media":
		if status == "encoding-shared" || status == "encoding-image" {
			r.EncodedTokens += eventInt(event, "tokens")
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
			stamp, _ := event["time"].(string)
			e.FirstToken, _ = time.Parse(time.RFC3339Nano, stamp)
		}
		if status == "slot-finished" {
			recordDuration(r, event, "elapsed", "generation_seconds")
			e.SlotFinished = true
			e.ReusedTokens = eventInt(event, "imc_actual_reused_tokens")
			e.InputTokens = eventInt(event, "total_prompt")
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
	if !r.TargetOnly && r.MTPBackend == "" {
		fail("warmup", "MTP backend activation not attested")
	}
	if r.TargetOnly && r.MTPBackend != "" {
		fail("warmup", "target-only model loaded an MTP backend")
	}
	if len(r.Results) != len(r.phases()) {
		fail("workload", "incomplete: all workload phases required")
	}
	if r.TwoSlots {
		defer validateConcurrentExecution(r)
	}
	var coldStable, freshStable, systemTokens int
	var cancelledAt time.Time
	for _, res := range r.Results {
		e := res.Execution
		validSlot := e.Slot != nil && ((!r.TwoSlots && *e.Slot == r.Slot) || (r.TwoSlots && slices.Contains(r.ConfiguredSlots, *e.Slot)))
		if !validSlot || e.SlotConflict || e.Started.IsZero() || e.Finished.Before(e.Started) || !e.SlotFinished || !e.SequenceCleared || (!r.TargetOnly && !e.DraftCleaned) {
			fail(res.Phase, "execution on configured slot, target cleanup, and any required draft cleanup not attested")
		}
		if r.TargetOnly && (e.DraftTokens != 0 || e.DraftAccepted != 0 || e.DraftRestored || e.DraftSnapshotted || e.DraftResume != "" || res.Usage.DraftTokens != 0 || res.Usage.DraftAcceptedTokens != 0 || res.Usage.DraftCoverage != 0 || res.Usage.DraftDisableReason != "") {
			fail(res.Phase, "target-only execution reported speculative activity")
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
		if res.Phase == "recovery" && (cancelledAt.IsZero() || e.Started.Before(cancelledAt)) {
			fail(res.Phase, "recovery must execute after cancelled request cleanup on the same slot")
		}
		if r.NoCache {
			if e.ReusedTokens != 0 || res.ReusableTokens != 0 || res.Usage.PromptTokensDetails.CachedTokens != 0 || res.CacheMatch != "" || res.MediaPlan || e.SystemPublished != 0 || e.TargetRestored || e.DraftRestored || e.TargetSnapshotted || e.DraftSnapshotted {
				fail(res.Phase, "IMC-off execution reported cache planning, reuse, or saved state")
			}
			if e.InputTokens <= r.SystemTokens || (!res.Cancelled && e.InputTokens != res.Usage.PromptTokens) || (!r.TargetOnly && e.DraftResume != "fresh-prefill") {
				fail(res.Phase, "full prompt prefill and any required fresh MTP prefill not attested")
			}
			// Ordinary media prefill has no IMC decode-chunk event. Encoding,
			// successful prefill/slot completion, and semantic answers attest it.
			media := r.Images && (!r.TwoSlots || (res.Phase != "cold" && res.Phase != "replay" && res.Phase != "recovery"))
			if media && (!res.MediaPrefill || res.EncodedTokens <= 0) {
				fail(res.Phase, "IMC-off image must be processed again on every request")
			}
			continue
		}
		if r.TwoSlots {
			continue // Concurrent cache paths are validated after common cleanup/mode checks.
		}
		if r.Images {
			if res.Phase == "cold" {
				coldStable = res.MediaPosition
				systemTokens = e.SystemPublished
			}
			if res.Phase == "fresh-session" {
				freshStable = res.MediaPosition
			}
			validateImageExecution(r, res, coldStable, freshStable, systemTokens)
			continue
		}
		if res.StableTokens < r.SystemTokens || res.TailTokens <= 0 || res.ReusableTokens+res.ExtensionTokens != res.StableTokens || e.ReusedTokens != res.ReusableTokens {
			fail(res.Phase, "actual cache reuse and stable-prefix accounting not attested")
		}
		if !res.Cancelled && res.StableTokens+res.TailTokens != res.Usage.PromptTokens {
			fail(res.Phase, "rendered prompt accounting disagrees with client usage")
		}
		if res.Phase == "cold" {
			coldStable, systemTokens = res.StableTokens, e.SystemPublished
			if res.CacheMatch != "rebuild" || res.ReusableTokens != 0 || res.SystemBoundary != systemTokens || systemTokens < r.SystemTokens || (!r.TargetOnly && e.DraftResume != "fresh-prefill") {
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
			if !e.TargetRestored || (!r.TargetOnly && (!e.DraftRestored || e.DraftResume != "restored-draft-kv")) {
				fail(res.Phase, "target restore and any required MTP continuation not attested")
			}
		}
		if res.Phase == "fresh-session" {
			freshStable = res.StableTokens
		}
		if res.Phase != "replay" && (!e.TargetSnapshotted || (!r.TargetOnly && !e.DraftSnapshotted)) {
			fail(res.Phase, "target snapshot and any required MTP snapshot publication not attested")
		}
	}
}

// Media plans separate logical positions from physical KV cells. Reuse counters
// follow logical positions; prompt counts include physical image embeddings.
func validateImageExecution(r *report, res result, cold, fresh, system int) {
	fail := func(text string) { r.Findings = append(r.Findings, res.Phase+": "+text) }
	e := res.Execution
	if !res.MediaPlan || res.MediaCount != 1 || res.TailTokens <= 0 {
		fail("single-image cache plan not attested")
	}
	if (res.Phase == "fresh-session" || res.Phase == "recovery") && res.CacheReason == "system-cache-prefix" {
		// The immutable text prefix can be restored independently of images.
		// This is not a media-anchor restore: the new image must still be processed.
		if res.CacheMatch != "append" || system < r.SystemTokens || system <= 0 || e.ReusedTokens != system || res.ReusableTokens != system || res.MediaAnchorKV != 0 || res.MediaPosition <= system || res.EncodedTokens <= 0 || res.DecodedTokens <= 0 {
			fail("System-prefix reuse with new image encoding/decoding not attested")
		}
		if !e.TargetRestored || (!r.TargetOnly && (!e.DraftRestored || e.DraftResume != "restored-draft-kv")) {
			fail("System target restore and any required MTP restore not attested")
		}
	} else if res.Phase == "cold" || res.Phase == "fresh-session" || (res.Phase == "recovery" && res.CacheMatch == "rebuild") {
		if res.CacheMatch != "rebuild" || e.ReusedTokens != 0 || res.MediaPosition <= 0 || res.EncodedTokens <= 0 || res.DecodedTokens <= 0 {
			fail("new image must be encoded and decoded into an uncached prefix")
		}
	} else {
		kind, reason, reused := "anchor", "media-prefix-text-replay", cold
		if res.Phase == "replay" || res.Phase == "recovery" {
			kind, reason = "exact", "logical-plan-equal"
		}
		if res.Phase == "cancel" {
			reused = fresh
		}
		if res.CacheMatch != kind || res.CacheReason != reason || reused <= 0 || e.ReusedTokens != reused || res.ReusableTokens != reused || res.MediaAnchorKV <= 0 || res.EncodedTokens != 0 || res.DecodedTokens != 0 {
			fail("required media-prefix reuse without image re-encoding not attested")
		}
		if !e.TargetRestored || (!r.TargetOnly && (!e.DraftRestored || e.DraftResume != "restored-draft-kv")) {
			fail("media target restore and any required MTP restore not attested")
		}
	}
	if res.CacheMatch != "exact" && (!e.TargetSnapshotted || (!r.TargetOnly && !e.DraftSnapshotted)) {
		fail("target snapshot and any required MTP snapshot publication not attested")
	}
}
