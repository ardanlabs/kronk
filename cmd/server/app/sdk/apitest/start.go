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
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mux"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/ardanlabs/kronk/sdk/pool"
	buckylibs "github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"google.golang.org/grpc/test/bufconn"
)

// New initialized the system to run a test.
func New(t *testing.T, testName string) *Test {
	ctx := context.Background()

	// -------------------------------------------------------------------------

	var buf bytes.Buffer
	log := logger.New(&buf, logger.LevelInfo, "TEST", web.GetTraceID)

	// -------------------------------------------------------------------------

	traceProvider, teardown, err := otel.InitTracing(log.Info, otel.Config{
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

	if err := models.BuildIndex(log.Info, false); err != nil {
		t.Fatal(err)
	}

	// -------------------------------------------------------------------------
	// Bucky (whisper) Libs + Models
	//
	// Mirrors the production wiring in cmd/server/api/services/kronk/main.go
	// so the /v1/audio/transcriptions endpoint is reachable in tests
	// when the whisper library and a whisper model (e.g. tiny.en)
	// have been installed under ~/.kronk.

	buckyLibs, err := buckylibs.New()
	if err != nil {
		t.Fatal(err)
	}

	buckyModels, err := buckymodels.New()
	if err != nil {
		t.Fatal(err)
	}

	if err := buckyModels.BuildIndex(log.Info, false); err != nil {
		t.Fatal(err)
	}

	// -------------------------------------------------------------------------
	// Init Kronk + Bucky

	if err := kronk.Init(); err != nil {
		t.Fatal(err)
	}

	if err := bucky.Init(bucky.WithInitLibPath(buckyLibs.LibsPath())); err != nil {
		log.Info(ctx, "startup", "WARNING", "bucky init failed, audio transcription tests will fail", "ERROR", err)
	}

	p, err := pool.New(pool.Config{
		Log:             log.Info,
		KronkModels:     models,
		BuckyModels:     buckyModels,
		ModelConfigFile: "../../../../../../zarf/kms/model_config.yaml",
		BudgetPercent:   95,
		ModelsInPool:    1,
		TTL:             5 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	// -------------------------------------------------------------------------

	t.Cleanup(func() {
		t.Helper()

		ctx := context.Background()

		if err := p.Shutdown(ctx); err != nil {
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
		Build:       "test",
		Log:         log,
		AuthClient:  authClient,
		Pool:        p,
		Libs:        libs,
		Models:      models,
		BuckyLibs:   buckyLibs,
		BuckyModels: buckyModels,
	}

	mux := mux.WebAPI(cfgMux,
		build.Routes(),
	)

	return &Test{
		Sec: sec,
		mux: mux,
	}
}
