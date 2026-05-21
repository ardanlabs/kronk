package libs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
)

// installOpts captures the parsed install-related CLI flags. Exactly one of
// install/list/listCombo/remove should be true when isInstallOp returns true;
// runInstall* functions return an error otherwise.
type installOpts struct {
	arch      string
	os        string
	processor string
	version   string
	install   bool
	list      bool
	listCombo bool
	remove    bool
}

// isInstallOp reports whether any install-related flag was supplied.
func (o installOpts) isInstallOp() bool {
	return o.install || o.list || o.listCombo || o.remove
}

// requireTriple validates that arch/os/processor were all supplied for
// operations that need a specific install identity.
func (o installOpts) requireTriple() error {
	if o.arch == "" || o.os == "" || o.processor == "" {
		return fmt.Errorf("--arch, --os, and --processor are all required for this operation")
	}
	return nil
}

// runInstallLocal handles every install subcommand against an in-process
// libs.Libs without contacting a running server.
func runInstallLocal(opts installOpts) error {
	if opts.listCombo {
		printCombinations(libs.SupportedCombinations())
		return nil
	}

	lib, err := libs.New()
	if err != nil {
		return fmt.Errorf("libs: install: %w", err)
	}

	switch {
	case opts.list:
		tags, err := lib.List()
		if err != nil {
			return fmt.Errorf("libs: list installs: %w", err)
		}
		printInstallTags(lib, tags)
		return nil

	case opts.remove:
		if err := opts.requireTriple(); err != nil {
			return err
		}
		if err := lib.Remove(opts.arch, opts.os, opts.processor); err != nil {
			return fmt.Errorf("libs: remove install: %w", err)
		}
		fmt.Printf("removed install arch=%s os=%s processor=%s\n", opts.arch, opts.os, opts.processor)
		return nil

	case opts.install:
		if err := opts.requireTriple(); err != nil {
			return err
		}
		if !libs.IsSupported(opts.arch, opts.os, opts.processor) {
			return fmt.Errorf("libs: unsupported combination arch=%s os=%s processor=%s", opts.arch, opts.os, opts.processor)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()

		if _, err := lib.DownloadFor(ctx, kronk.FmtLogger, opts.arch, opts.os, opts.processor, opts.version); err != nil {
			return fmt.Errorf("libs: install: %w", err)
		}
		fmt.Println()

		printUseHint(lib, opts.arch, opts.os, opts.processor)
		return nil
	}

	return fmt.Errorf("libs: no install operation requested")
}

// runInstallWeb handles every install subcommand by issuing requests to a
// running model server using the same client transport as the existing
// libs commands.
func runInstallWeb(opts installOpts) error {
	switch {
	case opts.listCombo:
		return webListCombinations()

	case opts.list:
		return webListInstalls()

	case opts.remove:
		if err := opts.requireTriple(); err != nil {
			return err
		}
		return webMutateInstall(http.MethodDelete, "/v1/libs/installs", opts)

	case opts.install:
		if err := opts.requireTriple(); err != nil {
			return err
		}
		return webPullInstall(opts)
	}

	return fmt.Errorf("libs: no install operation requested")
}

func webListCombinations() error {
	body, err := webGet("/v1/libs/combinations")
	if err != nil {
		return err
	}
	defer body.Close()

	var resp toolapp.CombinationsResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return fmt.Errorf("libs: decode combinations: %w", err)
	}

	combos := make([]libs.Combination, len(resp.Combinations))
	for i, c := range resp.Combinations {
		combos[i] = libs.Combination{Arch: c.Arch, OS: c.OS, Processor: c.Processor}
	}
	printCombinations(combos)
	return nil
}

func webListInstalls() error {
	body, err := webGet("/v1/libs/installs")
	if err != nil {
		return err
	}
	defer body.Close()

	var resp toolapp.BundleListResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return fmt.Errorf("libs: decode installs: %w", err)
	}

	tags := make([]libs.VersionTag, len(resp.Bundles))
	for i, v := range resp.Bundles {
		tags[i] = libs.VersionTag{
			Version:   v.Version,
			Arch:      v.Arch,
			OS:        v.OS,
			Processor: v.Processor,
		}
	}
	printInstallTags(nil, tags)
	return nil
}

func webMutateInstall(method string, path string, opts installOpts) error {
	q := neturl.Values{}
	q.Set("arch", opts.arch)
	q.Set("os", opts.os)
	q.Set("processor", opts.processor)

	url, err := client.DefaultURL(path)
	if err != nil {
		return fmt.Errorf("libs: default url: %w", err)
	}
	url += "?" + q.Encode()

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return fmt.Errorf("libs: build request: %w", err)
	}
	if tok := os.Getenv("KRONK_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("libs: request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("libs: server returned %d: %s", resp.StatusCode, string(body))
	}
	fmt.Println(string(body))
	return nil
}

func webPullInstall(opts installOpts) error {
	q := neturl.Values{}
	q.Set("arch", opts.arch)
	q.Set("os", opts.os)
	q.Set("processor", opts.processor)
	if opts.version != "" {
		q.Set("version", opts.version)
	}

	url, err := client.DefaultURL("/v1/libs/pull")
	if err != nil {
		return fmt.Errorf("libs: default url: %w", err)
	}
	url += "?" + q.Encode()

	cln := client.NewSSE[toolapp.VersionResponse](
		client.FmtLogger,
		client.WithBearer(os.Getenv("KRONK_TOKEN")),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	ch := make(chan toolapp.VersionResponse)
	if err := cln.Do(ctx, http.MethodPost, url, nil, ch); err != nil {
		return fmt.Errorf("libs: install: %w", err)
	}

	for ver := range ch {
		fmt.Print(ver.Status)
	}
	fmt.Println()
	return nil
}

func webGet(path string) (io.ReadCloser, error) {
	url, err := client.DefaultURL(path)
	if err != nil {
		return nil, fmt.Errorf("libs: default url: %w", err)
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("libs: build request: %w", err)
	}
	if tok := os.Getenv("KRONK_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("libs: request: %w", err)
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("libs: server returned %d: %s", resp.StatusCode, string(body))
	}
	return resp.Body, nil
}

func printCombinations(combos []libs.Combination) {
	if len(combos) == 0 {
		fmt.Println("no combinations available")
		return
	}
	fmt.Printf("%-10s %-10s %-10s\n", "OS", "ARCH", "PROCESSOR")
	for _, c := range combos {
		fmt.Printf("%-10s %-10s %-10s\n", c.OS, c.Arch, c.Processor)
	}
}

// printInstallTags prints the installed bundles. When lib is non-nil the
// active install (matching lib.LibsPath) is annotated with "(active)".
func printInstallTags(lib *libs.Libs, tags []libs.VersionTag) {
	if len(tags) == 0 {
		fmt.Println("no installs found")
		return
	}

	var activeTriple [3]string
	if lib != nil {
		activeTriple = [3]string{lib.OS(), lib.Arch(), lib.Processor()}
	}

	fmt.Printf("%-10s %-10s %-10s %-10s %s\n", "OS", "ARCH", "PROCESSOR", "VERSION", "")
	for _, t := range tags {
		flag := ""
		if lib != nil && [3]string{t.OS, t.Arch, t.Processor} == activeTriple {
			flag = "(active)"
		}
		fmt.Printf("%-10s %-10s %-10s %-10s %s\n", t.OS, t.Arch, t.Processor, t.Version, flag)
	}
}

// printUseHint prints the environment variable a user must export to load
// the install for the supplied triple as the active runtime libraries.
func printUseHint(lib *libs.Libs, arch string, opSys string, processor string) {
	path := filepath.Join(lib.Root(), opSys, arch, processor)
	fmt.Println("To use this install at runtime, set:")
	fmt.Printf("  export KRONK_LIB_PATH=%s\n", path)
}
