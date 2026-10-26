package tmprl

import (
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

// NewWorker constructs a worker and registers workflow-specific execution.
// The host owns calling Start/Run and Stop on the returned worker. It must stop
// workers before closing the borrowed client. Options are supplied by the host
// or workflow package, not imposed by generic infrastructure.
func NewWorker(c client.Client, queue string, options worker.Options, register func(worker.Registry)) worker.Worker {
	w := worker.New(c, queue, options)
	register(w)
	return w
}
