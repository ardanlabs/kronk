// Package kronk is the model server.
package kronk

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/ardanlabs/conf/v3"
	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/cmd/kronk/website/api/services/kronk/build/all"
	"github.com/ardanlabs/kronk/cmd/kronk/website/app/sdk/auth"
	"github.com/ardanlabs/kronk/cmd/kronk/website/app/sdk/debug"
	"github.com/ardanlabs/kronk/cmd/kronk/website/app/sdk/krn"
	"github.com/ardanlabs/kronk/cmd/kronk/website/app/sdk/mux"
	"github.com/ardanlabs/kronk/cmd/kronk/website/foundation/keystore"
	"github.com/ardanlabs/kronk/cmd/kronk/website/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/kronk/website/foundation/otel"
	"github.com/ardanlabs/kronk/tools"
)

var build = "develop"

func Run() error {
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

	if err := run(ctx, log); err != nil {
		return err
	}

	return nil
}

func run(ctx context.Context, log *logger.Logger) error {

	// -------------------------------------------------------------------------
	// GOMAXPROCS

	log.Info(ctx, "startup", "GOMAXPROCS", runtime.GOMAXPROCS(0))

	// -------------------------------------------------------------------------
	// Configuration

	cfg := struct {
		conf.Version
		Web struct {
			ReadTimeout        time.Duration `conf:"default:25s"`
			WriteTimeout       time.Duration `conf:"default:360s"`
			IdleTimeout        time.Duration `conf:"default:360s"`
			ShutdownTimeout    time.Duration `conf:"default:45s"`
			APIHost            string        `conf:"default:0.0.0.0:3000"`
			DebugHost          string        `conf:"default:0.0.0.0:3010"`
			CORSAllowedOrigins []string      `conf:"default:*"`
		}
		Auth struct {
			KeysJSON   string `conf:"mask"`
			KeysFolder string `conf:"default:cmd/kronk/website/zarf/keys/"`
			ActiveKID  string `conf:"default:54bb2165-71e1-41a6-af3e-7da4a0e1e2c1"`
			Issuer     string `conf:"default:kronk project"`
			Enabled    bool   `conf:"default:false"`
		}
		Tempo struct {
			Host        string  `conf:"default:tempo:4317"`
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
			Build: build,
			Desc:  "Kronk",
		},
	}

	const prefix = "KRONK"
	help, err := conf.Parse(prefix, &cfg)
	if err != nil {
		if errors.Is(err, conf.ErrHelpWanted) {
			fmt.Println(help)
			return nil
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

	// -------------------------------------------------------------------------
	// Initialize authentication support

	log.Info(ctx, "startup", "status", "initializing authentication support")

	// Check the enviornment first to see if a key is being provided. Then
	// load any private keys files from disk. We can assume some system like
	// Vault has created these files already. How that happens is not our
	// concern.

	ks := keystore.New()

	n1, err := ks.LoadByJSON(cfg.Auth.KeysJSON)
	if err != nil {
		return fmt.Errorf("loading keys by env: %w", err)
	}

	n2, err := ks.LoadByFileSystem(os.DirFS(cfg.Auth.KeysFolder))
	if err != nil {
		return fmt.Errorf("loading keys by fs: %w", err)
	}

	if n1+n2 == 0 {
		return errors.New("no keys exist")
	}

	authCfg := auth.Config{
		Log:       log,
		KeyLookup: ks,
		Issuer:    cfg.Auth.Issuer,
		Enabled:   cfg.Auth.Enabled,
	}

	ath := auth.New(authCfg)

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
	// Init Kronk

	log.Info(ctx, "startup", "status", "initializing kronk")

	libCfg, err := tools.NewLibConfig(
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

	if _, err := tools.DownloadLibraries(ctx, log.Info, libCfg); err != nil {
		return fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	if err := kronk.Init(libCfg.LibPath, libCfg.LlamaLog); err != nil {
		return fmt.Errorf("installation invalid: %w", err)
	}

	krnMngr, err := krn.NewManager(krn.Config{
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
	})

	if err != nil {
		return fmt.Errorf("initializing kronk manager: %w", err)
	}

	defer func() {
		log.Info(ctx, "shutdown", "status", "shutting down kronk")

		ctx, cancel := context.WithTimeout(context.Background(), cfg.Web.ShutdownTimeout)
		defer cancel()

		if err := krnMngr.Shutdown(ctx); err != nil {
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
		Build:   build,
		Log:     log,
		Auth:    ath,
		Tracer:  tracer,
		KrnMngr: krnMngr,
	}

	webAPI := mux.WebAPI(cfgMux,
		all.Routes(),
		mux.WithCORS(cfg.Web.CORSAllowedOrigins),
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
