package model

import (
	"context"
	"fmt"
)

// drainSlots finishes all active slots and pending jobs during shutdown.
func (e *batchEngine) drainSlots() {
	ctx := context.Background()
	shutdownErr := fmt.Errorf("drain-slots: engine shutting down")

	activeCount := 0
	for _, s := range e.slots {
		if s.active {
			activeCount++
		}
	}

	pendingCount := len(e.requestQ)
	hasDeferredJob := e.deferredJob != nil

	e.model.log(ctx, "batch-engine", "status", "drain-started", "active_slots", activeCount,
		"pending_jobs", pendingCount, "deferred_job", hasDeferredJob)

	// Execute any pending preemption so the victim slot is properly cleaned
	// up before we drain it again via the active-slot loop below.
	if e.pendingPreempt != nil {
		e.finishSlot(e.pendingPreempt, e.pendingPreemptErr)
		e.pendingPreempt = nil
		e.pendingPreemptErr = nil
	}

	for _, s := range e.slots {
		if s.active {
			e.finishSlot(s, shutdownErr)
		}
	}

	// Fail the deferred job that was dequeued but not yet assigned to a slot.
	if e.deferredJob != nil {
		e.failJob(e.deferredJob, shutdownErr)
		e.deferredJob = nil
	}

	// Drain pending jobs that were never assigned to a slot.
	drained := 0
	for {
		select {
		case job := <-e.requestQ:
			e.failJob(job, shutdownErr)
			drained++

		default:
			e.model.log(ctx, "batch-engine", "status", "drain-finished", "drained_pending", drained)
			return
		}
	}
}
