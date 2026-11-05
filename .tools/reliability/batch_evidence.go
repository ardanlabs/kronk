package main

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

// Server times share a clock with each other, not necessarily with the client.
// Execution includes preparation/prefill; it is not a generation interval.
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
}

// Diagnostic only: never changes scenario failures, status or coverage verdicts.
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
			turn.ServerExecution = evidence
		}
		scenario.Details["requests"], scenario.Details["turns"] = requests, turns
	}
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
