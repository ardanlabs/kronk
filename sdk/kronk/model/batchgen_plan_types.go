package model

type batchInputKind struct{ value uint8 }

var (
	batchInputUnknown    = batchInputKind{}
	batchInputTokens     = batchInputKind{value: 1}
	batchInputEmbeddings = batchInputKind{value: 2}
)

type batchPositionKind struct{ value uint8 }

var (
	batchPositionUnknown = batchPositionKind{}
	batchPositionLinear  = batchPositionKind{value: 1}
	batchPositionMRoPE   = batchPositionKind{value: 2}
)

type batchAttentionKind struct{ value uint8 }

var (
	batchAttentionUnknown   = batchAttentionKind{}
	batchAttentionCausal    = batchAttentionKind{value: 1}
	batchAttentionNonCausal = batchAttentionKind{value: 2}
)

type batchExecutionMode struct{ value uint8 }

var (
	batchExecutionDeferred = batchExecutionMode{}
	batchExecutionShared   = batchExecutionMode{value: 1}
	batchExecutionIsolated = batchExecutionMode{value: 2}
)

func (m batchExecutionMode) String() string {
	switch m {
	case batchExecutionShared:
		return "shared"
	case batchExecutionIsolated:
		return "isolated"
	default:
		return "deferred"
	}
}
