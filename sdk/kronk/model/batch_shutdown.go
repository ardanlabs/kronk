package model

import (
	"context"
	"fmt"

	"github.com/hybridgroup/yzma/pkg/mtmd"
)

// drainSlots finishes all active slots and pending jobs during shutdown.
func (e *batchEngine) drainSlots() {
	ctx := context.Background()

	activeCount := 0
	for _, s := range e.slots {
		if s.active {
			activeCount++
		}
	}

	pendingCount := len(e.requestQ)

	e.model.log(ctx, "batch-engine", "status", "drain-started", "active_slots", activeCount, "pending_jobs", pendingCount)

	for _, s := range e.slots {
		if s.active {
			e.finishSlot(s, fmt.Errorf("drain-slots: engine shutting down"))
		}
	}

	// Drain pending jobs that were never assigned to a slot.
	drained := 0
	for {
		select {
		case job := <-e.requestQ:
			if job.queueWaitSpan != nil {
				job.queueWaitSpan.End()
			}

			if job.mtmdCtx != 0 {
				mtmd.Free(job.mtmdCtx)
			}

			close(job.ch)
			e.model.activeStreams.Add(-1)
			drained++

		default:
			e.model.log(ctx, "batch-engine", "status", "drain-finished", "drained_pending", drained)
			return
		}
	}
}
