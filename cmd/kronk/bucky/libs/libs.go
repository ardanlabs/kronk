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
	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
)

// runDefaultLocal installs the whisper.cpp libraries for the current
// host using the well-known default version (or the supplied version,
// or the latest published release when upgrade is set) and then
// initializes the bucky runtime against the install path so the
// libraries can be loaded.
func runDefaultLocal(upgrade bool, version string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	lib, err := libs.New(
		libs.WithLibPath(""),
		libs.WithVersion(version),
		libs.WithAllowUpgrade(upgrade),
	)
	if err != nil {
		return fmt.Errorf("bucky libs: new: %w", err)
	}

	if _, err := lib.Download(ctx, bucky.FmtLogger); err != nil {
		return fmt.Errorf("bucky libs: unable to install whisper.cpp: %w", err)
	}

	if err := bucky.Init(bucky.WithInitLibPath(lib.LibsPath())); err != nil {
		return fmt.Errorf("bucky libs: installation invalid: %w", err)
	}

	return nil
}

// runDefaultWeb installs the whisper.cpp libraries for the active
// triple by streaming progress from the model server's
// /v1/bucky/libs/pull endpoint.
func runDefaultWeb(upgrade bool, version string) error {
	url, err := client.DefaultURL("/v1/bucky/libs/pull")
	if err != nil {
		return fmt.Errorf("bucky libs: default url: %w", err)
	}

	q := neturl.Values{}
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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	ch := make(chan toolapp.VersionResponse)
	if err := cln.Do(ctx, http.MethodPost, url, nil, ch); err != nil {
		return fmt.Errorf("bucky libs: unable to install whisper.cpp: %w", err)
	}

	for ver := range ch {
		fmt.Print(ver.Status)
	}
	fmt.Println()

	return nil
}
