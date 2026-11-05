package tmprl

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"go.yaml.in/yaml/v3"
)

// Job contains ordered stages and the policies shared by a job execution.
// Callers capture this value in workflow input before starting Temporal work.
type Job struct {
	Name               string        `json:"name" yaml:"name"`
	QueueTimeout       time.Duration `json:"queue_timeout" yaml:"queue_timeout"`
	ReservationTimeout time.Duration `json:"reservation_timeout" yaml:"reservation_timeout"`
	SessionAttempts    int           `json:"session_attempts" yaml:"session_attempts"`
	Stages             []Stage       `json:"stages" yaml:"stages"`
}

// Stage contains the ordered steps executed within its timeout.
type Stage struct {
	Name      string        `json:"name" yaml:"name"`
	WaitAfter string        `json:"wait_after,omitzero" yaml:"wait_after,omitempty"`
	Timeout   time.Duration `json:"timeout" yaml:"timeout"`
	Steps     []Step        `json:"steps" yaml:"steps"`
}

// Step contains one operation's timeout, heartbeat, and retry policies.
type Step struct {
	Name             string        `json:"name" yaml:"name"`
	WaitAfter        string        `json:"wait_after,omitzero" yaml:"wait_after,omitempty"`
	Timeout          time.Duration `json:"timeout" yaml:"timeout"`
	HeartbeatTimeout time.Duration `json:"heartbeat_timeout" yaml:"heartbeat_timeout"`
	Retry            Retry         `json:"retry" yaml:"retry"`
}

// Retry contains the bounded exponential retry policy for a step.
type Retry struct {
	MaximumAttempts    int32         `json:"maximum_attempts" yaml:"maximum_attempts"`
	InitialInterval    time.Duration `json:"initial_interval" yaml:"initial_interval"`
	BackoffCoefficient float64       `json:"backoff_coefficient" yaml:"backoff_coefficient"`
	MaximumInterval    time.Duration `json:"maximum_interval" yaml:"maximum_interval"`
}

// =============================================================================

// ParseJob strictly decodes and validates exactly one YAML job document.
func ParseJob(data []byte) (Job, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var job Job
	if err := dec.Decode(&job); err != nil {
		return Job{}, fmt.Errorf("decoding job: %w", err)
	}

	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err != nil {
			return Job{}, fmt.Errorf("decoding trailing document: %w", err)
		}
		return Job{}, errors.New("decoding job: multiple documents are not allowed")
	}

	if err := job.Validate(); err != nil {
		return Job{}, fmt.Errorf("validating job: %w", err)
	}

	return job, nil
}

// Validate checks names, ordering structure, and execution policies. Durations
// must be positive int64 nanoseconds; queue plus reservation must also fit.
// Names contain 1..64 lowercase ASCII letters, digits, hyphens, or underscores.
func (j Job) Validate() error {
	if !ValidName(j.Name) {
		return errors.New("name: must contain 1..64 lowercase ASCII letters, digits, hyphens, or underscores")
	}
	if j.QueueTimeout <= 0 || j.ReservationTimeout <= 0 {
		return errors.New("queue_timeout and reservation_timeout: must be positive")
	}
	if j.QueueTimeout > time.Duration(math.MaxInt64)-j.ReservationTimeout {
		return errors.New("queue_timeout plus reservation_timeout: duration overflow")
	}
	if j.SessionAttempts < 1 {
		return errors.New("session_attempts: must be at least 1")
	}
	if len(j.Stages) == 0 {
		return errors.New("stages: must not be empty")
	}

	names := make(map[string]bool, len(j.Stages))
	checkpoints := make(map[string]bool)
	for i, stage := range j.Stages {
		if !ValidName(stage.Name) {
			return fmt.Errorf("stages[%d].name: invalid name", i)
		}
		if names[stage.Name] {
			return fmt.Errorf("stages[%d].name: duplicate %q", i, stage.Name)
		}
		names[stage.Name] = true
		if stage.Timeout <= 0 {
			return fmt.Errorf("stage %q timeout: must be positive", stage.Name)
		}
		if len(stage.Steps) == 0 {
			return fmt.Errorf("stage %q steps: must not be empty", stage.Name)
		}
		if stage.WaitAfter != "" {
			if !ValidName(stage.WaitAfter) || checkpoints[stage.WaitAfter] || i == len(j.Stages)-1 {
				return fmt.Errorf("stage %q: invalid, duplicate, or final-stage checkpoint %q", stage.Name, stage.WaitAfter)
			}
			checkpoints[stage.WaitAfter] = true
		}

		steps := make(map[string]bool, len(stage.Steps))
		for k, step := range stage.Steps {
			if !ValidName(step.Name) {
				return fmt.Errorf("stage %q steps[%d].name: invalid name", stage.Name, k)
			}
			if steps[step.Name] {
				return fmt.Errorf("stage %q steps[%d].name: duplicate %q", stage.Name, k, step.Name)
			}
			steps[step.Name] = true
			if step.WaitAfter != "" {
				if !ValidName(step.WaitAfter) || checkpoints[step.WaitAfter] {
					return fmt.Errorf("stage %q step %q: invalid or duplicate checkpoint %q", stage.Name, step.Name, step.WaitAfter)
				}
				checkpoints[step.WaitAfter] = true
			}
			if err := validateStep(step); err != nil {
				return fmt.Errorf("stage %q step %q: %w", stage.Name, step.Name, err)
			}
		}
	}

	return nil
}

func validateStep(step Step) error {
	if step.Timeout <= 0 || step.HeartbeatTimeout <= 0 {
		return errors.New("timeout and heartbeat_timeout: must be positive")
	}
	if step.HeartbeatTimeout >= step.Timeout {
		return errors.New("heartbeat_timeout: must be less than timeout")
	}
	if step.Retry.MaximumAttempts < 1 {
		return errors.New("retry.maximum_attempts: must be at least 1")
	}
	if step.Retry.InitialInterval <= 0 || step.Retry.MaximumInterval <= 0 {
		return errors.New("retry intervals: must be positive")
	}
	if step.Retry.MaximumInterval < step.Retry.InitialInterval {
		return errors.New("retry.maximum_interval: must be at least initial_interval")
	}
	coefficient := step.Retry.BackoffCoefficient
	if math.IsNaN(coefficient) || math.IsInf(coefficient, 0) || coefficient < 1 {
		return errors.New("retry.backoff_coefficient: must be finite and at least 1")
	}
	return nil
}

// ValidName reports whether name contains 1..64 lowercase ASCII letters,
// digits, hyphens, or underscores.
func ValidName(name string) bool {
	if len(name) < 1 || len(name) > 64 {
		return false
	}
	for _, c := range name {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '_' {
			return false
		}
	}
	return true
}
