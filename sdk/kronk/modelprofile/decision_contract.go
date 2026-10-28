package modelprofile

// DecisionBatch identifies the logical and physical batching requirements.
type DecisionBatch uint8

const (
	// DecisionBatchContext uses context-sized logical batches and physical chunks.
	DecisionBatchContext DecisionBatch = iota
	// DecisionBatchPhysical limits the logical batch to the physical chunk size.
	DecisionBatchPhysical
	// DecisionBatchWholePrompt evaluates the complete context in one physical batch.
	DecisionBatchWholePrompt
)

// DecisionExecution describes a decision protocol's execution requirements.
type DecisionExecution struct {
	Batch           DecisionBatch
	SingleSequence  bool
	NonCausal       bool
	Embeddings      bool
	AllTokenOutputs bool
}

// DecisionContract declares protocol policy independent of model execution.
type DecisionContract struct {
	Execution            DecisionExecution
	InstructionsOptional bool
	temperatureBits      int
	temperatureBuckets   []decisionTemperatureBucket
}

type decisionTemperatureBucket struct {
	maxOptions int
	suffix     string
}

// DecisionContractFor returns protocol policy, with context batching and
// float64 calibration precision for unknown protocols.
func DecisionContractFor(protocol string) DecisionContract {
	contract := DecisionContract{temperatureBits: 64}
	switch protocol {
	case "laya", "kev":
		contract.Execution = DecisionExecution{Batch: DecisionBatchPhysical, Embeddings: true}
	case "clef":
		contract.Execution = DecisionExecution{Batch: DecisionBatchPhysical, Embeddings: true, AllTokenOutputs: true}
	case "vllm-decision2":
		contract.Execution = DecisionExecution{Batch: DecisionBatchWholePrompt, Embeddings: true, AllTokenOutputs: true}
		contract.temperatureBits = 32
	case "vllm-d3":
		contract.Execution = DecisionExecution{Batch: DecisionBatchWholePrompt, SingleSequence: true, NonCausal: true}
		contract.InstructionsOptional = true
		contract.temperatureBits = 32
	case "lfm3-d1", "lfm3-d1-omni":
		contract.Execution = DecisionExecution{Batch: DecisionBatchWholePrompt, SingleSequence: true, NonCausal: true}
		contract.temperatureBits = 32
	case "pplx-decider", "lfm2-d1":
		contract.temperatureBits = 32
	case "lev":
		contract.temperatureBuckets = []decisionTemperatureBucket{{8, "small"}, {26, "mid"}, {0, "large"}}
	}
	return contract
}

// TemperatureBucket returns the calibration suffix for the option count.
func (c DecisionContract) TemperatureBucket(optionCount int) string {
	buckets := c.temperatureBuckets
	if len(buckets) == 0 {
		buckets = []decisionTemperatureBucket{{2, "2"}, {5, "3_5"}, {10, "6_10"}, {0, "11"}}
	}
	for _, bucket := range buckets[:len(buckets)-1] {
		if optionCount <= bucket.maxOptions {
			return bucket.suffix
		}
	}
	return buckets[len(buckets)-1].suffix
}
