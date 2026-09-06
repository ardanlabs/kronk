package toolapp

import (
	"testing"
	"time"

	buckydownload "github.com/ardanlabs/bucky/pkg/download"
	"github.com/ardanlabs/kronk/sdk/tools/backend"
	buckylibs "github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	malinalibs "github.com/ardanlabs/kronk/sdk/tools/malina/libs"
	malinadownload "github.com/ardanlabs/malina/pkg/download"
	"github.com/hybridgroup/yzma/pkg/download"
)

func TestToAppLibIntegrity(t *testing.T) {
	arch, err := download.ParseArch("arm64")
	if err != nil {
		t.Fatalf("parse arch: %v", err)
	}
	opSys, err := download.ParseOS("darwin")
	if err != nil {
		t.Fatalf("parse os: %v", err)
	}
	processor, err := download.ParseProcessor("metal")
	if err != nil {
		t.Fatalf("parse processor: %v", err)
	}

	lib, err := libs.New(
		libs.WithBasePath(t.TempDir()),
		libs.WithArch(arch),
		libs.WithOS(opSys),
		libs.WithProcessor(processor),
	)
	if err != nil {
		t.Fatalf("new libs: %v", err)
	}

	report := libs.VerifyReport{
		Tag:        "b10786",
		Files:      []download.FileReport{{Name: "libllama.dylib", State: download.FileVerified}},
		Verified:   1,
		Unexpected: 1,
	}
	verifiedAt := time.Unix(123, 0).UTC()
	manifest := backend.BundleManifest{
		Version: backend.BundleManifestVersion,
		Digest:  "sha256:bundle",
		Files: []backend.BundleFile{
			{Name: "libllama.dylib", Kind: "file", Size: 42, SHA256: "file-sha"},
		},
	}

	got := toAppLibIntegrity(&report, lib, manifest, &verifiedAt, true)
	if got.Object != "lib_integrity" {
		t.Errorf("Object: got %q, want %q", got.Object, "lib_integrity")
	}
	if got.Version != "b10786" {
		t.Errorf("Version: got %q, want %q", got.Version, "b10786")
	}
	if got.Backend != "llama" {
		t.Errorf("Backend: got %q, want %q", got.Backend, "llama")
	}
	if !got.Verified || !got.ManifestAuthenticated {
		t.Errorf("Verification: got verified=%t authenticated=%t, want true/true", got.Verified, got.ManifestAuthenticated)
	}
	if got.BundleManifestVersion != backend.BundleManifestVersion || got.BundleDigest != "sha256:bundle" || !got.VerifiedAt.Equal(verifiedAt) {
		t.Errorf("Bundle evidence: got version=%q digest=%q verifiedAt=%v", got.BundleManifestVersion, got.BundleDigest, got.VerifiedAt)
	}
	if got.Arch != "arm64" || got.OS != "darwin" || got.Processor != "metal" {
		t.Errorf("triple: got %s/%s/%s, want arm64/darwin/metal", got.Arch, got.OS, got.Processor)
	}
	if len(got.Files) != 1 || got.Files[0].State != "verified" || got.Files[0].Size == nil || *got.Files[0].Size != 42 || got.Files[0].SHA256 != "file-sha" {
		t.Errorf("Files: got %+v, want one verified file", got.Files)
	}
}

func TestToAppBuckyLibIntegrity(t *testing.T) {
	lib, err := buckylibs.New(
		buckylibs.WithBasePath(t.TempDir()),
		buckylibs.WithArch("arm64"),
		buckylibs.WithOS("darwin"),
		buckylibs.WithProcessor("metal"),
	)
	if err != nil {
		t.Fatalf("new bucky libs: %v", err)
	}

	report := buckylibs.VerifyReport{
		Tag:                   "v1.9.3",
		ManifestAuthenticated: true,
		Source:                "publisher-manifest",
		Files:                 []buckydownload.FileReport{{Name: "libwhisper.dylib", State: buckydownload.FileVerified}},
		Verified:              1,
	}
	verifiedAt := time.Unix(123, 0).UTC()
	manifest := backend.BundleManifest{
		Version: backend.BundleManifestVersion,
		Digest:  "sha256:bundle",
		Files: []backend.BundleFile{
			{Name: "libwhisper.dylib", Kind: "file", Size: 42, SHA256: "file-sha"},
		},
	}

	got := toAppBuckyLibIntegrity(&report, lib, manifest, &verifiedAt)
	if got.Object != "lib_integrity" {
		t.Errorf("Object: got %q, want %q", got.Object, "lib_integrity")
	}
	if got.Version != "v1.9.3" {
		t.Errorf("Version: got %q, want %q", got.Version, "v1.9.3")
	}
	if got.Backend != "whisper" {
		t.Errorf("Backend: got %q, want %q", got.Backend, "whisper")
	}
	if !got.Verified || !got.ManifestAuthenticated {
		t.Errorf("verification: got verified=%t authenticated=%t, want true/true", got.Verified, got.ManifestAuthenticated)
	}
	if got.Source != "publisher-manifest" {
		t.Errorf("Source: got %q, want %q", got.Source, "publisher-manifest")
	}
	if got.Arch != "arm64" || got.OS != "darwin" || got.Processor != "metal" {
		t.Errorf("triple: got %s/%s/%s, want arm64/darwin/metal", got.Arch, got.OS, got.Processor)
	}
	if len(got.Files) != 1 || got.Files[0].State != "verified" {
		t.Errorf("Files: got %+v, want one verified file", got.Files)
	}
}

func TestToAppMalinaLibIntegrity(t *testing.T) {
	lib, err := malinalibs.New(
		malinalibs.WithBasePath(t.TempDir()),
		malinalibs.WithArch("arm64"),
		malinalibs.WithOS("darwin"),
		malinalibs.WithProcessor("metal"),
	)
	if err != nil {
		t.Fatalf("new malina libs: %v", err)
	}

	report := malinalibs.VerifyReport{
		Tag:                   "master-a1b2c3d",
		ManifestAuthenticated: true,
		Source:                "trusted-manifest",
		Files:                 []malinadownload.FileReport{{Name: "libstable-diffusion.dylib", State: malinadownload.FileVerified}},
		Verified:              1,
	}
	verifiedAt := time.Unix(123, 0).UTC()
	manifest := backend.BundleManifest{
		Version: backend.BundleManifestVersion,
		Digest:  "sha256:bundle",
		Files: []backend.BundleFile{
			{Name: "libstable-diffusion.dylib", Kind: "file", Size: 42, SHA256: "file-sha"},
		},
	}

	got := toAppMalinaLibIntegrity(&report, lib, manifest, &verifiedAt)
	if got.Backend != backend.KindStableDiffusion {
		t.Errorf("Backend: got %q, want %q", got.Backend, backend.KindStableDiffusion)
	}
	if !got.Verified || !got.ManifestAuthenticated {
		t.Errorf("Verification: got verified=%t authenticated=%t, want true/true", got.Verified, got.ManifestAuthenticated)
	}
	if got.BundleDigest != "sha256:bundle" || got.VerifiedAt == nil {
		t.Errorf("Bundle evidence: got digest=%q verifiedAt=%v", got.BundleDigest, got.VerifiedAt)
	}
	if len(got.Files) != 1 || got.Files[0].SHA256 != "file-sha" {
		t.Errorf("Files: got %+v, want one hashed file", got.Files)
	}
}
