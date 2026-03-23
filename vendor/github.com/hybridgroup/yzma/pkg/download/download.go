package download

import (
	"archive/tar"
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
	ErrUnknownArch      = errors.New("unknown architecture")
	ErrUnknownOS        = errors.New("unknown OS")
	ErrUnknownProcessor = errors.New("unknown processor")
	ErrInvalidVersion   = errors.New("invalid version")
	ErrFileNotFound     = errors.New("could not download file: the requested llama.cpp version may still be building for your platform.")
)

var (
	// RetryCount is how many times the package will retry to obtain the latest llama.cpp version.
	RetryCount = 3
	// RetryDelay is the delay between retries when obtaining the latest llama.cpp version.
	RetryDelay = 3 * time.Second
	// apiURL is the GitHub API URL for fetching the latest llama.cpp version.
	// We use the llama-cpp-builder repo instead of the original llama.cpp repo because
	// we need the precompiled binaries for certain platforms, and the build server might be
	// up to 1 hour out of sync with the latest commits to the original llama.cpp repo.
	//
	// Actual downloads will be from the llama.cpp repo for any builds that are available there,
	// and from the llama-cpp-builder repo for builds that are not available in the original repo
	// (e.g. ARM64 CUDA builds). This is handled in the getDownloadLocationAndFilename function.
	apiURL = "https://api.github.com/repos/hybridgroup/llama-cpp-builder/releases/latest"
)

// LlamaLatestVersion fetches the latest release tag of llama.cpp from the GitHub API.
func LlamaLatestVersion() (string, error) {
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
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", err
	}

	// Set required headers for GitHub API
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

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
		return "", fmt.Errorf("received status code %d from GitHub API: %s", resp.StatusCode, string(body))
	}

	var result struct {
		TagName string `json:"tag_name"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.TagName, nil
}

// getDownloadLocationAndFilename returns the download location and filename for the given parameters.
func getDownloadLocationAndFilename(arch Arch, os OS, prcssr Processor, version string, dest string) (location, filename string, err error) {
	location = fmt.Sprintf("https://github.com/ggml-org/llama.cpp/releases/download/%s", version)

	switch os {
	case Linux:
		switch prcssr {
		case CPU:
			if arch == ARM64 {
				location = fmt.Sprintf("https://github.com/hybridgroup/llama-cpp-builder/releases/download/%s", version)
				filename = fmt.Sprintf("llama-%s-bin-ubuntu-cpu-arm64.tar.gz", version)
				break
			}
			filename = fmt.Sprintf("llama-%s-bin-ubuntu-x64.tar.gz", version)
		case CUDA:
			location = fmt.Sprintf("https://github.com/hybridgroup/llama-cpp-builder/releases/download/%s", version)
			if arch == ARM64 {
				// defaults to CUDA 12 assuming that is running Jetson Orin.
				filename = fmt.Sprintf("llama-%s-bin-ubuntu-cuda-arm64.tar.gz", version)
			} else {
				filename = fmt.Sprintf("llama-%s-bin-ubuntu-cuda-13-x64.tar.gz", version)
			}
		case Vulkan:
			if arch == ARM64 {
				location = fmt.Sprintf("https://github.com/hybridgroup/llama-cpp-builder/releases/download/%s", version)
				filename = fmt.Sprintf("llama-%s-bin-ubuntu-vulkan-arm64.tar.gz", version)
				break
			}
			filename = fmt.Sprintf("llama-%s-bin-ubuntu-vulkan-x64.tar.gz", version)
		case ROCm:
			if arch != AMD64 {
				return "", "", errors.New("precompiled binaries for Linux ARM64 ROCm are not available")
			}
			filename = fmt.Sprintf("llama-%s-bin-ubuntu-rocm-7.2-x64.tar.gz", version)
		default:
			return "", "", ErrUnknownProcessor
		}

	case Bookworm:
		switch prcssr {
		case CPU:
			if arch == ARM64 {
				location = fmt.Sprintf("https://github.com/hybridgroup/llama-cpp-builder/releases/download/%s", version)
				filename = fmt.Sprintf("llama-%s-bin-ubuntu-cpu-arm64.tar.gz", version)
				break
			}

			// no AMD64 for bookworm
			return "", "", ErrUnknownProcessor
		case CUDA:
			location = fmt.Sprintf("https://github.com/hybridgroup/llama-cpp-builder/releases/download/%s", version)
			if arch == ARM64 {
				// Jetson Orin.
				filename = fmt.Sprintf("llama-%s-bin-ubuntu-cuda-arm64.tar.gz", version)
				break
			}

			// no AMD64 for bookworm
			return "", "", ErrUnknownProcessor
		case Vulkan:
			if arch == ARM64 {
				location = fmt.Sprintf("https://github.com/hybridgroup/llama-cpp-builder/releases/download/%s", version)
				filename = fmt.Sprintf("llama-%s-bin-ubuntu-vulkan-arm64.tar.gz", version)
				break
			}

			// no AMD64 for bookworm
			return "", "", ErrUnknownProcessor
		default:
			return "", "", ErrUnknownProcessor
		}

	case Trixie:
		switch prcssr {
		case CPU:
			if arch == ARM64 {
				location = fmt.Sprintf("https://github.com/hybridgroup/llama-cpp-builder/releases/download/%s", version)
				filename = fmt.Sprintf("llama-%s-bin-ubuntu-trixie-cpu-arm64.tar.gz", version)
				break
			}
			filename = fmt.Sprintf("llama-%s-bin-ubuntu-x64.tar.gz", version)
		case CUDA:
			location = fmt.Sprintf("https://github.com/hybridgroup/llama-cpp-builder/releases/download/%s", version)
			if arch == ARM64 {
				// not yet
				return "", "", ErrUnknownProcessor
			} else {
				filename = fmt.Sprintf("llama-%s-bin-ubuntu-cuda-13-x64.tar.gz", version)
			}
		case Vulkan:
			if arch == ARM64 {
				location = fmt.Sprintf("https://github.com/hybridgroup/llama-cpp-builder/releases/download/%s", version)
				filename = fmt.Sprintf("llama-%s-bin-ubuntu-trixie-vulkan-arm64.tar.gz", version)
				break
			}
			filename = fmt.Sprintf("llama-%s-bin-ubuntu-vulkan-x64.tar.gz", version)
		default:
			return "", "", ErrUnknownProcessor
		}

	case Darwin:
		switch prcssr {
		case Metal:
			if arch != ARM64 {
				return "", "", errors.New("precompiled binaries for macOS non-ARM64 CPU/Metal are not available")
			}
			filename = fmt.Sprintf("llama-%s-bin-macos-arm64.tar.gz", version)
		case CPU:
			if arch == ARM64 {
				filename = fmt.Sprintf("llama-%s-bin-macos-arm64.tar.gz", version)
			} else {
				filename = fmt.Sprintf("llama-%s-bin-macos-x64.tar.gz", version)
			}
		default:
			return "", "", ErrUnknownProcessor
		}

	case Windows:
		switch prcssr {
		case CPU:
			if arch == ARM64 {
				filename = fmt.Sprintf("llama-%s-bin-win-cpu-arm64.zip", version)
			} else {
				filename = fmt.Sprintf("llama-%s-bin-win-cpu-x64.zip", version)
			}
		case CUDA:
			if arch == ARM64 {
				return "", "", errors.New("precompiled binaries for Windows ARM64 CUDA are not available")
			}
			// also requires the CUDA RT files
			cudart := "cudart-llama-bin-win-cuda-13.1-x64.zip"
			url := fmt.Sprintf("%s/%s", location, cudart)
			if err := get(context.Background(), url, dest, ProgressTracker); err != nil {
				return "", "", err
			}
			filename = fmt.Sprintf("llama-%s-bin-win-cuda-13.1-x64.zip", version)
		case Vulkan:
			if arch == ARM64 {
				return "", "", errors.New("precompiled binaries for Windows ARM64 Vulkan are not available")
			}
			filename = fmt.Sprintf("llama-%s-bin-win-vulkan-x64.zip", version)
		case ROCm:
			if arch != AMD64 {
				return "", "", errors.New("precompiled binaries for Windows ARM64 ROCm are not available")
			}
			filename = fmt.Sprintf("llama-%s-bin-win-hip-radeon-x64.zip", version)
		default:
			return "", "", ErrUnknownProcessor
		}

	default:
		return "", "", ErrUnknownOS
	}

	return location, filename, nil
}

// getFunc is the function used to download files. It can be overridden for testing.
var getFunc = get

// Get downloads the llama.cpp precompiled binaries for the desired arch/OS/processor.
// arch can be one of the following values: "amd64", "arm64".
// os can be one of the following values: "linux", "darwin", "windows", "bookworm", "trixie".
// processor can be one of the following values: "cpu", "cuda", "metal", "rocm", "vulkan".
// version should be the desired `b1234` formatted llama.cpp version. You can use the
// [LlamaLatestVersion] function to obtain the latest release.
// dest in the destination directory for the downloaded binaries.
func Get(architecture string, operatingSystem string, processor string, version string, dest string) error {
	return GetWithProgress(architecture, operatingSystem, processor, version, dest, ProgressTracker)
}

// GetWithProgress downloads the llama.cpp precompiled binaries for the desired arch/OS/processor
// using the provided progress tracker.
// arch can be one of the following values: "amd64", "arm64".
// os can be one of the following values: "linux", "darwin", "windows", "bookworm", "trixie".
// processor can be one of the following values: "cpu", "cuda", "metal", "rocm", "vulkan".
// version should be the desired `b1234` formatted llama.cpp version. You can use the
// [LlamaLatestVersion] function to obtain the latest release.
// dest in the destination directory for the downloaded binaries.
func GetWithProgress(architecture string, operatingSystem string, processor string, version string, dest string, progress getter.ProgressTracker) error {
	return GetWithContext(context.Background(), architecture, operatingSystem, processor, version, dest, progress)
}

// GetWithContext downloads the llama.cpp precompiled binaries for the desired arch/OS/processor
// using the provided context and progress tracker.
// arch can be one of the following values: "amd64", "arm64".
// os can be one of the following values: "linux", "darwin", "windows", "bookworm", "trixie".
// processor can be one of the following values: "cpu", "cuda", "metal", "rocm", "vulkan".
// version should be the desired `b1234` formatted llama.cpp version. You can use the
// [LlamaLatestVersion] function to obtain the latest release.
// dest in the destination directory for the downloaded binaries.
func GetWithContext(ctx context.Context, architecture string, operatingSystem string, processor string, version string, dest string, progress getter.ProgressTracker) error {
	arch, err := ParseArch(architecture)
	if err != nil {
		return ErrUnknownArch
	}

	os, err := ParseOS(operatingSystem)
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

	location, filename, err := getDownloadLocationAndFilename(arch, os, prcssr, version, dest)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/%s", location, filename)
	return getFunc(ctx, url, dest, progress)
}

func get(ctx context.Context, url, dest string, progress getter.ProgressTracker) error {
	// Check if it's a .tar.gz file
	if strings.HasSuffix(url, ".tar.gz") {
		err := downloadAndExtractTarGz(url, dest, progress)
		if err != nil && strings.Contains(err.Error(), "404") {
			return fmt.Errorf("%w: %s", ErrFileNotFound, url)
		}
		return err
	}

	// Use go-getter for other file types (e.g., .zip)
	client := &getter.Client{
		Ctx:  ctx,
		Src:  url,
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

	return nil
}

// downloadAndExtractTarGz downloads a .tar.gz file and extracts it to the destination directory.
func downloadAndExtractTarGz(url, dest string, progress getter.ProgressTracker) error {
	downloadFile := filepath.Join(dest, filepath.Base(url))

	client := &getter.Client{
		Ctx:  context.Background(),
		Src:  url + "?archive=false",
		Dst:  dest,
		Mode: getter.ClientModeAny,
	}

	if progress != nil {
		client.ProgressListener = progress
	}

	if err := client.Get(); err != nil {
		// Check for 404 errors specifically
		if strings.Contains(err.Error(), "404") {
			return fmt.Errorf("404 not found: %s", url)
		}
		return err
	}
	defer os.Remove(downloadFile)

	resp, err := os.Open(downloadFile)
	if err != nil {
		return fmt.Errorf("failed to open downloaded file: %w", err)
	}
	defer resp.Close()

	// Create gzip reader
	gzr, err := gzip.NewReader(resp)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	// Create tar reader
	tr := tar.NewReader(gzr)

	// Extract files
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		// Strip the top-level directory (e.g., "llama-b1234/")
		name := header.Name
		if idx := strings.Index(name, "/"); idx != -1 {
			name = name[idx+1:]
		}

		// Skip empty names (the top-level directory itself)
		if name == "" {
			continue
		}

		target := filepath.Join(dest, filepath.Clean(name))

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg:
			// Ensure parent directory exists
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return fmt.Errorf("failed to create parent directory: %w", err)
			}

			// Create the file
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}

			// Copy contents
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return fmt.Errorf("failed to write file: %w", err)
			}
			f.Close()
		case tar.TypeSymlink:
			// Handle symlinks
			if err := os.Symlink(header.Linkname, target); err != nil {
				// Ignore error if symlink already exists
				if !os.IsExist(err) {
					return fmt.Errorf("failed to create symlink: %w", err)
				}
			}
		}
	}

	return nil
}

// VersionIsValid checks if the provided version string is valid.
func VersionIsValid(version string) error {
	if !strings.HasPrefix(version, "b") {
		return ErrInvalidVersion
	}

	return nil
}

// LibraryName returns the name for the llama.cpp library for any given OS.
func LibraryName(operatingSystem string) string {
	os, err := ParseOS(operatingSystem)
	if err != nil {
		return "unknown"
	}

	switch os {
	case Linux:
		return "libllama.so"
	case Windows:
		return "llama.dll"
	case Darwin:
		return "libllama.dylib"
	default:
		return "unknown"
	}
}
