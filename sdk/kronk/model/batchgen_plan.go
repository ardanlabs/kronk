package model

import "github.com/ardanlabs/kronk/sdk/kronk/modelprofile"

type batchContributionRequirements struct {
	input     batchInputKind
	positions batchPositionKind
	attention batchAttentionKind
	rows      int
	mustFit   bool
}

type batchContributionPlan struct {
	mode batchExecutionMode
	rows int
}

func planBatchContribution(inputMixing modelprofile.BatchInputMixing, requirements batchContributionRequirements, availableRows, prefillLimit int) batchContributionPlan {
	if requirements.rows <= 0 {
		return batchContributionPlan{}
	}
	if requirements.positions != batchPositionLinear && requirements.positions != batchPositionMRoPE {
		return batchContributionPlan{mode: batchExecutionIsolated, rows: requirements.rows}
	}
	if requirements.attention != batchAttentionCausal {
		return batchContributionPlan{mode: batchExecutionIsolated, rows: requirements.rows}
	}

	availableRows = max(availableRows, 0)
	prefillLimit = max(prefillLimit, 0)

	switch requirements.input {
	case batchInputTokens:
		rows := min(requirements.rows, availableRows, prefillLimit)
		if rows == 0 {
			return batchContributionPlan{}
		}
		return batchContributionPlan{
			mode: batchExecutionShared,
			rows: rows,
		}

	case batchInputEmbeddings:
		if inputMixing != modelprofile.BatchInputMixingSupported {
			return batchContributionPlan{mode: batchExecutionIsolated, rows: requirements.rows}
		}
		if requirements.mustFit && requirements.rows > min(availableRows, prefillLimit) {
			return batchContributionPlan{mode: batchExecutionIsolated, rows: requirements.rows}
		}

		rows := min(requirements.rows, availableRows, prefillLimit)
		if rows == 0 {
			return batchContributionPlan{}
		}
		return batchContributionPlan{
			mode: batchExecutionShared,
			rows: rows,
		}

	default:
		return batchContributionPlan{mode: batchExecutionIsolated, rows: requirements.rows}
	}
}
