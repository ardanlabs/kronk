package model

// TrainingStep names one executable training operation. A stage groups steps
// in the job definition; it is not an executable handler. Execute must honor
// cancellation, preserve incoming artifacts, and validate its own outputs.
// Name identifies the step, not its containing stage.
// YAML-defined jobs register StepHandler implementations by stage/step instead.
type TrainingStep interface {
	StepHandler
	Name() string
}
