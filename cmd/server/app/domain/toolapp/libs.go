package toolapp

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
)

func (a *app) listLibs(ctx context.Context, r *http.Request) web.Encoder {
	versionTag, err := a.libs.VersionInformation()
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return toAppVersionTag("retrieve", versionTag, a.libs.AllowUpgrade)
}

// pullLibs streams a library install. With no triple query parameters it
// runs the active-triple workflow (version checks, AllowUpgrade, network
// fallback, and an opportunistic kronk.Init when the runtime hasn't loaded
// yet). When arch, os, and processor are all supplied it performs a
// cross-triple install into <root>/<os>/<arch>/<processor>/ via
// libs.DownloadFor and never touches the active runtime.
func (a *app) pullLibs(ctx context.Context, r *http.Request) web.Encoder {
	q := r.URL.Query()
	arch := q.Get("arch")
	opSys := q.Get("os")
	processor := q.Get("processor")
	version := q.Get("version")

	tripleAny := arch != "" || opSys != "" || processor != ""
	tripleAll := arch != "" && opSys != "" && processor != ""

	if tripleAny && !tripleAll {
		return errs.Errorf(errs.InvalidArgument, "arch, os, and processor must all be supplied together")
	}
	if tripleAll && !libs.IsSupported(arch, opSys, processor) {
		return errs.Errorf(errs.InvalidArgument, "unsupported combination arch=%q os=%q processor=%q", arch, opSys, processor)
	}

	w := web.GetWriter(ctx)

	f, ok := w.(http.Flusher)
	if !ok {
		return errs.Errorf(errs.Internal, "streaming not supported")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)
	f.Flush()

	// -------------------------------------------------------------------------

	allowUpgrade := a.libs.AllowUpgrade
	if tripleAll {
		allowUpgrade = true
	}

	logger := func(ctx context.Context, msg string, args ...any) {
		var sb strings.Builder
		for i := 0; i < len(args); i += 2 {
			if i+1 < len(args) {
				fmt.Fprintf(&sb, " %v[%v]", args[i], args[i+1])
			}
		}

		status := fmt.Sprintf("%s:%s\n", msg, sb.String())
		ver := toAppVersion(status, libs.VersionTag{}, allowUpgrade)

		a.log.Info(ctx, "pull-libs", "info", ver[:len(ver)-1])
		fmt.Fprint(w, ver)
		f.Flush()
	}

	if tripleAll {
		tag, err := a.libs.DownloadFor(ctx, logger, arch, opSys, processor, version)
		if err != nil {
			ver := toAppVersion(err.Error(), libs.VersionTag{}, true)
			a.log.Info(ctx, "pull-libs", "status", "ERROR", "error", err.Error())
			fmt.Fprint(w, ver)
			f.Flush()
			return web.NewNoResponse()
		}

		ver := toAppVersion("downloaded", tag, true)
		fmt.Fprint(w, ver)
		f.Flush()
		return web.NewNoResponse()
	}

	instVer, err := a.libs.InstalledVersion()
	if err != nil {
		a.log.Warn(ctx, "pull-libs", "status", "no installed version found", "warning", err)
	}

	// I know this is a hack and a race condition. I expect this situation
	// to only exist for a few people and in a single tenant mode.
	if !a.libs.AllowUpgrade {
		if q.Get("allow-upgrade") != "" {
			a.log.Info(ctx, "pull-libs", "status", "allowing libs upgrade")
			a.libs.AllowUpgrade = true
			defer func() {
				a.libs.AllowUpgrade = false
			}()
		}
	}

	if version != "" {
		a.log.Info(ctx, "pull-libs", "status", "using specified version", "version", version)
		a.libs.SetVersion(version)
		defer func() {
			a.libs.SetVersion("")
		}()
	}

	vi, err := a.libs.Download(ctx, logger)
	if err != nil {
		ver := toAppVersion(err.Error(), libs.VersionTag{}, a.libs.AllowUpgrade)

		a.log.Info(ctx, "pull-libs", "status", "ERROR", "error", err.Error(), "info", ver[:len(ver)-1])
		fmt.Fprint(w, ver)
		f.Flush()

		return web.NewNoResponse()
	}

	// If kronk hasn't been initialized yet, attempt to load the freshly
	// downloaded libraries. This allows the server to recover from a
	// degraded state without a restart.
	if !kronk.Initialized() {
		if err := kronk.Init(kronk.WithLibPath(a.libs.LibsPath())); err != nil {
			a.log.Info(ctx, "pull-libs", "WARNING", "libraries downloaded but failed to initialize kronk", "ERROR", err)
		} else {
			a.log.Info(ctx, "pull-libs", "status", "kronk initialized successfully after library download")
		}
	}

	var ver string
	ver = toAppVersion("downloaded", vi, a.libs.AllowUpgrade)
	if instVer.Version == vi.Version {
		ver = toAppVersion("using installed version", vi, a.libs.AllowUpgrade)
	}

	a.log.Info(ctx, "pull-libs", "info", ver[:len(ver)-1])
	fmt.Fprint(w, ver)
	f.Flush()

	return web.NewNoResponse()
}

func (a *app) listLibsCombinations(ctx context.Context, r *http.Request) web.Encoder {
	return toAppCombinations(libs.SupportedCombinations())
}

func (a *app) listLibsInstalls(ctx context.Context, r *http.Request) web.Encoder {
	tags, err := a.libs.List()
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return toAppBundleList(tags)
}

func (a *app) removeLibsInstall(ctx context.Context, r *http.Request) web.Encoder {
	q := r.URL.Query()
	arch := q.Get("arch")
	opSys := q.Get("os")
	processor := q.Get("processor")

	if arch == "" || opSys == "" || processor == "" {
		return errs.Errorf(errs.InvalidArgument, "arch, os, and processor are required")
	}

	if err := a.libs.Remove(arch, opSys, processor); err != nil {
		return errs.New(errs.Internal, err)
	}

	return BundleActionResponse{Status: "removed", Arch: arch, OS: opSys, Processor: processor}
}
