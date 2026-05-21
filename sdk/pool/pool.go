// Package pool is the application-facing entry point for Kronk's
// model pools.
//
// One call to New gives you back a Pool that holds:
//
//   - a shared resman.Manager (built from the host's detected device
//     topology),
//   - a kronk (llama) pool wired around it,
//   - a bucky (whisper) pool wired around it.
//
// Each domain-level HTTP handler then takes the typed sub-pool it
// needs: embedapp / chatapp / etc. take p.Kronk; audioapp takes
// p.Bucky. The application never has to wire the resman manually or
// know which backend a given endpoint serves — the endpoint itself
// already encodes that choice.
//
// The generic pool engine lives in sub-package sdk/pool/engine. It is
// for backend authors only; application code should not import it
// directly.
package pool

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/applog"
	buckypool "github.com/ardanlabs/kronk/sdk/bucky/pool"
	kronkpool "github.com/ardanlabs/kronk/sdk/kronk/pool"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
	kronkmodels "github.com/ardanlabs/kronk/sdk/tools/models"
)

// Config carries the settings for the application-facing pool.
//
// KronkModels and BuckyModels are the pre-built catalogs the
// underlying backend pools consult for path / size resolution. At
// least one must be supplied; either may be nil to disable that
// backend (the corresponding p.Kronk / p.Bucky will be nil).
//
// BudgetPercent feeds the shared resman.Manager (defaults to 80 when
// zero). ModelsInPool and TTL apply to both backend pools (defaults
// to 10 and 5 minutes respectively).
type Config struct {
	Log             applog.Logger
	KronkModels     *kronkmodels.Models
	BuckyModels     *buckymodels.Models
	ModelConfigFile string
	BudgetPercent   int
	ModelsInPool    int
	TTL             time.Duration
	InsecureLogging bool
}

// Pool is the application-facing pool. It owns the shared resource
// manager and the per-backend typed pools.
type Pool struct {
	Resman *resman.Manager
	Kronk  *kronkpool.Pool
	Bucky  *buckypool.Pool
}

// Re-exports so observability code (BUI, toolapp) does not have to
// pull in the kronk sub-package just to format byte counts or talk
// about model status entries.
type ModelDetail = kronkpool.ModelDetail

const (
	ModelStatusLoaded  = kronkpool.ModelStatusLoaded
	ModelStatusLoading = kronkpool.ModelStatusLoading
)

// HumanBytes formats a byte count using decimal (SI) units.
func HumanBytes(n int64) string {
	return kronkpool.HumanBytes(n)
}

// New builds the resource manager and every enabled backend pool.
//
// At least one of cfg.KronkModels or cfg.BuckyModels must be set;
// otherwise no pools would be built and the facade would be useless.
func New(cfg Config) (*Pool, error) {
	if cfg.Log == nil {
		return nil, errors.New("new: log is required")
	}
	if cfg.KronkModels == nil && cfg.BuckyModels == nil {
		return nil, errors.New("new: at least one of kronk-models or bucky-models is required")
	}

	rm, err := resman.New(resman.Config{
		Snapshot:      resman.FromDevices(devices.List()),
		BudgetPercent: cfg.BudgetPercent,
	})
	if err != nil {
		return nil, fmt.Errorf("new: resource manager: %w", err)
	}

	p := Pool{
		Resman: rm,
	}

	if cfg.KronkModels != nil {
		kp, err := kronkpool.New(kronkpool.Config{
			Log:             cfg.Log,
			Models:          cfg.KronkModels,
			Resman:          rm,
			ModelConfigFile: cfg.ModelConfigFile,
			ModelsInPool:    cfg.ModelsInPool,
			TTL:             cfg.TTL,
			InsecureLogging: cfg.InsecureLogging,
		})
		if err != nil {
			return nil, fmt.Errorf("new: kronk pool: %w", err)
		}
		p.Kronk = kp
	}

	if cfg.BuckyModels != nil {
		bp, err := buckypool.New(buckypool.Config{
			Log:          cfg.Log,
			Models:       cfg.BuckyModels,
			Resman:       rm,
			ModelsInPool: cfg.ModelsInPool,
			TTL:          cfg.TTL,
		})
		if err != nil {
			return nil, fmt.Errorf("new: bucky pool: %w", err)
		}
		p.Bucky = bp
	}

	return &p, nil
}

// Shutdown drains every enabled backend pool. It honors the supplied
// context deadline; each backend is given the same context.
func (p *Pool) Shutdown(ctx context.Context) error {
	var errs []error

	if p.Kronk != nil {
		if err := p.Kronk.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("kronk: %w", err))
		}
	}
	if p.Bucky != nil {
		if err := p.Bucky.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("bucky: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown: %w", errors.Join(errs...))
	}
	return nil
}
