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

	pendingCount := len(e.requestQ) + len(e.pendingJobs)

	e.model.log(ctx, "batch-engine", "status", "drain-started", "active_slots", activeCount,
		"pending_jobs", pendingCount)

	for _, s := range e.slots {
		if s.active {
			e.finishSlot(s, shutdownErr)
		}
	}

	// Fail pending jobs that were dequeued but not yet assigned to a slot.
	for _, job := range e.pendingJobs {
		e.failJob(job, shutdownErr)
	}
	e.pendingJobs = nil

	// Drain pending jobs still in the request queue.
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
