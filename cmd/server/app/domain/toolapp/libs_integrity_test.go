package toolapp

import (
	"testing"

	buckydownload "github.com/ardanlabs/bucky/pkg/download"
	buckylibs "github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
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

	got := toAppLibIntegrity(&report, lib)
	if got.Object != "lib_integrity" {
		t.Errorf("Object: got %q, want %q", got.Object, "lib_integrity")
	}
	if got.Version != "b10786" {
		t.Errorf("Version: got %q, want %q", got.Version, "b10786")
	}
	if got.Backend != "llama" {
		t.Errorf("Backend: got %q, want %q", got.Backend, "llama")
	}
	if !got.Verified {
		t.Error("Verified: got false, want true")
	}
	if got.Arch != "arm64" || got.OS != "darwin" || got.Processor != "metal" {
		t.Errorf("triple: got %s/%s/%s, want arm64/darwin/metal", got.Arch, got.OS, got.Processor)
	}
	if len(got.Files) != 1 || got.Files[0].State != "verified" {
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

	got := toAppBuckyLibIntegrity(&report, lib)
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
