// Package libs provides the libs command code.
package libs

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/sdk/client"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
)

func runWeb() error {
	url, err := client.DefaultURL("/v1/libs/pull")
	if err != nil {
		return fmt.Errorf("libs: default: %w", err)
	}

	fmt.Println("URL:", url)

	cln := client.NewSSE[toolapp.VersionResponse](client.FmtLogger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ch := make(chan toolapp.VersionResponse)
	if err := cln.Do(ctx, http.MethodPost, url, nil, ch); err != nil {
		return fmt.Errorf("libs: unable to download libs: %w", err)
	}

	for ver := range ch {
		fmt.Print(ver.Status)
	}

	fmt.Println()

	return nil
}

func runLocal() error {
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

	libCfg, err := libs.NewConfig(
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	_, err = libs.Download(ctx, kronk.FmtLogger, libCfg)
	if err != nil {
		return errs.Errorf(errs.Internal, "libs:unable to install llama.cpp: %s", err)
	}

	if err := kronk.Init(libCfg.LibPath, kronk.LogSilent); err != nil {
		return fmt.Errorf("libs:installation invalid: %w", err)
	}

	return nil
}
