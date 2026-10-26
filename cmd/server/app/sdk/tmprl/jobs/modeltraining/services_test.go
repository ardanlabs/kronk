package modeltraining_test

import (
	"testing"
	"time"

	modeltraining "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/dataset"
	. "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/model"
	prepare "github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/jobs/modeltraining/standalone/preparation"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/definition"
	"github.com/ardanlabs/kronk/sdk/applog"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

type serviceConfig struct {
	Job          *definition.Job
	Steps        map[string]StepHandler
	TaskQueue    string
	StageTimeout time.Duration
	JobTimeout   time.Duration
	DatasetRoot  string
	Extractor    DocumentExtractor
	Completer    ModelCompleter
	Models       map[string]string
	Trainer      Trainer
	Backend      modeltraining.Backend
	LlamaServer  string
	LegacySteps  []TrainingStep
}

type services struct {
	Datasets    *dataset.Business
	Preparation *prepare.Business
	Training    *modeltraining.Business
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
	progress := new(tmprl.ProgressBroker[tmprl.Execution, tmprl.Progress])
	preparationQueue := ""
	if cfg.TaskQueue != "" {
		preparationQueue = cfg.TaskQueue + "-preparation"
	}
	p, err := prepare.New(c, prepare.Config{TaskQueue: preparationQueue, StageTimeout: cfg.StageTimeout, Datasets: d, Extractor: cfg.Extractor, Completer: cfg.Completer, Progress: progress, Log: applog.DiscardLogger})
	if err != nil {
		t.Fatal(err)
	}
	b, err := modeltraining.New(c, modeltraining.Config{Job: cfg.Job, Steps: cfg.Steps, TaskQueue: cfg.TaskQueue, StageTimeout: cfg.StageTimeout, JobTimeout: cfg.JobTimeout, Datasets: d, Preparation: p, Backend: cfg.Backend, Models: cfg.Models, Trainer: cfg.Trainer, LlamaServer: cfg.LlamaServer, Progress: progress, LegacySteps: cfg.LegacySteps, Log: applog.DiscardLogger})
	if err != nil {
		t.Fatal(err)
	}
	return services{Datasets: d, Preparation: p, Training: b}
}

func startServices(t *testing.T, b services) worker.Worker {
	t.Helper()
	if b.Preparation.Enabled() {
		p, err := b.Preparation.NewWorker()
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(p.Stop)
	}
	w, err := b.Training.NewWorker()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	return w
}
