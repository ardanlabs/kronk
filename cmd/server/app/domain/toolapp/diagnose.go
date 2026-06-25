package toolapp

import (
	"context"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/tools/diagnose"
)

// diagnose gathers the host diagnostic report used by the BUI Info screen. By
// default the benchmark step is skipped because it loads a model and would make
// this endpoint slow; inspect-only collection keeps the page responsive. The
// benchmark runs only when requested with "?bench=true" (the BUI's on-demand
// "Run benchmark" button), and an optional "?model=" selects which installed
// model to benchmark. The Kronk version reported is the release version
// (kronk.Version) so the BUI matches the CLI, not the server's ldflags build
// stamp (which is "develop" for unstamped builds).
func (a *app) diagnose(ctx context.Context, r *http.Request) web.Encoder {
	bench := r.URL.Query().Get("bench") == "true"
	model := r.URL.Query().Get("model")

	opts := []diagnose.Option{
		diagnose.WithKronkVersion(kronk.Version),
		diagnose.WithSkipBench(!bench),
	}
	if model != "" {
		opts = append(opts, diagnose.WithModelSource(model))
	}

	report, err := diagnose.Collect(ctx, a.log.Info, opts...)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return DiagnoseResponse(report)
}
