package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"
)

// Server times share a clock with each other, not necessarily with the client.
// Execution includes slot-owned prefill; it is not a generation interval.
type batchRequestExecution struct {
	Status              string     `json:"status"`
	Slot                *int       `json:"slot,omitempty"`
	QueuedAt            *time.Time `json:"queued_at,omitempty"`
	AssignedAt          *time.Time `json:"assigned_at,omitempty"`
	ExecutionStartedAt  *time.Time `json:"execution_started_at,omitempty"`
	ExecutionFinishedAt *time.Time `json:"execution_finished_at,omitempty"`
	FinishStatus        string     `json:"finish_status,omitempty"`
	QueueWaitSeconds    *float64   `json:"queue_wait_seconds,omitempty"`
	Notes               []string   `json:"notes,omitempty"`
}

type batchTurnExecution struct {
	CompleteRequests        int      `json:"complete_requests"`
	IncompleteRequests      int      `json:"incomplete_requests"`
	InconsistentRequests    int      `json:"inconsistent_requests"`
	ObservedQueueEntries    int      `json:"observed_queue_entries"`
	AssignedSlots           []int    `json:"assigned_slots"`
	MaximumQueueWaitSeconds *float64 `json:"maximum_queue_wait_seconds,omitempty"`
	MaximumOccupiedSlots    int      `json:"maximum_occupied_slots"`
	OwnershipConsistent     bool     `json:"ownership_consistent"`
	WaitingHandoff          bool     `json:"waiting_handoff"`
	GenerationDuringPrefill bool     `json:"generation_during_prefill"`
}

// Scheduling coverage is separate from behavior. Insufficient evidence never
// truncates histories or changes a correctness verdict into a server failure.
func annotateBatchExecution(summary *runSummary) {
	for i := range summary.Scenarios {
		scenario := &summary.Scenarios[i]
		requests, ok := scenario.Details["requests"].([]batchRequestResult)
		if !strings.HasPrefix(scenario.Name, "batch") || !ok {
			continue
		}
		configuration, _ := scenario.Details["configuration"].(map[string]any)
		slots, _ := configuration["slots"].(int)
		for j := range requests {
			request := &requests[j]
			request.ServerExecution = batchExecution(summary.ServerEvidence, request.TraceID, slots)
		}
		turns, _ := scenario.Details["turns"].([]batchTurnSummary)
		progressObserved := false
		for j := range turns {
			turn := &turns[j]
			evidence := &batchTurnExecution{AssignedSlots: []int{}}
			for _, request := range requests {
				if request.Turn != turn.Turn || request.Phase != turn.Phase {
					continue
				}
				x := request.ServerExecution
				switch x.Status {
				case "complete":
					evidence.CompleteRequests++
				case "inconsistent":
					evidence.InconsistentRequests++
				default:
					evidence.IncompleteRequests++
				}
				if x.Status == "inconsistent" {
					continue // Do not aggregate contradictory observations.
				}
				if x.QueuedAt != nil {
					evidence.ObservedQueueEntries++
				}
				if x.Slot != nil && !slices.Contains(evidence.AssignedSlots, *x.Slot) {
					evidence.AssignedSlots = append(evidence.AssignedSlots, *x.Slot)
				}
				if x.QueueWaitSeconds != nil && (evidence.MaximumQueueWaitSeconds == nil || *x.QueueWaitSeconds > *evidence.MaximumQueueWaitSeconds) {
					evidence.MaximumQueueWaitSeconds = x.QueueWaitSeconds
				}
			}
			slices.Sort(evidence.AssignedSlots)
			var executions []*batchRequestExecution
			var responseIDs []string
			for _, request := range requests {
				if request.Turn == turn.Turn && request.Phase == turn.Phase {
					executions = append(executions, request.ServerExecution)
					if request.ResponseID != "" {
						responseIDs = append(responseIDs, request.ResponseID)
					}
				}
			}
			evidence.MaximumOccupiedSlots, evidence.OwnershipConsistent, evidence.WaitingHandoff = batchSlotCoverage(executions, slots)
			evidence.GenerationDuringPrefill = batchPrefillProgress(turn.SchedulerSamples, responseIDs)
			progressObserved = progressObserved || evidence.GenerationDuringPrefill
			turn.ServerExecution = evidence
			status := "not exercised"
			if evidence.OwnershipConsistent && evidence.MaximumOccupiedSlots == slots && evidence.WaitingHandoff && turn.Complete {
				status = "exercised"
			}
			scenario.Coverage = append(scenario.Coverage, pathCoverage{
				Path: fmt.Sprintf("batch scheduling turn %d %s", turn.Turn, turn.Phase), Status: status,
				Note: fmt.Sprintf("peak occupied slots=%d/%d; ownership consistent=%t; prepared waiting job handed off and completed=%t", evidence.MaximumOccupiedSlots, slots, evidence.OwnershipConsistent, evidence.WaitingHandoff),
			})
		}
		status := "not exercised"
		if progressObserved {
			status = "exercised"
		}
		scenario.Coverage = append(scenario.Coverage, pathCoverage{Path: "batch generation progress during peer prefill", Status: status,
			Note: "requires increasing generated-token counters for the same request across advancing scheduler snapshots while a different request remains in prefill; a single prefill owner is expected"})
		scenario.Details["requests"], scenario.Details["turns"] = requests, turns
	}
}

// Half-open ownership intervals: a finish and assignment at the same instant
// are a handoff, not simultaneous ownership of the same physical slot.
func batchSlotCoverage(executions []*batchRequestExecution, slots int) (int, bool, bool) {
	type boundary struct {
		at    time.Time
		slot  int
		delta int
	}
	var boundaries []boundary
	consistent := len(executions) > 0 && slots > 0
	for _, x := range executions {
		if x.Status == "inconsistent" || x.Slot == nil || x.AssignedAt == nil || x.ExecutionFinishedAt == nil || !x.ExecutionFinishedAt.After(*x.AssignedAt) {
			consistent = false
			continue
		}
		boundaries = append(boundaries, boundary{*x.AssignedAt, *x.Slot, 1}, boundary{*x.ExecutionFinishedAt, *x.Slot, -1})
	}
	slices.SortFunc(boundaries, func(a, b boundary) int {
		if order := a.at.Compare(b.at); order != 0 {
			return order
		}
		return a.delta - b.delta
	})
	active := make(map[int]int)
	peak := 0
	for _, b := range boundaries {
		active[b.slot] += b.delta
		if active[b.slot] > 1 || active[b.slot] < 0 {
			consistent = false
		}
		if active[b.slot] == 0 {
			delete(active, b.slot)
		}
		peak = max(peak, len(active))
	}
	if !consistent {
		return peak, false, false
	}
	for _, waiting := range executions {
		if waiting.Status != "complete" || waiting.FinishStatus != "complete" || waiting.QueuedAt == nil {
			continue
		}
		checkAt := []time.Time{*waiting.QueuedAt}
		for _, b := range boundaries {
			if b.delta == 1 {
				checkAt = append(checkAt, b.at)
			}
		}
		for _, at := range checkAt {
			if at.Before(*waiting.QueuedAt) || !at.Before(*waiting.AssignedAt) {
				continue
			}
			occupied := make(map[int]bool)
			for _, x := range executions {
				if !at.Before(*x.AssignedAt) && at.Before(*x.ExecutionFinishedAt) {
					occupied[*x.Slot] = true
				}
			}
			if len(occupied) != slots {
				continue
			}
			for _, x := range executions {
				if x != waiting && *x.Slot == *waiting.Slot && x.ExecutionFinishedAt.After(at) && !x.ExecutionFinishedAt.After(*waiting.AssignedAt) {
					return peak, true, true
				}
			}
		}
	}
	return peak, true, false
}

func sampleBatchScheduler(ctx context.Context, rc *runContext, model string) []engineDiagnostic {
	var samples []engineDiagnostic
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return samples
		case <-ticker.C:
			var engines []engineDiagnostic
			pollCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, err := rc.client.json(pollCtx, rc.scenario, http.MethodGet, "/v1/kronk/models/slots", nil, &engines)
			cancel()
			if err != nil {
				if ctx.Err() == nil {
					rc.artifacts.logf("batch scheduler sampling unavailable: %v; coverage may be incomplete", err)
				}
				return samples
			}
			for _, engine := range engines {
				if engine.ModelID == model && (len(samples) == 0 || samples[len(samples)-1].Iteration != engine.Iteration) {
					samples = append(samples, engine)
				}
			}
		}
	}
}

func batchPrefillProgress(samples []engineDiagnostic, responseIDs []string) bool {
	for i := 1; i < len(samples); i++ {
		before, after := samples[i-1], samples[i]
		if after.Iteration <= before.Iteration {
			continue
		}
		next := make(map[int]slotDiagnostic, len(after.Slots))
		for _, slot := range after.Slots {
			next[slot.ID] = slot
		}
		for _, p := range before.Slots {
			nextP, ok := next[p.ID]
			if !ok || p.RequestID == "" || !slices.Contains(responseIDs, p.RequestID) || nextP.RequestID != p.RequestID || !strings.Contains(p.Phase, "prefill") || !strings.Contains(nextP.Phase, "prefill") {
				continue
			}
			for _, g := range before.Slots {
				if g.ID == p.ID || g.RequestID == "" || g.RequestID == p.RequestID || !slices.Contains(responseIDs, g.RequestID) || g.Phase != "generation" {
					continue
				}
				if nextG, ok := next[g.ID]; ok && nextG.RequestID == g.RequestID && nextG.GeneratedTokens > g.GeneratedTokens {
					return true
				}
			}
		}
	}
	return false
}

func batchExecution(server serverEvidence, traceID string, slots int) *batchRequestExecution {
	x := &batchRequestExecution{Status: "complete"}
	if !server.Available || traceID == "" {
		x.Status, x.Notes = "incomplete", []string{"server log unavailable or request trace ID missing"}
		return x
	}
	invalid := func(note string) {
		x.Status = "inconsistent"
		x.Notes = append(x.Notes, note)
	}
	setTime := func(target **time.Time, raw any, label string) {
		if raw == nil {
			x.Notes = append(x.Notes, "missing "+label+" timestamp")
			return
		}
		text, _ := raw.(string)
		value, err := time.Parse(time.RFC3339Nano, text)
		if err != nil {
			invalid("invalid " + label + " timestamp")
			return
		}
		if *target != nil && !(*target).Equal(value) {
			invalid("conflicting " + label + " timestamps")
			return
		}
		*target = &value
	}
	for _, event := range server.BatchLifecycle[traceID] {
		stage, status := event["stage_name"], event["status"]
		if stage == "schedule-job" && status == "queued" {
			setTime(&x.QueuedAt, event["time"], "queued")
			continue
		}
		assignment := stage == "schedule-job" && status == "complete"
		start := stage == "execute-in-slot" && status == "started"
		finish := stage == "execute-in-slot" && (status == "complete" || status == "error" || status == "cancel" || status == "timeout")
		if !assignment && !start && !finish {
			continue
		}
		slot, ok := event["slot"].(float64)
		if event["slot"] == nil {
			x.Notes = append(x.Notes, "missing slot")
		} else if !ok || slot < 0 || slot != math.Trunc(slot) || slot >= float64(math.MaxInt) || (slots > 0 && slot >= float64(slots)) {
			invalid("invalid slot")
		} else if x.Slot != nil && *x.Slot != int(slot) {
			invalid("conflicting slot assignments")
		} else {
			value := int(slot)
			x.Slot = &value
		}
		switch {
		case assignment:
			setTime(&x.AssignedAt, event["time"], "assigned")
			text, _ := event["queue_wait"].(string)
			wait, err := time.ParseDuration(text)
			if event["queue_wait"] == nil {
				x.Notes = append(x.Notes, "missing queue wait")
			} else if err != nil || wait < 0 {
				invalid("invalid queue wait")
			} else if x.QueueWaitSeconds != nil && *x.QueueWaitSeconds != wait.Seconds() {
				invalid("conflicting queue waits")
			} else {
				value := wait.Seconds()
				x.QueueWaitSeconds = &value
			}
		case start:
			setTime(&x.ExecutionStartedAt, event["time"], "execution start")
		case finish:
			setTime(&x.ExecutionFinishedAt, event["time"], "execution finish")
			value, _ := status.(string)
			if x.FinishStatus != "" && x.FinishStatus != value {
				invalid("conflicting execution finish statuses")
			}
			x.FinishStatus = value
		}
	}
	times := []*time.Time{x.QueuedAt, x.AssignedAt, x.ExecutionStartedAt, x.ExecutionFinishedAt}
	var previous *time.Time
	for i, at := range times {
		if at == nil {
			x.Notes = append(x.Notes, fmt.Sprintf("missing %s event", []string{"queued", "assigned", "execution start", "execution finish"}[i]))
			continue
		}
		if previous != nil && at.Before(*previous) {
			invalid("server lifecycle timestamps out of order")
		}
		previous = at
	}
	if x.Status != "inconsistent" && len(x.Notes) > 0 {
		x.Status = "incomplete"
	}
	return x
}
