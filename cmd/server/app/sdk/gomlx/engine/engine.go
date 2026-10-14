// Package engine exposes the Gemma computation runtime without application
// policy, dataset contracts, server lifecycle, or managed-run orchestration.
package engine

import (
	"context"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/gomlx/internal/gemma"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/gomlx/internal/tokenizer"
)

type (
	Runtime              = gemma.Runtime
	AdapterConfig        = gemma.AdapterConfig
	Record               = gemma.Record
	Dataset              = gemma.Dataset
	TrainingRecord       = gemma.TrainingRecord
	PreparedDataset      = gemma.PreparedDataset
	TrainingConfig       = gemma.TrainingConfig
	TrainingResult       = gemma.TrainingResult
	TrainingProgress     = gemma.TrainingProgress
	EpochMetrics         = gemma.EpochMetrics
	TokenizationProgress = gemma.TokenizationProgress
	Config               = gemma.Config
	Result               = gemma.Result
	Tokenizer            = gemma.Tokenizer
	VerifiedTokenizer    = gemma.VerifiedTokenizer
	Client               = tokenizer.Client
)

// NewLoRARuntime loads the existing trainable runtime.
func NewLoRARuntime(modelPath string, config AdapterConfig) (*Runtime, error) {
	return gemma.NewLoRARuntime(modelPath, config)
}

// PrepareRecord formats a source pair and masks only the answer.
func PrepareRecord(ctx context.Context, source Record, tokenizer Tokenizer, config Config) (TrainingRecord, error) {
	return gemma.PrepareRecord(ctx, source, tokenizer, config)
}

// New connects to an already-running tokenizer server; it does not start one.
func New(endpoint string, httpClient *http.Client) (*Client, error) {
	return tokenizer.New(endpoint, httpClient)
}
