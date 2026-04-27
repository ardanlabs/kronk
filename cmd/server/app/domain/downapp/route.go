package downapp

import (
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
)

// Config contains all the mandatory systems required by handlers.
type Config struct {
	Log        *logger.Logger
	ModelsPath string
	Libs       *libs.Libs
}

// Routes registers the download surface. Two surfaces are registered:
//
//  1. Consumer-side endpoints under /v1/download/libs: list bundles
//     advertised by another Kronk server and stream a pull from that
//     peer into this server's libraries root.
//
//  2. Publisher-side endpoints under /download: serve model files and
//     library bundle zips to other Kronk servers (and
//     HuggingFace-compatible clients) on the local network.
//
// All routes are unauthenticated by design: when the operator opts in
// to downloads (gated upstream by the caller), the entire surface is
// open. Either you have full access to these endpoints or you don't.
func Routes(app *web.App, cfg Config) {
	const version = "v1"

	api := newApp(cfg)

	app.HandlerFunc(http.MethodGet, version, "/download/libs/peer-bundles", api.listPeerLibsBundles)
	app.HandlerFunc(http.MethodPost, version, "/download/libs/pull-from-peer", api.pullLibsFromPeer)

	// All publisher download requests funnel through the same catch-all
	// so that model and library-bundle paths can coexist without GET/HEAD
	// method pattern conflicts in the stdlib mux. The handler dispatches
	// based on the path prefix.
	app.RawHandlerFunc(http.MethodGet, "", "/download/{path...}", api.handle)
	app.RawHandlerFunc(http.MethodHead, "", "/download/{path...}", api.handle)
}
