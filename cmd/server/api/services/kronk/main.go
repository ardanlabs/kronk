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
	var log *logger.Logger

	events := logger.Events{
		Error: func(ctx context.Context, r logger.Record) {
			log.Info(ctx, "******* SEND ALERT *******")
		},
	}

	log = logger.NewWithEvents(os.Stdout, logger.LevelInfo, "KRONK", web.GetTraceID, events)

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

	// +-------------------+--------------------+--------------------------+---------------------------------------------------------------+
	// | WEB_ADMIN_ENABLED | AUTH_ADMIN_ENABLED | AUTH_LOCAL_ENABLED       | Effective mode                                                |
	// +-------------------+--------------------+--------------------------+---------------------------------------------------------------+
	// | false             | false              | false                    | Headless; inference is open                                   |
	// | true              | false              | false                    | BUI without login; inference is open                          |
	// | true              | true               | false                    | BUI login; management protected; inference is open            |
	// | true              | implied true       | true                     | BUI login; management and inference are protected             |
	// | false             | implied true       | true                     | Headless; management and inference are protected              |
	// +-------------------+--------------------+--------------------------+---------------------------------------------------------------+

	// Notes:
	// - WEB_ADMIN_ENABLED  controls whether the BUI is served under /admin/.
	// - AUTH_ADMIN_ENABLED protects BUI login and management, playground, tool, and security endpoints.
	// - AUTH_LOCAL_ENABLED protects inference endpoints and automatically enables admin authentication.

	cfg := struct {
		conf.Version
		Web struct {
			ReadTimeout        time.Duration `conf:"default:30s"`
			WriteTimeout       time.Duration `conf:"default:61m"`
			InferenceTimeout   time.Duration `conf:"default:60m"`
			IdleTimeout        time.Duration `conf:"default:1m"`
			ShutdownTimeout    time.Duration `conf:"default:1m"`
			APIHost            string        `conf:"default:0.0.0.0:11435"`
			DebugHost          string        `conf:"default:0.0.0.0:11445"`
			CORSAllowedOrigins []string      `conf:"default:*"`
			Admin              struct {
				Enabled        bool   `conf:"default:true"`
				PasswordSHA256 string `conf:"default:18511e63760230cd17291273b607e7e13da2a2bb9a1750e0becdac08185a3c11,mask"` // kronk
			}
		}
		Auth struct {
			Host         string // Leave empty to run the local auth service.
			AdminEnabled bool   `conf:"default:false"`
			TLS          struct {
				Enabled    bool `conf:"default:false"`
				CAFile     string
				ServerName string
			}
			Local struct {
				Issuer  string `conf:"default:kronk project"`
				Enabled bool   `conf:"default:false"`
			}
		}
		MCP struct {
			Enabled     bool   `conf:"default:true"`
			Host        string // Leave empty to run the local MCP service.
			AuthEnabled bool   `conf:"default:false"`
			BraveAPIKey string `conf:"mask"`
		}
		Download struct {
			Enabled bool `conf:"default:false"`
		}
		Tempo struct {
			Host        string  `conf:"default:localhost:4317"`
			ServiceName string  `conf:"default:kronk"`
			Probability float64 `conf:"default:0.25"`
			// Shouldn't use a high Probability value in non-developer systems.
			// 25% should be enough for most systems. Some might want to have
			// this even lower.
		}
		Pool struct {
			ModelConfigFile string
			BudgetPercent   int           `conf:"default:95"`
			ModelsInPool    int           `conf:"default:10"`
			TTL             time.Duration `conf:"default:20m"`
		}
		BasePath        string
		LibPath         string
		LibVersion      string
		Arch            string
		OS              string
		Processor       string
		AllowUpgrade    bool
		InsecureLogging bool
		HfToken         string `conf:"mask"`
		LlamaLog        int    `conf:"default:1"`
	}{
		Version: conf.Version{
			Build: tag,
			Desc:  "Kronk",
		},
	}

	const prefix = "KRONK"
	if showHelp {
		help, err := conf.UsageInfo(prefix, &cfg)
		if err != nil {
			return fmt.Errorf("parsing config: %w", err)
		}
		return fmt.Errorf("%s", help)
	}

	help, err := conf.Parse(prefix, &cfg)
	if err != nil {
		if errors.Is(err, conf.ErrHelpWanted) {
			fmt.Println(help)
		}
		return fmt.Errorf("parsing config: %w", err)
	}
	mcpAuthEnabled := cfg.MCP.Enabled && cfg.MCP.Host == "" && cfg.MCP.AuthEnabled
	cfg.Auth.AdminEnabled = cfg.Auth.AdminEnabled || cfg.Auth.Local.Enabled || mcpAuthEnabled
	if err := validateAdminConfig(cfg.Auth.AdminEnabled, cfg.Web.Admin.Enabled, cfg.Web.Admin.PasswordSHA256, cfg.Auth.Host); err != nil {
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
			Enabled:          cfg.Auth.Local.Enabled,
			AdminAuthEnabled: cfg.Auth.AdminEnabled,
		})

		defer authApp.Shutdown(ctx)

		authClientOpts = append(authClientOpts, authclient.WithDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.Dial()
		}))
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

	log.Info(ctx, "startup", "status", "downloading libraries")

	libs, err := libs.New(
		libs.WithDetectOverrides(ctx, log.Info, cfg.LibPath, cfg.Arch, cfg.OS, cfg.Processor),
		libs.WithBasePath(cfg.BasePath),
		libs.WithAllowUpgrade(cfg.AllowUpgrade),
		libs.WithVersion(defaults.LibVersion(cfg.LibVersion)),
	)
	if err != nil {
		return fmt.Errorf("unable to create libs api: %w", err)
	}

	log.Info(ctx, "startup", "status", "installing/updating libraries", "libPath", libs.LibsPath(), "arch", libs.Arch(), "os", libs.OS(), "processor", libs.Processor(), "update", true)

	downloadCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	if _, err := libs.Download(downloadCtx, log.Info); err != nil {
		log.Info(ctx, "startup", "WARNING", "unable to install llama.cpp, running in degraded mode", "ERROR", err)
	}

	// Capability fallback applies only to automatic processor selection. An
	// explicit processor or library path remains a strict user choice.
	if cfg.Processor == "" && cfg.LibPath == "" {
		selected, decision, err := libs.SelectInstalledRuntime(ctx, log.Info)
		if err != nil {
			log.Info(ctx, "startup", "WARNING", "unable to select installed llama.cpp runtime", "ERROR", err)
		} else {
			libs = selected
			log.Info(ctx, "startup", "status", "selected llama.cpp runtime", "preferred", decision.PreferredProcessor, "selected", decision.SelectedProcessor, "reason", decision.Reason)
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
		buckylibs.WithLibPath(os.Getenv("KRONK_BUCKY_LIB_PATH")),
		buckylibs.WithAllowUpgrade(cfg.AllowUpgrade),
		buckylibs.WithDetect(ctx, log.Info),
	)
	if err != nil {
		return fmt.Errorf("unable to create bucky libs api: %w", err)
	}

	log.Info(ctx, "startup", "status", "bucky libs ready", "libPath", buckyLibs.LibsPath(), "arch", buckyLibs.Arch(), "os", buckyLibs.OS(), "processor", buckyLibs.Processor())

	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	if _, err := buckyLibs.Download(ctx, log.Info); err != nil {
		log.Info(ctx, "startup", "WARNING", "unable to install whisper.cpp, running in degraded mode", "ERROR", err)
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

	modelConfigFile, err := defaults.ModelConfigFile(cfg.Pool.ModelConfigFile, cfg.BasePath)
	if err != nil {
		return fmt.Errorf("resolving model config file: %w", err)
	}

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
	if err := kronk.Init(kronk.WithLibPath(libs.LibsPath())); err != nil {
		log.Info(ctx, "startup", "WARNING", "kronk init failed, running in degraded mode (use BUI to download libraries)", "ERROR", err)
	}

	if err := bucky.Init(bucky.WithLibPath(buckyLibs.LibsPath())); err != nil {
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
		Libs:                libs,
		Models:              models,
		BuckyLibs:           buckyLibs,
		BuckyModels:         buckyModels,
		DownloadEnabled:     cfg.Download.Enabled,
		AdminAuthEnabled:    cfg.Auth.AdminEnabled,
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
				ModelID:   session.ModelID,
				Entry:     session.ID,
				State:     string(session.State),
				Messages:  session.Messages,
				Context:   session.Context,
				Allocated: session.Allocated,
				Window:    session.ContextWindow,
				HasMedia:  session.HasMedia,
				LastUsed:  session.LastUsed,
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
