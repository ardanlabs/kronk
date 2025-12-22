package main

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/ardanlabs/conf/v3"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/authapp"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/debug"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/otel"
	"github.com/ardanlabs/kronk/sdk/tools/security"
)

var tag = "develop"

func main() {
	var log *logger.Logger

	events := logger.Events{
		Error: func(ctx context.Context, r logger.Record) {
			log.Info(ctx, "******* SEND ALERT *******")
		},
	}

	log = logger.NewWithEvents(os.Stdout, logger.LevelInfo, "AUTH", otel.GetTraceID, events)

	// -------------------------------------------------------------------------

	ctx := context.Background()

	if err := run(ctx, log); err != nil {
		log.Error(ctx, "startup", "err", err)
		os.Exit(1)
	}
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
			DebugHost string `conf:"default:0.0.0.0:6010"`
		}
		Auth struct {
			Host    string `conf:"default:0.0.0.0:6000"`
			Issuer  string `conf:"default:kronk project"`
			Enabled bool   `conf:"default:false"`
		}
		Tempo struct {
			Host        string  `conf:"default:tempo:4317"`
			ServiceName string  `conf:"default:auth"`
			Probability float64 `conf:"default:0.05"`
		}
	}{
		Version: conf.Version{
			Build: tag,
			Desc:  "Auth",
		},
	}

	const prefix = "AUTH"
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

	sec, err := security.New(security.Config{
		Issuer: cfg.Auth.Issuer,
	})

	if err != nil {
		return fmt.Errorf("unable to initialize security system: %w", err)
	}

	defer sec.Close()

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

	defer teardown(context.Background())

	tracer := traceProvider.Tracer(cfg.Tempo.ServiceName)

	// -------------------------------------------------------------------------
	// Start Debug Service

	go func() {
		log.Info(ctx, "startup", "status", "debug v1 router started", "host", cfg.Web.DebugHost)

		if err := http.ListenAndServe(cfg.Web.DebugHost, debug.Mux()); err != nil {
			log.Error(ctx, "shutdown", "status", "debug v1 router closed", "host", cfg.Web.DebugHost, "msg", err)
		}
	}()

	// -------------------------------------------------------------------------
	// Start Auth Service

	log.Info(ctx, "startup", "status", "initializing auth server")

	lis, err := net.Listen("tcp", cfg.Auth.Host)
	if err != nil {
		return fmt.Errorf("failed to listen on host %s : %w", cfg.Auth.Host, err)
	}

	authApp := authapp.Start(ctx, authapp.Config{
		Log:      log,
		Security: sec,
		Listener: lis,
		Tracer:   tracer,
		Enabled:  cfg.Auth.Enabled,
	})

	defer authApp.Shutdown(ctx)

	// -------------------------------------------------------------------------
	// Wait and Shutdown

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	sig := <-shutdown

	log.Info(ctx, "shutdown", "status", "shutdown started", "signal", sig)
	defer log.Info(ctx, "shutdown", "status", "shutdown complete", "signal", sig)

	return nil
}
