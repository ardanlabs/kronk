// Package debug provides handler support for the debugging endpoints.
package debug

import (
	"net/http"
	"net/http/pprof"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/arl/statsviz"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Mux registers all the debug routes from the standard library into a new mux
// bypassing the use of the DefaultServerMux. Using the DefaultServerMux would
// be a security risk since a dependency could inject a handler into our service
// without us knowing it.
func Mux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	// Kronk metrics live on a private registry inside the metrics package
	// (so SDK-only callers don't pollute the global default registry).
	// Expose that registry here instead of using promhttp.Handler(), which
	// would scrape the default registry and find nothing.
	mux.Handle("/metrics", promhttp.HandlerFor(metrics.Gatherer(), promhttp.HandlerOpts{}))

	statsviz.Register(mux)

	return mux
}
