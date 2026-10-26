package training_test

import (
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/prepare/workflow"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/shared/dataset"
	. "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/shared/model"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/training/workflow"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

type serviceConfig struct {
	TaskQueue    string
	StageTimeout time.Duration
	DatasetRoot  string
	Extractor    DocumentExtractor
	Completer    ModelCompleter
	Models       map[string]string
	Trainer      Trainer
	Backend      training.Backend
	LlamaServer  string
}

type services struct {
	Datasets    *dataset.Business
	Preparation *prepare.Business
	Training    *training.Business
}

func newServices(t *testing.T, c client.Client, cfg serviceConfig) services {
	t.Helper()
	if cfg.DatasetRoot == "" {
		cfg.DatasetRoot = t.TempDir()
	}
	d, err := dataset.New(dataset.Config{DatasetRoot: cfg.DatasetRoot})
	if err != nil {
		t.Fatal(err)
	}
	progress := new(tmprl.ProgressBroker[Execution, Progress])
	p, err := prepare.New(c, prepare.Config{TaskQueue: cfg.TaskQueue, StageTimeout: cfg.StageTimeout, Datasets: d, Extractor: cfg.Extractor, Completer: cfg.Completer, Progress: progress})
	if err != nil {
		t.Fatal(err)
	}
	b, err := training.New(c, training.Config{TaskQueue: cfg.TaskQueue, StageTimeout: cfg.StageTimeout, Datasets: d, Preparation: p, Backend: cfg.Backend, Models: cfg.Models, Trainer: cfg.Trainer, LlamaServer: cfg.LlamaServer, Progress: progress})
	if err != nil {
		t.Fatal(err)
	}
	return services{Datasets: d, Preparation: p, Training: b}
}

func startServices(t *testing.T, c client.Client, queue string, b services) worker.Worker {
	t.Helper()
	w := tmprl.NewWorker(c, queue, b.Training.WorkerOptions(), func(r worker.Registry) {
		b.Preparation.Register(r)
		b.Training.Register(r)
	})
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	return w
}
