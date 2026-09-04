// Package kronk is the model server.
package kronk

import (
	"context"
	"embed"
	"errors"
	"expvar"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/ardanlabs/conf/v3"
	"github.com/ardanlabs/kronk/cmd/server/api/services/kronk/build"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/authapp"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/mcpapp"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/debug"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mux"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/ardanlabs/kronk/sdk/pool"
	buckylibs "github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"google.golang.org/grpc/test/bufconn"
)

//go:embed static
var static embed.FS

var tag = "develop"

func Run(showHelp bool) error {
	log := logger.New(os.Stdout, logger.LevelInfo, "KRONK", web.GetTraceID)

	// -------------------------------------------------------------------------

	ctx := context.Background()

	if err := run(ctx, log, showHelp); err != nil {
		return err
	}

	return nil
}

func run(ctx context.Context, log *logger.Logger, showHelp bool) error {

	// -------------------------------------------------------------------------
	// GOMAXPROCS

	if !showHelp {
		log.Info(ctx, "startup", "GOMAXPROCS", runtime.GOMAXPROCS(0))
	}

	// -------------------------------------------------------------------------
	// Configuration

	// AUTHORIZATION_MODE provides the explicit API access policy:
	// - open: discovery, inference, and management are open.
	// - management: management requires an administrator; discovery and inference are open.
	// - authenticated: discovery and inference require a valid JWT; management requires an administrator.
	// - full-protected: inference requires endpoint grants, discovery requires a valid JWT, and management requires an administrator.
	//
	// When AUTHORIZATION_MODE is set, it overrides AUTH_LOCAL_ENABLED and
	// AUTH_ADMIN_ENABLED. When it is unset, the legacy settings retain their
	// existing behavior. WEB_ADMIN_ENABLED independently controls whether the
	// BUI is served under /admin/.

	cfg, err := loadConfig(showHelp)
	if err != nil {
		return err
	}
	mcpAuthEnabled := cfg.MCP.Enabled && cfg.MCP.Host == "" && cfg.MCP.AuthEnabled
	inferenceAuthEnabled, managementAuthEnabled, authServiceAdminEnabled := resolveAuthorizationSettings(
		cfg.Authorization.Mode,
		cfg.Auth.Local.Enabled,
		cfg.Auth.AdminEnabled,
		mcpAuthEnabled,
	)
	if err := validateAdminConfig(managementAuthEnabled, cfg.Web.Admin.Enabled, cfg.Web.Admin.PasswordSHA256, cfg.Auth.Host); err != nil {
		return err
	}
	if err := validateTimeoutConfig(cfg.Web.InferenceTimeout, cfg.Web.WriteTimeout); err != nil {
		return err
	}

	// -------------------------------------------------------------------------
	// App Starting

	log.Info(ctx, "starting service", "version", cfg.Build)
	defer log.Info(ctx, "shutdown complete")

	out, err := conf.String(&cfg)
	if err != nil {
		return fmt.Errorf("generating config for output: %w", err)
	}
	log.Info(ctx, "startup", "config", out)

	log.BuildInfo(ctx)

	expvar.NewString("build").Set(cfg.Build)

	fmt.Println(logo)

	// -------------------------------------------------------------------------
	// Start Tracing Support

	log.Info(ctx, "startup", "status", "initializing tracing support")

	traceProvider, teardown, err := otel.InitTracing(log.Info, otel.Config{
		ServiceName: cfg.Tempo.ServiceName,
		Host:        cfg.Tempo.Host,
		ExcludedRoutes: map[string]struct{}{
			"/v1/liveness":  {},
			"/v1/readiness": {},
		},
		Probability: cfg.Tempo.Probability,
	})

	if err != nil {
		return fmt.Errorf("starting tracing: %w", err)
	}

	defer func() {
		log.Info(ctx, "shutdown", "status", "teardown otel")
		teardown(context.Background())
	}()

	tracer := traceProvider.Tracer(cfg.Tempo.ServiceName)

	// -------------------------------------------------------------------------
	// Start the auth server

	var authClientOpts []func(*authclient.Client)
	var sec *security.Security

	// If no host is provided for the auth service, we will start it ourselves
	// with a bufconn listener.
	if cfg.Auth.Host == "" {
		sec, err = security.New(security.Config{
			Issuer: cfg.Auth.Local.Issuer,
		})

		if err != nil {
			return fmt.Errorf("unable to initialize security system: %w", err)
		}

		defer sec.Close()

		log.Info(ctx, "startup", "status", "starting auth server")

		lis := bufconn.Listen(1024 * 1024)

		authApp := authapp.Start(ctx, authapp.Config{
			Log:              log,
			Security:         sec,
			Listener:         lis,
			Tracer:           tracer,
			Enabled:          inferenceAuthEnabled,
			AdminAuthEnabled: authServiceAdminEnabled,
		})

		defer authApp.Shutdown(ctx)

		authClientOpts = append(authClientOpts,
			authclient.WithLocalAuth(inferenceAuthEnabled, authServiceAdminEnabled),
			authclient.WithDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return lis.Dial()
			}),
		)
	}

	// -------------------------------------------------------------------------
	// Initialize Auth Client

	log.Info(ctx, "startup", "status", "initializing authentication client")

	authHost := cfg.Auth.Host
	if len(authClientOpts) > 0 {
		authHost = "passthrough:///bufnet"
	}
	if cfg.Auth.Host != "" {
		if cfg.Auth.TLS.Enabled {
			creds, err := authclient.TLSCredentials(cfg.Auth.TLS.CAFile, cfg.Auth.TLS.ServerName)
			if err != nil {
				return fmt.Errorf("failed to initialize authentication TLS: %w", err)
			}
			authClientOpts = append(authClientOpts, authclient.WithTransportCredentials(creds))
		} else {
			log.Warn(ctx, "startup", "status", "external authentication uses plaintext transport", "host", cfg.Auth.Host)
		}
	} else if cfg.Auth.TLS.Enabled {
		return errors.New("configuration: auth TLS requires an external auth host")
	}

	authClient, err := authclient.New(log, authHost, authClientOpts...)
	if err != nil {
		return fmt.Errorf("failed to initialize authentication client: %w", err)
	}

	defer authClient.Close()

	// -------------------------------------------------------------------------
	// Library System

	libVersion := defaults.LibVersion(cfg.LibVersion)
	llamaLibs, err := libs.New(
		libs.WithDetectOverrides(ctx, log.Info, cfg.LibPath, cfg.Arch, cfg.OS, cfg.Processor),
		libs.WithBasePath(cfg.BasePath),
		libs.WithAllowUpgrade(cfg.AllowUpgrade),
		libs.WithValidation(cfg.LibVerifyEnabled),
		libs.WithVersion(libVersion),
	)
	if err != nil {
		return fmt.Errorf("unable to create libs api: %w", err)
	}

	libVerified := !cfg.LibVerifyEnabled
	if cfg.LibDownloadEnabled {
		log.Info(ctx, "startup", "status", "installing/updating libraries", "libPath", llamaLibs.LibsPath(), "arch", llamaLibs.Arch(), "os", llamaLibs.OS(), "processor", llamaLibs.Processor(), "update", true)

		downloadCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		_, err := llamaLibs.Download(downloadCtx, log.Info)
		cancel()
		if err != nil {
			log.Info(ctx, "startup", "WARNING", "unable to install llama.cpp, running in degraded mode", "ERROR", err)
		} else {
			libVerified = true
		}
	} else {
		log.Info(ctx, "startup", "status", "automatic llama.cpp library download disabled", "libPath", llamaLibs.LibsPath())
	}

	// Capability fallback applies only to automatic processor selection. An
	// explicit processor or library path remains a strict user choice. When
	// verification is enabled, every bundle is checked before its executable
	// device probe runs.
	if cfg.Processor == "" && cfg.LibPath == "" {
		var selected *libs.Libs
		var decision libs.RuntimeSelection
		var err error
		if cfg.LibVerifyEnabled {
			verifyCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
			selected, decision, err = llamaLibs.SelectInstalledRuntime(verifyCtx, log.Info)
			cancel()
		} else {
			selected, decision, err = llamaLibs.SelectInstalledRuntime(ctx, log.Info)
		}
		if err != nil {
			status := "unable to select installed llama.cpp runtime"
			if cfg.LibVerifyEnabled {
				status = "unable to select and verify installed llama.cpp runtime"
			}
			log.Info(ctx, "startup", "WARNING", status, "ERROR", err)
		} else {
			llamaLibs = selected
			libVerified = true
			log.Info(ctx, "startup", "status", "selected llama.cpp runtime", "preferred", decision.PreferredProcessor, "selected", decision.SelectedProcessor, "reason", decision.Reason)
		}
	} else if cfg.LibVerifyEnabled && !libVerified {
		// Download normally validates the selected bundle through
		// WithValidation. Verify directly only when Download was disabled or
		// failed and the explicit runtime bypassed selection-time validation.
		verifyCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		report, err := llamaLibs.Verify(verifyCtx, libVersion)
		cancel()
		if err != nil {
			log.Info(ctx, "startup", "WARNING", "unable to verify selected llama.cpp runtime", "ERROR", err)
		} else if !report.OK() {
			log.Info(ctx, "startup", "WARNING", "selected llama.cpp runtime failed verification", "changed", report.Changed, "missing", report.Missing)
		} else {
			libVerified = true
			log.Info(ctx, "startup", "status", "verified llama.cpp runtime", "version", report.Tag, "files", report.Verified, "unexpected", report.Unexpected)
		}
	}

	// -------------------------------------------------------------------------
	// Model System

	models, err := models.NewWithPaths(cfg.BasePath)
	if err != nil {
		return fmt.Errorf("unable to create catalog system: %w", err)
	}

	log.Info(ctx, "startup", "status", "model integrity checks, may take a few seconds")

	models.BuildIndex(log.Info, false)

	if err := models.ReconcileCatalog(ctx, log.Info); err != nil {
		log.Info(ctx, "startup", "WARNING", "reconcile catalog", "ERROR", err)
	}

	// -------------------------------------------------------------------------
	// Bucky (whisper) Libs + Models
	//
	// The server exposes both the /v1/bucky/* admin endpoints (library
	// installs + downloaded whisper model management) and the
	// /v1/audio/transcriptions inference endpoint. bucky.Init wires up
	// the whisper.cpp shared library so the bucky pool can load
	// models on demand; failure is non-fatal so the admin endpoints
	// still work when the runtime library has not been downloaded yet.

	buckyLibs, err := buckylibs.New(
		buckylibs.WithBasePath(cfg.BasePath),
		buckylibs.WithLibPath(cfg.BuckyLibPath),
		buckylibs.WithAllowUpgrade(cfg.AllowUpgrade),
		buckylibs.WithValidation(cfg.LibVerifyEnabled),
		buckylibs.WithDetect(ctx, log.Info),
	)
	if err != nil {
		return fmt.Errorf("unable to create bucky libs api: %w", err)
	}

	log.Info(ctx, "startup", "status", "bucky libs ready", "libPath", buckyLibs.LibsPath(), "arch", buckyLibs.Arch(), "os", buckyLibs.OS(), "processor", buckyLibs.Processor())

	buckyLibVerified := !cfg.LibVerifyEnabled
	downloadCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	buckyVersion, err := buckyLibs.Download(downloadCtx, log.Info)
	cancel()
	if err != nil {
		log.Info(ctx, "startup", "WARNING", "unable to install whisper.cpp, running in degraded mode", "ERROR", err)
	} else {
		buckyLibVerified = true
		if cfg.LibVerifyEnabled {
			log.Info(ctx, "startup", "status", "verified whisper.cpp runtime", "version", buckyVersion.Version)
		}
	}

	if cfg.LibVerifyEnabled && !buckyLibVerified {
		// Download normally validates the selected bundle through
		// WithValidation. Unlike llama.cpp, Bucky always attempts Download;
		// verify directly only when that attempt failed and an older bundle
		// may still be usable.
		verifyCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		report, err := buckyLibs.Verify(verifyCtx, "")
		cancel()
		if err != nil {
			log.Info(ctx, "startup", "WARNING", "unable to verify selected whisper.cpp runtime", "ERROR", err)
		} else if !report.OK() {
			log.Info(ctx, "startup", "WARNING", "selected whisper.cpp runtime failed verification", "changed", report.Changed, "missing", report.Missing, "unexpected", report.Unexpected)
		} else {
			buckyLibVerified = true
			log.Info(ctx, "startup", "status", "verified whisper.cpp runtime", "version", report.Tag, "files", report.Verified, "manifestAuthenticated", report.ManifestAuthenticated)
		}
	}

	buckyModels, err := buckymodels.NewWithPaths(cfg.BasePath)
	if err != nil {
		return fmt.Errorf("unable to create bucky models api: %w", err)
	}

	if err := buckyModels.BuildIndex(log.Info, false); err != nil {
		log.Info(ctx, "startup", "WARNING", "bucky build index", "ERROR", err)
	}

	// -------------------------------------------------------------------------
	// Model Config

	modelConfigFile := cfg.Pool.ModelConfigFile

	log.Info(ctx, "startup", "status", "model config", "path", modelConfigFile)

	// -------------------------------------------------------------------------
	// Jinja Templates
	//
	// Seed the embedded chat templates to disk on every start so fixes
	// shipped in the binary always replace older on-disk copies.

	if err := defaults.WriteJinjaFiles("", cfg.BasePath); err != nil {
		return fmt.Errorf("writing jinja files: %w", err)
	}

	log.Info(ctx, "startup", "status", "jinja templates", "path", filepath.Join(defaults.BaseDir(cfg.BasePath), "jinja"))

	// -------------------------------------------------------------------------
	// Init Kronk

	log.Info(ctx, "startup", "status", "initializing kronk")

	// The server may use a base path that differs from the process default.
	// Initialize from the manager's resolved path so that base-path and
	// library-path configuration remain authoritative.
	llamaLogLevel := kronk.LogSilent
	if cfg.LlamaLog == 1 {
		llamaLogLevel = kronk.LogNormal
	}

	if !libVerified {
		log.Info(ctx, "startup", "WARNING", "kronk init skipped because llama.cpp runtime verification did not succeed")
	} else if err := kronk.Init(kronk.WithLibPath(llamaLibs.LibsPath()), kronk.WithLogLevel(llamaLogLevel)); err != nil {
		log.Info(ctx, "startup", "WARNING", "kronk init failed, running in degraded mode (use BUI to download libraries)", "ERROR", err)
	}

	if !buckyLibVerified {
		log.Info(ctx, "startup", "WARNING", "bucky init skipped because whisper.cpp runtime verification did not succeed")
	} else if err := bucky.Init(bucky.WithLibPath(buckyLibs.LibsPath())); err != nil {
		log.Info(ctx, "startup", "WARNING", "bucky init failed, running in degraded mode (use BUI to download whisper libraries)", "ERROR", err)
	}

	// -------------------------------------------------------------------------
	// Pool
	//
	// One call to pool.New constructs the shared resource manager and
	// every enabled backend pool (kronk + bucky). The resman is
	// shared so VRAM/RAM budgeting is unified across backends.

	p, err := pool.New(pool.Config{
		Log:             log.Info,
		KronkModels:     models,
		BuckyModels:     buckyModels,
		ModelConfigFile: modelConfigFile,
		BudgetPercent:   cfg.Pool.BudgetPercent,
		ModelsInPool:    cfg.Pool.ModelsInPool,
		TTL:             cfg.Pool.TTL,
		InsecureLogging: cfg.InsecureLogging,
	})
	if err != nil {
		return fmt.Errorf("initializing pool: %w", err)
	}

	defer func() {
		log.Info(ctx, "shutdown", "status", "shutting down pool")

		ctx, cancel := context.WithTimeout(context.Background(), cfg.Web.ShutdownTimeout)
		defer cancel()

		if err := p.Shutdown(ctx); err != nil {
			log.Error(ctx, "pool", "ERROR", err)
		}
	}()

	unregisterIMCMetrics, err := registerIMCSessionMetrics(p)
	if err != nil {
		return fmt.Errorf("registering IMC session metrics: %w", err)
	}
	defer unregisterIMCMetrics()

	// -------------------------------------------------------------------------
	// Start the MCP server

	if cfg.MCP.Enabled && cfg.MCP.Host == "" {
		log.Info(ctx, "startup", "status", "starting local mcp server")

		mcpLis, err := net.Listen("tcp", "localhost:9000")
		if err != nil {
			return fmt.Errorf("failed to listen for mcp: %w", err)
		}

		var authenticate func(context.Context, string) error
		if mcpAuthEnabled {
			authenticate = func(ctx context.Context, bearerToken string) error {
				_, err := authClient.AuthenticateRequired(ctx, bearerToken, true, "")
				return err
			}
		}

		mcpApp := mcpapp.Start(ctx, mcpapp.Config{
			Log:          log,
			Listener:     mcpLis,
			BraveAPIKey:  cfg.MCP.BraveAPIKey,
			Authenticate: authenticate,
		})

		defer mcpApp.Shutdown(ctx)

		log.Info(ctx, "startup", "status", "local mcp server started", "host", mcpLis.Addr().String())
	}

	// -------------------------------------------------------------------------
	// Start Debug Service

	go func() {
		log.Info(ctx, "startup", "status", "debug v1 router started", "host", cfg.Web.DebugHost)

		if err := http.ListenAndServe(cfg.Web.DebugHost, debug.Mux()); err != nil {
			log.Error(ctx, "shutdown", "status", "debug v1 router closed", "host", cfg.Web.DebugHost, "msg", err)
		}
	}()

	// -------------------------------------------------------------------------
	// Start API Service

	log.Info(ctx, "startup", "status", "initializing V1 API support")

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	cfgMux := mux.Config{
		Build:               tag,
		Log:                 log,
		AuthClient:          authClient,
		Pool:                p,
		Libs:                llamaLibs,
		LibVersion:          libVersion,
		LibVerifyEnabled:    cfg.LibVerifyEnabled,
		Models:              models,
		BuckyLibs:           buckyLibs,
		BuckyModels:         buckyModels,
		DownloadEnabled:     cfg.Download.Enabled,
		AuthorizationMode:   cfg.Authorization.Mode,
		AdminAuthEnabled:    managementAuthEnabled,
		WebAdminEnabled:     cfg.Web.Admin.Enabled,
		AdminPasswordSHA256: cfg.Web.Admin.PasswordSHA256,
		Security:            sec,
		InferenceTimeout:    cfg.Web.InferenceTimeout,
	}

	options := []func(*mux.Options){mux.WithCORS(cfg.Web.CORSAllowedOrigins)}
	if cfg.Web.Admin.Enabled {
		options = append(options, mux.WithFileServer(true, static, "static", "/admin/", []string{"admin/api", "v1"}))
	}
	webAPI := mux.WebAPI(cfgMux, build.Routes(), options...)

	api := http.Server{
		Addr:         cfg.Web.APIHost,
		Handler:      webAPI,
		ReadTimeout:  cfg.Web.ReadTimeout,
		WriteTimeout: cfg.Web.WriteTimeout,
		IdleTimeout:  cfg.Web.IdleTimeout,
		ErrorLog:     logger.NewStdLogger(log, logger.LevelError),
	}

	serverErrors := make(chan error, 1)

	go func() {
		log.Info(ctx, "startup", "status", "api router started", "host", api.Addr)

		serverErrors <- api.ListenAndServe()
	}()

	// -------------------------------------------------------------------------
	// Shutdown

	select {
	case err := <-serverErrors:
		return fmt.Errorf("server error: %w", err)

	case sig := <-shutdown:
		log.Info(ctx, "shutdown", "status", "shutdown started", "signal", sig)

		ctx, cancel := context.WithTimeout(ctx, cfg.Web.ShutdownTimeout)
		defer cancel()

		if err := api.Shutdown(ctx); err != nil {
			api.Close()
			return fmt.Errorf("could not stop server gracefully: %w", err)
		}

		log.Info(ctx, "shutdown", "status", "shutdown complete", "signal", sig)
	}

	return nil
}

func registerIMCSessionMetrics(p *pool.Pool) (func(), error) {
	return metrics.RegisterIMCSessionsProvider(func() []metrics.IMCSession {
		sessions := p.Kronk.IMCSessions()
		details := make([]metrics.IMCSession, len(sessions))

		for i, session := range sessions {
			details[i] = metrics.IMCSession{
				ModelID:       session.ModelID,
				Entry:         session.ID,
				State:         string(session.State),
				Messages:      session.Messages,
				Context:       session.Context,
				Allocated:     session.Allocated,
				InputMessages: session.InputMessages,
				InputTokens:   session.InputTokens,
				OutputTokens:  session.OutputTokens,
				PeakContext:   session.PeakContext,
				Window:        session.ContextWindow,
				HasMedia:      session.HasMedia,
				LastUsed:      session.LastUsed,
			}
		}

		return details
	})
}

var logo = `
██╗  ██╗██████╗  ██████╗ ███╗   ██╗██╗  ██╗    ███╗   ███╗███████╗
██║ ██╔╝██╔══██╗██╔═══██╗████╗  ██║██║ ██╔╝    ████╗ ████║██╔════╝
█████╔╝ ██████╔╝██║   ██║██╔██╗ ██║█████╔╝     ██╔████╔██║███████╗
██╔═██╗ ██╔══██╗██║   ██║██║╚██╗██║██╔═██╗     ██║╚██╔╝██║╚════██║
██║  ██╗██║  ██║╚██████╔╝██║ ╚████║██║  ██╗    ██║ ╚═╝ ██║███████║
╚═╝  ╚═╝╚═╝  ╚═╝ ╚═════╝ ╚═╝  ╚═══╝╚═╝  ╚═╝    ╚═╝     ╚═╝╚══════╝                                                                                         
`
