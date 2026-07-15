package download

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	getter "github.com/hashicorp/go-getter"
)

var (
	ErrUnknownArch         = errors.New("unknown architecture")
	ErrUnknownOS           = errors.New("unknown OS")
	ErrUnknownProcessor    = errors.New("unknown processor")
	ErrInvalidVersion      = errors.New("invalid version")
	ErrFileNotFound        = errors.New("could not download file: the requested whisper.cpp version may still be building for your platform")
	ErrUnsupportedPlatform = errors.New("no prebuilt whisper.cpp asset for this platform")
)

// BuckyBuilderRepo is the GitHub repo serving prebuilt whisper.cpp
// libraries for Linux, macOS, and Windows CPU. whisper.cpp upstream
// publishes no Linux release artifacts at all, and we mirror their
// macOS xcframework + Windows CPU x64 zip there too so every bucky
// download lines up against the same tag with the same build provenance.
// Only Windows CUDA still comes straight from upstream. See
// https://github.com/ardanlabs/bucky-builder for the build matrix.
const BuckyBuilderRepo = "ardanlabs/bucky-builder"

// DefaultWhisperVersion is the well-known whisper.cpp release tag bucky's
// FFI struct mirrors (e.g. WhisperFullParams's 304-byte layout) are tested
// against. `bucky install` uses this when no -v flag is supplied so first
// installs and CI runs do not depend on the GitHub releases API. Bumping
// this value is a deliberate, reviewable change that should be paired with
// re-running the FFI sizeof + by-ref/by-value tests in pkg/whisper.
const DefaultWhisperVersion = "v1.9.1"

var (
	// RetryCount is how many times the package will retry to obtain the latest whisper.cpp version.
	RetryCount = 3
	// RetryDelay is the delay between retries when obtaining the latest whisper.cpp version.
	RetryDelay = 3 * time.Second
	// versionURL is the URL serving the latest whisper.cpp tag bucky-builder
	// has produced Linux artifacts for. We deliberately do NOT hit the
	// GitHub releases API here — that endpoint is rate-limited per IP,
	// which bit our macOS CI run. The Pages-hosted version.json is
	// republished by bucky-builder's publish-version workflow whenever a
	// new whisper.cpp release ships.
	versionURL = "https://ardanlabs.github.io/bucky-builder/version.json"
)

// WhisperLatestVersion fetches the latest whisper.cpp release tag bucky knows
// about. This is sourced from bucky-builder's GitHub Pages, NOT from
// whisper.cpp upstream directly, so the value reflects what bucky-builder
// has built + tested across Linux, macOS, and Windows CPU.
func WhisperLatestVersion() (string, error) {
	var version string
	var err error
	for range RetryCount {
		version, err = getLatestVersion()
		if err == nil {
			return version, nil
		}
		time.Sleep(RetryDelay)
	}

	return "", errors.New("unable to fetch latest version")
}

func getLatestVersion() (string, error) {
	req, err := http.NewRequest("GET", versionURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("received status code %d from version URL: %s", resp.StatusCode, string(body))
	}

	var result struct {
		TagName string `json:"tag_name"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.TagName, nil
}

// getDownloadLocationAndFilename returns the download URL location and the
// asset filename for the given parameters.
func getDownloadLocationAndFilename(arch Arch, os OS, prcssr Processor, version string) (location, filename string, err error) {
	buckyBuilder := fmt.Sprintf("https://github.com/%s/releases/download/%s", BuckyBuilderRepo, version)
	upstream := fmt.Sprintf("https://github.com/ggml-org/whisper.cpp/releases/download/%s", version)

	switch os {
	case Darwin:
		// bucky-builder packages the macOS slice of whisper.cpp's
		// xcframework (universal arm64+x86_64, Metal + CoreML) so the
		// download is small. extractDarwinXCFramework still expects the
		// build-apple/whisper.xcframework/macos-arm64_x86_64/... layout.
		switch prcssr {
		case CPU, Metal:
			location = buckyBuilder
			filename = fmt.Sprintf("whisper-%s-bin-darwin-metal-universal.zip", version)
		default:
			return "", "", fmt.Errorf("%w: darwin only supports cpu/metal", ErrUnknownProcessor)
		}

	case Windows:
		if arch != AMD64 {
			return "", "", fmt.Errorf("%w: windows %s not supported in v1", ErrUnsupportedPlatform, arch)
		}
		switch prcssr {
		case CPU:
			// CPU x64 is built by bucky-builder with the same flag set
			// as Linux (GGML_CPU_ALL_VARIANTS=ON, GGML_BACKEND_DL=ON).
			location = buckyBuilder
			filename = fmt.Sprintf("whisper-%s-bin-windows-cpu-x64.zip", version)
		case CUDA:
			// CUDA is still pulled straight from whisper.cpp upstream;
			// bucky-builder does not build Windows CUDA yet.
			location = upstream
			filename = "whisper-cublas-12.4.0-bin-x64.zip"
		default:
			return "", "", fmt.Errorf("%w: windows supports cpu/cuda", ErrUnknownProcessor)
		}

	case Linux:
		// Filename pattern is whisper-<TAG>-bin-ubuntu-<backend>-<arch>
		// .tar.gz; both AMD64 and ARM64 are supported across cpu/cuda
		// /vulkan.
		location = buckyBuilder

		var archStr string
		switch arch {
		case AMD64:
			archStr = "x64"
		case ARM64:
			archStr = "arm64"
		default:
			return "", "", fmt.Errorf("%w: linux %s not supported", ErrUnsupportedPlatform, arch)
		}

		switch prcssr {
		case CPU, CUDA, Vulkan:
			filename = fmt.Sprintf("whisper-%s-bin-ubuntu-%s-%s.tar.gz", version, prcssr, archStr)
		default:
			return "", "", fmt.Errorf("%w: linux supports cpu/cuda/vulkan", ErrUnknownProcessor)
		}

	default:
		return "", "", ErrUnknownOS
	}

	return location, filename, nil
}

// getFunc is the function used to download asset files. It can be overridden for testing.
var getFunc = get

// Get downloads the whisper.cpp precompiled binaries for the desired arch/OS/processor.
//
//	arch:      "amd64" or "arm64"
//	os:        "linux", "darwin", or "windows"
//	processor: "cpu", "cuda", "metal", or "vulkan"
//	version:   the desired whisper.cpp release tag, e.g. "v1.9.1"
//	dest:      destination directory for the extracted libraries
func Get(architecture string, operatingSystem string, processor string, version string, dest string) error {
	return GetWithProgress(architecture, operatingSystem, processor, version, dest, ProgressTracker)
}

// GetWithProgress downloads the whisper.cpp precompiled binaries using the
// provided progress tracker.
func GetWithProgress(architecture string, operatingSystem string, processor string, version string, dest string, progress getter.ProgressTracker) error {
	return GetWithContext(context.Background(), architecture, operatingSystem, processor, version, dest, progress)
}

// GetWithContext downloads the whisper.cpp precompiled binaries using the
// provided context and progress tracker.
func GetWithContext(ctx context.Context, architecture string, operatingSystem string, processor string, version string, dest string, progress getter.ProgressTracker) error {
	arch, err := ParseArch(architecture)
	if err != nil {
		return ErrUnknownArch
	}

	osVal, err := ParseOS(operatingSystem)
	if err != nil {
		return ErrUnknownOS
	}

	prcssr, err := ParseProcessor(processor)
	if err != nil {
		return ErrUnknownProcessor
	}

	if err := VersionIsValid(version); err != nil {
		return ErrInvalidVersion
	}

	location, filename, err := getDownloadLocationAndFilename(arch, osVal, prcssr, version)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/%s", location, filename)
	return getFunc(ctx, url, dest, osVal, progress)
}

// get downloads the asset zip and extracts the relevant whisper library file(s)
// into dest. The extraction logic differs per OS because whisper.cpp ships
// platform-specific archive layouts.
func get(ctx context.Context, url, dest string, osVal OS, progress getter.ProgressTracker) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("failed to create destination dir: %w", err)
	}

	downloadFile := filepath.Join(dest, filepath.Base(url))

	client := &getter.Client{
		Ctx:  ctx,
		Src:  url + "?archive=false",
		Dst:  dest,
		Mode: getter.ClientModeAny,
	}

	if progress != nil {
		client.ProgressListener = progress
	}

	if err := client.Get(); err != nil {
		if strings.Contains(err.Error(), "404") {
			return fmt.Errorf("%w: %s", ErrFileNotFound, url)
		}
		return err
	}
	defer os.Remove(downloadFile)

	switch osVal {
	case Darwin:
		return extractDarwinXCFramework(downloadFile, dest)
	case Windows:
		return extractWindowsZip(downloadFile, dest)
	case Linux:
		return extractLinuxTarGz(downloadFile, dest)
	default:
		return fmt.Errorf("%w: extraction not implemented for %s", ErrUnsupportedPlatform, osVal)
	}
}

// extractLinuxTarGz pulls libwhisper.so + libggml*.so out of a bucky-builder
// .tar.gz and writes them flat into dest. Archive layout (set by the
// builder's `tar --transform "s,./,whisper-<TAG>/,"`) is:
//
//	whisper-vX.Y.Z/libwhisper.so
//	whisper-vX.Y.Z/libwhisper.so.1     (symlink)
//	whisper-vX.Y.Z/libggml.so
//	whisper-vX.Y.Z/libggml-base.so
//	whisper-vX.Y.Z/libggml-cpu.so
//	whisper-vX.Y.Z/libggml-cpu-*.so    (per-microarch variants from
//	                                    GGML_CPU_ALL_VARIANTS=ON; the
//	                                    ggml backend registry picks the
//	                                    best one at runtime)
//	whisper-vX.Y.Z/libggml-cuda.so     (cuda variant only)
//	whisper-vX.Y.Z/libggml-vulkan.so   (vulkan variant only)
//
// The leading whisper-vX.Y.Z/ component is stripped on extract so callers
// can point BUCKY_LIB straight at dest.
func extractLinuxTarGz(tgzPath, dest string) error {
	f, err := os.Open(tgzPath)
	if err != nil {
		return fmt.Errorf("failed to open tar.gz: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to open gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	any := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}
		wrote, err := writeTarEntry(hdr, tr, dest)
		if err != nil {
			return err
		}
		if wrote {
			any = true
		}
	}

	if !any {
		return errors.New("linux tar.gz contained no regular files")
	}
	return nil
}

// writeTarEntry strips the leading whisper-<TAG>/ path component and writes
// the entry to dest. Returns true if a regular file was written. Anything we
// don't understand (hardlinks, devices, fifos) is silently skipped.
func writeTarEntry(hdr *tar.Header, tr *tar.Reader, dest string) (bool, error) {
	// Strip the leading whisper-<TAG>/ component. Top-level dir entries
	// are skipped; top-level loose files are treated as flat.
	name := strings.TrimLeft(hdr.Name, "./")
	if i := strings.Index(name, "/"); i >= 0 {
		name = name[i+1:]
	} else if hdr.Typeflag == tar.TypeDir {
		return false, nil
	}
	if name == "" {
		return false, nil
	}

	target := filepath.Join(dest, name)
	switch hdr.Typeflag {
	case tar.TypeDir:
		if err := os.MkdirAll(target, 0o755); err != nil {
			return false, fmt.Errorf("mkdir %s: %w", target, err)
		}
		return false, nil
	case tar.TypeReg:
		if err := writeTarRegular(target, hdr, tr); err != nil {
			return false, err
		}
		return true, nil
	case tar.TypeSymlink:
		// Library SONAME symlinks (e.g. libwhisper.so -> libwhisper.so.1)
		// must be preserved or dlopen will fail at runtime.
		return false, writeTarSymlink(target, hdr)
	}
	return false, nil
}

func writeTarRegular(target string, hdr *tar.Header, tr *tar.Reader) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("mkdir parent of %s: %w", target, err)
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(hdr.Mode)&0o777)
	if err != nil {
		return fmt.Errorf("create %s: %w", target, err)
	}
	if _, err := io.Copy(out, tr); err != nil {
		out.Close()
		return fmt.Errorf("write %s: %w", target, err)
	}
	return out.Close()
}

func writeTarSymlink(target string, hdr *tar.Header) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("mkdir parent of %s: %w", target, err)
	}
	_ = os.Remove(target) // overwrite if present
	if err := os.Symlink(hdr.Linkname, target); err != nil {
		return fmt.Errorf("symlink %s -> %s: %w", target, hdr.Linkname, err)
	}
	return nil
}

// extractDarwinXCFramework pulls the macos-arm64_x86_64 universal dylib out
// of the xcframework zip and writes it to dest as libwhisper.dylib.
func extractDarwinXCFramework(zipPath, dest string) error {
	const wantPath = "build-apple/whisper.xcframework/macos-arm64_x86_64/whisper.framework/Versions/A/whisper"

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open xcframework zip: %w", err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.Name != wantPath {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to open dylib in zip: %w", err)
		}
		out, err := os.OpenFile(filepath.Join(dest, "libwhisper.dylib"), os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o755)
		if err != nil {
			rc.Close()
			return fmt.Errorf("failed to create libwhisper.dylib: %w", err)
		}
		if _, err := io.Copy(out, rc); err != nil {
			rc.Close()
			out.Close()
			return fmt.Errorf("failed to write libwhisper.dylib: %w", err)
		}
		rc.Close()
		out.Close()
		return nil
	}

	return fmt.Errorf("xcframework zip did not contain %s", wantPath)
}

// extractWindowsZip pulls all DLLs out of the Release/ directory of the
// windows release zip and writes them to dest.
func extractWindowsZip(zipPath, dest string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open windows zip: %w", err)
	}
	defer zr.Close()

	any := false
	for _, f := range zr.File {
		// Only extract DLLs from the Release/ directory.
		base := filepath.Base(f.Name)
		if !strings.HasSuffix(strings.ToLower(base), ".dll") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to open %s in zip: %w", f.Name, err)
		}
		target := filepath.Join(dest, base)
		out, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o755)
		if err != nil {
			rc.Close()
			return fmt.Errorf("failed to create %s: %w", target, err)
		}
		if _, err := io.Copy(out, rc); err != nil {
			rc.Close()
			out.Close()
			return fmt.Errorf("failed to write %s: %w", target, err)
		}
		rc.Close()
		out.Close()
		any = true
	}

	if !any {
		return errors.New("windows zip contained no DLL files")
	}

	return nil
}

// VersionIsValid checks if the provided version string looks like a
// whisper.cpp release tag (e.g. "v1.9.1").
func VersionIsValid(version string) error {
	if !strings.HasPrefix(version, "v") {
		return ErrInvalidVersion
	}
	// Cheap shape check: must contain at least one dot.
	if !strings.Contains(version, ".") {
		return ErrInvalidVersion
	}
	return nil
}

// LibraryName returns the filename for the whisper.cpp library on the given OS.
func LibraryName(operatingSystem string) string {
	osVal, err := ParseOS(operatingSystem)
	if err != nil {
		return "unknown"
	}

	switch osVal {
	case Linux:
		return "libwhisper.so"
	case Windows:
		return "whisper.dll"
	case Darwin:
		return "libwhisper.dylib"
	default:
		return "unknown"
	}
}
