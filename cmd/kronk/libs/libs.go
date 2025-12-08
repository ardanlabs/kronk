// Package libs provides the libs command code.
package libs

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/kronk/website/app/domain/toolapp"
	"github.com/ardanlabs/kronk/cmd/kronk/website/app/sdk/errs"
	"github.com/ardanlabs/kronk/defaults"
	"github.com/ardanlabs/kronk/tools"
)

// RunWeb executes the libs command against the model server.
func RunWeb(args []string) error {
	url, err := client.DefaultURL("/v1/libs")
	if err != nil {
		return fmt.Errorf("run-web: default: %w", err)
	}

	fmt.Println("URL:", url)

	client := client.New(client.FmtLogger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var version toolapp.Version
	if err := client.Do(ctx, http.MethodGet, url, nil, &version); err != nil {
		return fmt.Errorf("libs:unable to get version: %w", err)
	}

	return nil
}

// RunLocal executes the libs command locally.
func RunLocal(args []string) error {
	arch, err := defaults.Arch("")
	if err != nil {
		return err
	}

	os, err := defaults.OS("")
	if err != nil {
		return err
	}

	proc, err := defaults.Processor("")
	if err != nil {
		return err
	}

	libCfg, err := tools.NewLibConfig(
		defaults.LibsDir(""),
		arch.String(),
		os.String(),
		proc.String(),
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
		return errs.Errorf(errs.Internal, "libs:unable to install llama.cpp: %s", err)
	}

	if err := kronk.Init(libCfg.LibPath, kronk.LogSilent); err != nil {
		return fmt.Errorf("libs:installation invalid: %w", err)
	}

	return nil
}
