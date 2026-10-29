package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func TestFactsBoxWarnsWhenAutoTuneDoesNotFit(t *testing.T) {
	a := newApp(context.Background(), nil, true)
	a.facts = source.Facts{
		ID:            "a/b",
		HardwareKnown: true,
		Analysis: models.Analysis{
			Recommended: models.RuntimeRecommendation{
				ContextWindow:      32768,
				NSeqMax:            1,
				CacheTypeK:         "f16",
				EstimatedVRAMBytes: 29 * 1 << 30,
				Fits:               false,
			},
		},
	}

	view := a.factsView()
	if !strings.Contains(view, "May not fully fit in the target GPU") {
		t.Fatalf("facts box has no fit warning:\n%s", view)
	}
	if strings.Contains(view, "fits (") {
		t.Fatalf("facts box still shows the fit mark:\n%s", view)
	}
}

func TestFactsBoxIsQuietWhenAutoTuneFits(t *testing.T) {
	a := newApp(context.Background(), nil, true)
	a.facts = source.Facts{
		ID:            "a/b",
		HardwareKnown: true,
		Analysis: models.Analysis{
			Recommended: models.RuntimeRecommendation{
				ContextWindow:      32768,
				NSeqMax:            1,
				CacheTypeK:         "f16",
				EstimatedVRAMBytes: 3 * 1 << 30,
				Fits:               true,
			},
		},
	}

	if view := a.factsView(); strings.Contains(view, "May not fully fit") {
		t.Fatalf("facts box warns although AutoTune fits:\n%s", view)
	}
}
