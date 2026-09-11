package toolapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	malinalibs "github.com/ardanlabs/kronk/sdk/tools/malina/libs"
)

func (a *app) listMalinaLibs(ctx context.Context, r *http.Request) web.Encoder {
	tag, err := a.malinaLibs.InstalledVersion()
	if err != nil {
		return toAppVersionTag("not installed", tag, a.malinaLibs.AllowUpgrade)
	}

	return toAppVersionTag("retrieve", tag, a.malinaLibs.AllowUpgrade)
}

func (a *app) verifyMalinaLibs(ctx context.Context, r *http.Request) web.Encoder {
	version := r.URL.Query().Get("version")
	if version == "" {
		tag, err := a.malinaLibs.InstalledVersion()
		if err != nil {
			return errs.Errorf(errs.Internal, "unable to read stable-diffusion.cpp library version: %s", err)
		}
		version = tag.Version
	}

	report, err := a.malinaLibs.Verify(ctx, version)
	if err != nil {
		if errors.Is(err, malinalibs.ErrInvalidDigest) || errors.Is(err, malinalibs.ErrInvalidVersion) {
			return errs.Errorf(errs.InvalidArgument, "invalid library version: %s", err)
		}
		return errs.Errorf(errs.Internal, "unable to verify stable-diffusion.cpp libraries: %s", err)
	}

	var verifiedPaths []string
	for _, file := range report.Files {
		if file.State.String() == "verified" {
			verifiedPaths = append(verifiedPaths, file.Name)
		}
	}
	manifest, verifiedAt, err := buildRuntimeBundleManifest(ctx, a.malinaLibs.LibsPath(), verifiedPaths, report.OK())
	if err != nil {
		return errs.Errorf(errs.Internal, "unable to identify stable-diffusion.cpp libraries: %s", err)
	}

	return toAppMalinaLibIntegrity(report, a.malinaLibs, manifest, verifiedAt)
}

func (a *app) pullMalinaLibs(ctx context.Context, r *http.Request) web.Encoder {
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
	if tripleAll && !malinalibs.IsSupported(arch, opSys, processor) {
		return errs.Errorf(errs.InvalidArgument, "unsupported combination arch=%q os=%q processor=%q", arch, opSys, processor)
	}
	if a.malinaLibs.ReadOnly() {
		return errs.FromSDK(malinalibs.ErrReadOnly)
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

	allowUpgrade := a.malinaLibs.AllowUpgrade
	if !tripleAll && version == "" && q.Get("allow-upgrade") != "" {
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
		ver := toAppVersion(status, malinalibs.VersionTag{}, allowUpgrade)

		a.log.Info(ctx, "pull-malina-libs", "info", ver[:len(ver)-1])
		fmt.Fprint(w, ver)
		f.Flush()
	}

	if allowUpgrade && !a.malinaLibs.AllowUpgrade {
		a.log.Info(ctx, "pull-malina-libs", "status", "allowing libs upgrade")
		a.malinaLibs.AllowUpgrade = true
		defer func() {
			a.malinaLibs.AllowUpgrade = false
		}()
	}

	var (
		tag malinalibs.VersionTag
		err error
	)
	switch {
	case tripleAll:
		tag, err = a.malinaLibs.DownloadFor(ctx, logger, arch, opSys, processor, version)
	case version != "":
		tag, err = a.malinaLibs.DownloadFor(ctx, logger, a.malinaLibs.Arch(), a.malinaLibs.OS(), a.malinaLibs.Processor(), version)
	default:
		tag, err = a.malinaLibs.Download(ctx, logger)
	}
	if err != nil {
		ver := toAppVersion(err.Error(), malinalibs.VersionTag{}, allowUpgrade)
		a.log.Info(ctx, "pull-malina-libs", "status", "ERROR", "error", err.Error())
		fmt.Fprint(w, ver)
		f.Flush()
		return web.NewNoResponse()
	}

	ver := toAppVersion("downloaded", tag, allowUpgrade)
	a.log.Info(ctx, "pull-malina-libs", "info", ver[:len(ver)-1])
	fmt.Fprint(w, ver)
	f.Flush()

	return web.NewNoResponse()
}

func (a *app) listMalinaLibsCombinations(ctx context.Context, r *http.Request) web.Encoder {
	return toAppCombinations(malinalibs.SupportedCombinations())
}

func (a *app) listMalinaLibsInstalls(ctx context.Context, r *http.Request) web.Encoder {
	tags, err := a.malinaLibs.List()
	if err != nil {
		return errs.FromSDK(err)
	}

	return toAppBundleList(tags)
}

func (a *app) removeMalinaLibsInstall(ctx context.Context, r *http.Request) web.Encoder {
	q := r.URL.Query()
	arch := q.Get("arch")
	opSys := q.Get("os")
	processor := q.Get("processor")

	if arch == "" || opSys == "" || processor == "" {
		return errs.Errorf(errs.InvalidArgument, "arch, os, and processor are required")
	}

	if err := a.malinaLibs.Remove(arch, opSys, processor); err != nil {
		return errs.FromSDK(err)
	}

	return BundleActionResponse{Status: "removed", Arch: arch, OS: opSys, Processor: processor}
}
