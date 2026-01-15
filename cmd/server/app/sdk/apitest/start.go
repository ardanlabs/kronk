package apitest

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/api/services/kronk/build"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/authapp"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/cache"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mux"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
	"google.golang.org/grpc/test/bufconn"
)

// New initialized the system to run a test.
func New(t *testing.T, testName string) *Test {
	ctx := context.Background()

	// -------------------------------------------------------------------------

	var buf bytes.Buffer
	log := logger.New(&buf, logger.LevelInfo, "TEST", func(context.Context) string { return otel.GetTraceID(ctx) })

	// -------------------------------------------------------------------------

	traceProvider, teardown, err := otel.InitTracing(log, otel.Config{
		ServiceName: "kronk",
		Host:        "",
		ExcludedRoutes: map[string]struct{}{
			"/v1/liveness":  {},
			"/v1/readiness": {},
		},
		Probability: 0.05,
	})

	if err != nil {
		t.Fatal(err)
	}

	tracer := traceProvider.Tracer("kronk")

	// -------------------------------------------------------------------------

	auth, err := auth.New(auth.Config{
		KeyLookup: &keyStore{},
		Issuer:    "kronk project",
	})

	if err != nil {
		t.Fatal(err)
	}

	// -------------------------------------------------------------------------

	var authClientOpts []func(*authclient.Client)

	// If no host is provided for the auth service, we will start it ourselves
	// with a bufconn listener.
	sec, err := security.New(security.Config{
		Issuer: auth.Issuer(),
	})

	if err != nil {
		t.Fatal(err)
	}

	log.Info(ctx, "startup", "status", "starting auth server")

	lis := bufconn.Listen(1024 * 1024)

	authApp := authapp.Start(ctx, authapp.Config{
		Log:      log,
		Security: sec,
		Listener: lis,
		Tracer:   tracer,
		Enabled:  true,
	})

	authClientOpts = append(authClientOpts, authclient.WithDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.Dial()
	}))

	// -------------------------------------------------------------------------

	authHost := ""
	if len(authClientOpts) > 0 {
		authHost = "passthrough:///bufnet"
	}

	authClient, err := authclient.New(log, authHost, authClientOpts...)
	if err != nil {
		t.Fatal(err)
	}

	// -------------------------------------------------------------------------

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithVersion(defaults.LibVersion("")),
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := libs.Download(ctx, log.Info); err != nil {
		t.Fatal(err)
	}

	// -------------------------------------------------------------------------

	models, err := models.New()
	if err != nil {
		t.Fatal(err)
	}

	// -------------------------------------------------------------------------
	// Catalog System

	ctlg, err := catalog.New()
	if err != nil {
		t.Fatal(err)
	}

	if err := ctlg.Download(ctx, catalog.WithLogger(log.Info)); err != nil {
		t.Fatal(err)
	}

	// -------------------------------------------------------------------------
	// Template System

	tmplts, err := templates.New(templates.WithCatalog(ctlg))
	if err != nil {
		t.Fatal(err)
	}

	if err := tmplts.Download(ctx, templates.WithLogger(log.Info)); err != nil {
		t.Fatal(err)
	}

	// -------------------------------------------------------------------------
	// Init Kronk

	if err := kronk.Init(); err != nil {
		t.Fatal(err)
	}

	cache, err := cache.New(cache.Config{
		Log:             log.Info,
		Templates:       tmplts,
		ModelsInCache:   3,
		ModelInstances:  1,
		CacheTTL:        5 * time.Minute,
		ModelConfigFile: "../../../../../../zarf/kms/model_config.yaml",
	})

	if err != nil {
		t.Fatal(err)
	}

	// -------------------------------------------------------------------------

	t.Cleanup(func() {
		t.Helper()

		ctx := context.Background()

		if err := cache.Shutdown(ctx); err != nil {
			t.Fatal(err)
		}

		authClient.Close()
		authApp.Shutdown(ctx)
		sec.Close()
		teardown(context.Background())

		t.Logf("******************** LOGS (%s) ********************\n\n", testName)
		t.Log(buf.String())
		t.Logf("******************** LOGS (%s) ********************\n", testName)
	})

	// -------------------------------------------------------------------------

	cfgMux := mux.Config{
		Build:      "test",
		Log:        log,
		AuthClient: authClient,
		Tracer:     tracer,
		Cache:      cache,
		Libs:       libs,
		Models:     models,
		Catalog:    ctlg,
		Templates:  tmplts,
	}

	mux := mux.WebAPI(cfgMux,
		build.Routes(),
	)

	return &Test{
		Sec: sec,
		mux: mux,
	}
}
