package model

type extendedBatchOutput struct{ value uint8 }

var (
	extendedBatchOutputNone       = extendedBatchOutput{}
	extendedBatchOutputLogits     = extendedBatchOutput{value: 1}
	extendedBatchOutputEmbeddings = extendedBatchOutput{value: 2}
)
