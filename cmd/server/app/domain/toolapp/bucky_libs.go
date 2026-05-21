package toolapp

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	buckylibs "github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
)

// listBuckyLibs returns the installed whisper.cpp library version for
// the active triple. Unlike the llama backend, bucky's library system
// does not track an "allow upgrade" toggle and does not synthesize a
// "latest" comparison, so the response is a thin status envelope around
// the on-disk version tag.
func (a *app) listBuckyLibs(ctx context.Context, r *http.Request) web.Encoder {
	tag, err := a.buckyLibs.InstalledVersion()
	if err != nil {
		return toAppVersionTag("not installed", tag, false)
	}

	return toAppVersionTag("retrieve", tag, false)
}

// pullBuckyLibs streams a whisper.cpp library install. With no triple
// query parameters it installs into the active triple via
// buckylibs.Download. When arch, os, and processor are all supplied it
// performs a cross-triple install via buckylibs.DownloadFor.
func (a *app) pullBuckyLibs(ctx context.Context, r *http.Request) web.Encoder {
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
	if tripleAll && !buckylibs.IsSupported(arch, opSys, processor) {
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

	logger := func(ctx context.Context, msg string, args ...any) {
		var sb strings.Builder
		for i := 0; i < len(args); i += 2 {
			if i+1 < len(args) {
				fmt.Fprintf(&sb, " %v[%v]", args[i], args[i+1])
			}
		}

		status := fmt.Sprintf("%s:%s\n", msg, sb.String())
		ver := toAppVersion(status, buckylibs.VersionTag{}, false)

		a.log.Info(ctx, "pull-bucky-libs", "info", ver[:len(ver)-1])
		fmt.Fprint(w, ver)
		f.Flush()
	}

	var (
		tag buckylibs.VersionTag
		err error
	)
	switch {
	case tripleAll:
		tag, err = a.buckyLibs.DownloadFor(ctx, logger, arch, opSys, processor, version)
	case version != "":
		// Bucky's Libs has no SetVersion knob, so a version override
		// against the active triple is dispatched through DownloadFor
		// using the libs handle's own triple.
		tag, err = a.buckyLibs.DownloadFor(ctx, logger, a.buckyLibs.Arch(), a.buckyLibs.OS(), a.buckyLibs.Processor(), version)
	default:
		tag, err = a.buckyLibs.Download(ctx, logger)
	}
	if err != nil {
		ver := toAppVersion(err.Error(), buckylibs.VersionTag{}, false)
		a.log.Info(ctx, "pull-bucky-libs", "status", "ERROR", "error", err.Error())
		fmt.Fprint(w, ver)
		f.Flush()
		return web.NewNoResponse()
	}

	ver := toAppVersion("downloaded", tag, false)
	a.log.Info(ctx, "pull-bucky-libs", "info", ver[:len(ver)-1])
	fmt.Fprint(w, ver)
	f.Flush()

	return web.NewNoResponse()
}

func (a *app) listBuckyLibsCombinations(ctx context.Context, r *http.Request) web.Encoder {
	return toAppCombinations(buckylibs.SupportedCombinations())
}

func (a *app) listBuckyLibsInstalls(ctx context.Context, r *http.Request) web.Encoder {
	tags, err := a.buckyLibs.List()
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	return toAppBundleList(tags)
}

func (a *app) removeBuckyLibsInstall(ctx context.Context, r *http.Request) web.Encoder {
	q := r.URL.Query()
	arch := q.Get("arch")
	opSys := q.Get("os")
	processor := q.Get("processor")

	if arch == "" || opSys == "" || processor == "" {
		return errs.Errorf(errs.InvalidArgument, "arch, os, and processor are required")
	}

	if err := a.buckyLibs.Remove(arch, opSys, processor); err != nil {
		return errs.New(errs.Internal, err)
	}

	return BundleActionResponse{Status: "removed", Arch: arch, OS: opSys, Processor: processor}
}
