package definition_test

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl/definition"
	"go.yaml.in/yaml/v3"
)

func TestValidName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"a", true},
		{"step-01_ready", true},
		{strings.Repeat("a", 64), true},
		{"", false},
		{strings.Repeat("a", 65), false},
		{"Step", false},
		{"stage/step", false},
		{"../step", false},
		{"step name", false},
		{"modél", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := definition.ValidName(tt.name); got != tt.want {
				t.Errorf("ValidName(%q): got %t, want %t", tt.name, got, tt.want)
			}
		})
	}
}

func TestParseStrict(t *testing.T) {
	data, err := yaml.Marshal(load(t))
	if err != nil {
		t.Fatal(err)
	}
	input := string(data)
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"null", "null"},
		{"empty mapping", "{}"},
		{"syntax", "["},
		{"unknown job field", input + "typo: true\n"},
		{"unknown stage field", strings.Replace(input, "name: preparation", "typo: preparation", 1)},
		{"unknown step field", strings.Replace(input, "heartbeat_timeout:", "heartbeat_timeot:", 1)},
		{"unknown retry field", strings.Replace(input, "maximum_attempts:", "maximum_atempts:", 1)},
		{"duplicate field", input + "name: duplicate\n"},
		{"multiple documents", input + "---\nname: another\n"},
		{"empty second document", input + "---\n"},
		{"invalid trailing document", input + "---\n["},
		{"negative duration", strings.Replace(input, "queue_timeout: 24h0m0s", "queue_timeout: -1s", 1)},
		{"overflow duration", strings.Replace(input, "queue_timeout: 24h0m0s", "queue_timeout: 9223372036854775808ns", 1)},
		{"overflow sum", strings.Replace(input, "queue_timeout: 24h0m0s", "queue_timeout: 9223372036854775807ns", 1)},
		{"numeric duration", strings.Replace(input, "queue_timeout: 24h0m0s", "queue_timeout: 123", 1)},
		{"nan coefficient", strings.Replace(input, "backoff_coefficient: 2", "backoff_coefficient: .nan", 1)},
		{"infinite coefficient", strings.Replace(input, "backoff_coefficient: 2", "backoff_coefficient: .inf", 1)},
		{"overflow attempts", strings.Replace(input, "maximum_attempts: 3", "maximum_attempts: 2147483648", 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.input == input {
				t.Fatal("invalid fixture: got unchanged input, want schema mutation")
			}
			got, err := definition.Parse([]byte(tt.input))
			if err == nil {
				t.Fatalf("Parse: got %+v, want error", got)
			}
			if !reflect.DeepEqual(got, definition.Job{}) {
				t.Errorf("Parse on error: got %+v, want zero Job", got)
			}
		})
	}

	got, err := definition.Parse(data)
	if err != nil || !reflect.DeepEqual(got, load(t)) {
		t.Errorf("YAML round trip: got %+v, %v, want fixture job, nil", got, err)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*definition.Job)
	}{
		{"empty name", func(j *definition.Job) { j.Name = "" }},
		{"long name", func(j *definition.Job) { j.Name = strings.Repeat("a", 65) }},
		{"uppercase name", func(j *definition.Job) { j.Name = "Model" }},
		{"unsafe name", func(j *definition.Job) { j.Name = "../model" }},
		{"non ASCII name", func(j *definition.Job) { j.Name = "modél" }},
		{"zero queue", func(j *definition.Job) { j.QueueTimeout = 0 }},
		{"negative queue", func(j *definition.Job) { j.QueueTimeout = -1 }},
		{"zero reservation", func(j *definition.Job) { j.ReservationTimeout = 0 }},
		{"negative reservation", func(j *definition.Job) { j.ReservationTimeout = -1 }},
		{"overflow sum", func(j *definition.Job) { j.QueueTimeout = math.MaxInt64 }},
		{"zero session attempts", func(j *definition.Job) { j.SessionAttempts = 0 }},
		{"negative session attempts", func(j *definition.Job) { j.SessionAttempts = -1 }},
		{"empty stages", func(j *definition.Job) { j.Stages = nil }},
		{"invalid stage name", func(j *definition.Job) { j.Stages[0].Name = " " }},
		{"duplicate stage", func(j *definition.Job) { j.Stages[1].Name = j.Stages[0].Name }},
		{"zero stage timeout", func(j *definition.Job) { j.Stages[0].Timeout = 0 }},
		{"negative stage timeout", func(j *definition.Job) { j.Stages[0].Timeout = -1 }},
		{"empty steps", func(j *definition.Job) { j.Stages[0].Steps = nil }},
		{"invalid step name", func(j *definition.Job) { j.Stages[0].Steps[0].Name = "/" }},
		{"duplicate step", func(j *definition.Job) { j.Stages[0].Steps[1].Name = j.Stages[0].Steps[0].Name }},
		{"zero step timeout", func(j *definition.Job) { j.Stages[0].Steps[0].Timeout = 0 }},
		{"negative step timeout", func(j *definition.Job) { j.Stages[0].Steps[0].Timeout = -1 }},
		{"zero heartbeat", func(j *definition.Job) { j.Stages[0].Steps[0].HeartbeatTimeout = 0 }},
		{"negative heartbeat", func(j *definition.Job) { j.Stages[0].Steps[0].HeartbeatTimeout = -1 }},
		{"equal heartbeat", func(j *definition.Job) { j.Stages[0].Steps[0].HeartbeatTimeout = 24 * time.Hour }},
		{"long heartbeat", func(j *definition.Job) { j.Stages[0].Steps[0].HeartbeatTimeout = 25 * time.Hour }},
		{"zero retries", func(j *definition.Job) { j.Stages[0].Steps[0].Retry.MaximumAttempts = 0 }},
		{"negative retries", func(j *definition.Job) { j.Stages[0].Steps[0].Retry.MaximumAttempts = -1 }},
		{"zero initial interval", func(j *definition.Job) { j.Stages[0].Steps[0].Retry.InitialInterval = 0 }},
		{"negative initial interval", func(j *definition.Job) { j.Stages[0].Steps[0].Retry.InitialInterval = -1 }},
		{"zero maximum interval", func(j *definition.Job) { j.Stages[0].Steps[0].Retry.MaximumInterval = 0 }},
		{"negative maximum interval", func(j *definition.Job) { j.Stages[0].Steps[0].Retry.MaximumInterval = -1 }},
		{"reversed intervals", func(j *definition.Job) { j.Stages[0].Steps[0].Retry.MaximumInterval = time.Nanosecond }},
		{"small coefficient", func(j *definition.Job) { j.Stages[0].Steps[0].Retry.BackoffCoefficient = 0.5 }},
		{"nan coefficient", func(j *definition.Job) { j.Stages[0].Steps[0].Retry.BackoffCoefficient = math.NaN() }},
		{"positive infinity", func(j *definition.Job) { j.Stages[0].Steps[0].Retry.BackoffCoefficient = math.Inf(1) }},
		{"negative infinity", func(j *definition.Job) { j.Stages[0].Steps[0].Retry.BackoffCoefficient = math.Inf(-1) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := load(t)
			tt.mutate(&job)
			if err := job.Validate(); err == nil {
				t.Error("Validate: got nil, want error")
			}
		})
	}
}

func TestBoundaryPoliciesAndJSON(t *testing.T) {
	job := load(t)
	job.Name = strings.Repeat("a", 64)
	job.QueueTimeout = math.MaxInt64 - 1
	job.ReservationTimeout = 1
	job.SessionAttempts = 1
	job.Stages[1].Steps[0].Name = job.Stages[0].Steps[0].Name // Unique only within a stage.
	job.Stages[0].Timeout = math.MaxInt64
	step := &job.Stages[0].Steps[0]
	step.Timeout = math.MaxInt64
	step.HeartbeatTimeout = math.MaxInt64 - 1
	step.Retry = definition.Retry{MaximumAttempts: 1, InitialInterval: math.MaxInt64, MaximumInterval: math.MaxInt64, BackoffCoefficient: 1}
	if err := job.Validate(); err != nil {
		t.Fatalf("boundary Validate: got %v, want nil", err)
	}
	data, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	var got definition.Job
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, job) {
		t.Errorf("JSON round trip: got %+v, want %+v", got, job)
	}
	if err := got.Validate(); err != nil {
		t.Errorf("JSON Validate: got %v, want nil", err)
	}
}

func load(t *testing.T) definition.Job {
	t.Helper()
	job := definition.Job{
		Name: "example", QueueTimeout: 24 * time.Hour, ReservationTimeout: 168 * time.Hour, SessionAttempts: 3,
	}
	for _, name := range []string{"preparation", "processing"} {
		stage := definition.Stage{Name: name, Timeout: 24 * time.Hour}
		for _, step := range []string{"first", "second"} {
			stage.Steps = append(stage.Steps, definition.Step{
				Name: step, Timeout: 24 * time.Hour, HeartbeatTimeout: 5 * time.Second,
				Retry: definition.Retry{MaximumAttempts: 3, InitialInterval: time.Second, BackoffCoefficient: 2, MaximumInterval: time.Minute},
			})
		}
		job.Stages = append(job.Stages, stage)
	}
	return job
}
