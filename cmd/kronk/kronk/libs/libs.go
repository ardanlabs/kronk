// Package libs provides the libs command code.
package libs

import (
	"context"
	"fmt"
	"net/http"
	neturl "net/url"
	"os"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
)

func runWeb(ctx context.Context, upgrade bool, version string) error {
	url, err := client.DefaultURL("/v1/kronk/libs/pull")
	if err != nil {
		return fmt.Errorf("libs: default: %w", err)
	}

	q := make(neturl.Values)
	if upgrade {
		q.Set("allow-upgrade", "true")
	}
	if version != "" {
		q.Set("version", version)
	}
	if len(q) > 0 {
		url += "?" + q.Encode()
	}

	fmt.Println("URL:", url)

	cln := client.NewSSE[toolapp.VersionResponse](
		client.FmtLogger,
		client.WithBearer(os.Getenv("KRONK_TOKEN")),
	)

	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	ch := make(chan toolapp.VersionResponse)
	errCh, err := cln.DoWithErrors(ctx, http.MethodPost, url, nil, ch)
	if err != nil {
		return fmt.Errorf("libs: unable to download libs: %w", err)
	}

	var status string
	for ver := range ch {
		fmt.Print(ver.Status)
		status = ver.Status
	}

	fmt.Println()

	if err := <-errCh; err != nil {
		return fmt.Errorf("libs: unable to download libs: %w", err)
	}
	if status != "downloaded" && status != "using installed version" {
		return fmt.Errorf("libs: install did not complete successfully: %s", status)
	}

	return nil
}

func runLocal(ctx context.Context, basePath string, upgrade bool, version string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	v := defaults.LibVersion("")
	if version != "" {
		v = version
	}

	libs, err := libs.New(
		libs.WithDetect(ctx, kronk.FmtLogger),
		libs.WithBasePath(basePath),
		libs.WithVersion(v),
		libs.WithAllowUpgrade(upgrade),
	)
	if err != nil {
		return err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	if err := kronk.Init(kronk.WithLibPath(libs.LibsPath())); err != nil {
		return fmt.Errorf("libs:installation invalid: %w", err)
	}

	return nil
}
