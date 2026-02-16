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

func runWeb(noUpgrade bool, version string) error {
	url, err := client.DefaultURL("/v1/libs/pull")
	if err != nil {
		return fmt.Errorf("libs: default: %w", err)
	}

	q := make(neturl.Values)
	if !noUpgrade {
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

func runLocal(noUpgrade bool, version string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	v := defaults.LibVersion("")
	if version != "" {
		v = version
	}

	libs, err := libs.New(
		libs.WithVersion(v),
		libs.WithAllowUpgrade(!noUpgrade),
	)
	if err != nil {
		return err
	}

	if _, err := libs.Download(ctx, kronk.FmtLogger); err != nil {
		return fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	if err := kronk.Init(); err != nil {
		return fmt.Errorf("libs:installation invalid: %w", err)
	}

	return nil
}
