// Package install provides helper functions to install llama.cpp libraries
// and models.
package install

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/ardanlabs/kronk/install"
	"github.com/hybridgroup/yzma/pkg/download"
)

func Libraries(libPath string, processor download.Processor, allowUpgrade bool) error {
	orgVI, err := install.VersionInformation(libPath)
	if err != nil {
		return fmt.Errorf("error retrieving version info: %w", err)
	}

	fmt.Println()
	fmt.Println("- libPath   :", libPath)
	fmt.Println("- processor :", processor)
	fmt.Print("- check llama.cpp installation: ")

	if orgVI.Current == orgVI.Latest {
		fmt.Println("✓")
		fmt.Printf("  - latest version : %s\n  - current version: %s\n", orgVI.Latest, orgVI.Current)
		return nil
	}

	fmt.Println("✓")

	vi, err := install.Libraries(libPath, processor, true)
	if err != nil {
		fmt.Println("x")

		if _, err := install.InstalledVersion(libPath); err != nil {
			return fmt.Errorf("failed to install llama: %q: error: %w", libPath, err)
		}

		fmt.Println("  - failed to install new version, using current version")
	}

	fmt.Print("- updated llama.cpp installation: ")
	fmt.Println("✓")
	fmt.Printf("  - old version    : %s\n  - current version: %s\n", orgVI.Current, vi.Current)
	return nil
}

func Model(modelURL string, modelPath string) (string, error) {
	u, _ := url.Parse(modelURL)
	filename := path.Base(u.Path)
	name := strings.TrimSuffix(filename, path.Ext(filename))
	fmt.Printf("- check %q installation: ", name)

	f := func(src string, currentSize int64, totalSize int64, mibPerSec float64, complete bool) {
		fmt.Printf("\r\x1b[KDownloading %s... %d MiB of %d MiB (%.2f MiB/s)", src, currentSize/(1024*1024), totalSize/(1024*1024), mibPerSec)
		if complete {
			fmt.Println()
		}
	}

	localPath, downloaded, err := install.ModelWithProgress(modelURL, modelPath, f)
	if err != nil {
		fmt.Printf("\r\x1b[K- check %q installation: x\n", name)
		return "", fmt.Errorf("unable to download model: %w", err)
	}

	fmt.Print("✓")
	if downloaded {
		fmt.Printf("\r\x1b[K- check %q installation: ✓", name)
	}
	fmt.Println()

	return localPath, nil
}
