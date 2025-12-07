// Package libs provides the libs command code.
package libs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/kronk/website/app/domain/toolapp"
	"github.com/ardanlabs/kronk/cmd/kronk/website/app/sdk/errs"
	"github.com/ardanlabs/kronk/defaults"
	"github.com/ardanlabs/kronk/tools"
	"github.com/hybridgroup/yzma/pkg/download"
)

var ErrInvalidArguments = errors.New("invalid arguments")

// RunWeb executes the libs command against the model server.
func RunWeb(args []string) error {
	host := "127.0.0.1:3000"
	if v := os.Getenv("KRONK_HOST"); v != "" {
		host = v
	}

	u := &url.URL{
		Scheme: "http",
		Host:   host,
		Path:   "/v1/tool/libs",
	}

	endpoint, err := url.JoinPath(u.String(), "v1", "tool", "libs")
	if err != nil {
		return fmt.Errorf("invalid host information %q: %w", host, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := client.New(client.FmtLogger)

	var version toolapp.Version
	if err := client.Do(ctx, http.MethodGet, endpoint, nil, &version); err != nil {
		return fmt.Errorf("unable to get version: %w", err)
	}

	return nil
}

// RunLocal executes the libs command locally.
func RunLocal(args []string) error {
	libCfg, err := tools.NewLibConfig(
		defaults.LibsDir(""),
		runtime.GOARCH,
		runtime.GOOS,
		download.CPU.String(),
		kronk.LogSilent.Int(),
		true,
	)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	_, err = tools.DownloadLibraries(ctx, tools.FmtLogger, libCfg)
	if err != nil {
		return errs.Errorf(errs.Internal, "unable to install llama.cpp: %s", err)
	}

	if err := kronk.Init(libCfg.LibPath, kronk.LogSilent); err != nil {
		return fmt.Errorf("installation invalid: %w", err)
	}

	return nil
}
