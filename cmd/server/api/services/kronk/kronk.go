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
	"runtime"
	"syscall"
	"time"

	"github.com/ardanlabs/conf/v3"
	"github.com/ardanlabs/kronk/cmd/server/api/services/kronk/build"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/authapp"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/authclient"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/debug"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mux"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/otel"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/cache"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/security"
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

	traceIDFn := func(ctx context.Context) string {
		return otel.GetTraceID(ctx)
	}

	log = logger.NewWithEvents(os.Stdout, logger.LevelInfo, "SALES", traceIDFn, events)

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

	cfg := struct {
		conf.Version
		Web struct {
			ReadTimeout        time.Duration `conf:"default:30s"`
			WriteTimeout       time.Duration `conf:"default:15m"`
			IdleTimeout        time.Duration `conf:"default:1m"`
			ShutdownTimeout    time.Duration `conf:"default:1m"`
			APIHost            string        `conf:"default:0.0.0.0:8080"`
			DebugHost          string        `conf:"default:0.0.0.0:8090"`
			CORSAllowedOrigins []string      `conf:"default:*"`
		}
		Auth struct {
			Host  string // Leave empty to run the local auth service.
			Local struct {
				Issuer  string `conf:"default:kronk project"`
				Enabled bool   `conf:"default:false"`
			}
		}
		Tempo struct {
			Host        string  // `conf:"default:tempo:4317"`
			ServiceName string  `conf:"default:sales"`
			Probability float64 `conf:"default:0.05"`
			// Shouldn't use a high Probability value in non-developer systems.
			// 0.05 should be enough for most systems. Some might want to have
			// this even lower.
		}
		Model struct {
			Path          string
			Device        string
			MaxInstances  int           `conf:"default:1"`
			MaxInCache    int           `conf:"default:3"`
			ContextWindow int           `conf:"default:0"`
			CacheTTL      time.Duration `conf:"default:5m"`
		}
		LibPath      string
		Arch         string
		OS           string
		Processor    string
		AllowUpgrade bool `conf:"default:true"`
		LlamaLog     int  `conf:"default:1"`
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

	traceProvider, teardown, err := otel.InitTracing(log, otel.Config{
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

	// If no host is provided for the auth service, we will start it ourselves
	// with a bufconn listener.
	if cfg.Auth.Host == "" {
		sec, err := security.New(security.Config{
			Issuer: cfg.Auth.Local.Issuer,
		})

		if err != nil {
			return fmt.Errorf("unable to initialize security system: %w", err)
		}

		defer sec.Close()

		log.Info(ctx, "startup", "status", "starting auth server")

		lis := bufconn.Listen(1024 * 1024)

		authApp := authapp.Start(ctx, authapp.Config{
			Log:      log,
			Security: sec,
			Listener: lis,
			Tracer:   tracer,
			Enabled:  cfg.Auth.Local.Enabled,
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

	authClient, err := authclient.New(log, authHost, authClientOpts...)
	if err != nil {
		return fmt.Errorf("failed to initialize authentication client: %w", err)
	}

	defer authClient.Close()

	// -------------------------------------------------------------------------
	// Init Kronk

	log.Info(ctx, "startup", "status", "initializing kronk")

	libCfg, err := libs.NewConfig(
		cfg.LibPath,
		cfg.Arch,
		cfg.OS,
		cfg.Processor,
		cfg.LlamaLog,
		cfg.AllowUpgrade,
	)

	if err != nil {
		return err
	}

	log.Info(ctx, "startup", "status", "installing/updating libraries", "libPath", libCfg.LibPath, "arch", libCfg.Arch, "os", libCfg.OS, "processor", libCfg.Processor, "update", libCfg.AllowUpgrade)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if _, err := libs.Download(ctx, log.Info, libCfg); err != nil {
		return fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	if err := kronk.Init(libCfg.LibPath, libCfg.LlamaLog); err != nil {
		return fmt.Errorf("installation invalid: %w", err)
	}

	cache, err := cache.NewCache(cache.Config{
		Log:            log,
		LibPath:        libCfg.LibPath,
		Arch:           libCfg.Arch,
		OS:             libCfg.OS,
		Processor:      libCfg.Processor,
		ModelPath:      cfg.Model.Path,
		Device:         cfg.Model.Device,
		MaxInCache:     cfg.Model.MaxInCache,
		ModelInstances: cfg.Model.MaxInstances,
		ContextWindow:  cfg.Model.ContextWindow,
		CacheTTL:       cfg.Model.CacheTTL,
	})

	if err != nil {
		return fmt.Errorf("initializing kronk manager: %w", err)
	}

	defer func() {
		log.Info(ctx, "shutdown", "status", "shutting down kronk")

		ctx, cancel := context.WithTimeout(context.Background(), cfg.Web.ShutdownTimeout)
		defer cancel()

		if err := cache.Shutdown(ctx); err != nil {
			log.Error(ctx, "kronk manager", "ERROR", err)
		}
	}()

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
		Build:      tag,
		Log:        log,
		AuthClient: authClient,
		Tracer:     tracer,
		Cache:      cache,
	}

	webAPI := mux.WebAPI(cfgMux,
		build.Routes(),
		mux.WithCORS(cfg.Web.CORSAllowedOrigins),
		mux.WithFileServer(true, static, "static", "/"),
	)

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
		defer log.Info(ctx, "shutdown", "status", "shutdown complete", "signal", sig)

		ctx, cancel := context.WithTimeout(ctx, cfg.Web.ShutdownTimeout)
		defer cancel()

		if err := api.Shutdown(ctx); err != nil {
			api.Close()
			return fmt.Errorf("could not stop server gracefully: %w", err)
		}
	}

	return nil
}

var logo = `
██╗  ██╗██████╗  ██████╗ ███╗   ██╗██╗  ██╗
██║ ██╔╝██╔══██╗██╔═══██╗████╗  ██║██║ ██╔╝
█████╔╝ ██████╔╝██║   ██║██╔██╗ ██║█████╔╝ 
██╔═██╗ ██╔══██╗██║   ██║██║╚██╗██║██╔═██╗ 
██║  ██╗██║  ██║╚██████╔╝██║ ╚████║██║  ██╗
╚═╝  ╚═╝╚═╝  ╚═╝ ╚═════╝ ╚═╝  ╚═══╝╚═╝  ╚═╝
`
