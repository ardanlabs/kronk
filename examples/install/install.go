// Package install provides helper functions to install llama.cpp libraries
// and models.
package install

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
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
	fmt.Print("- check llama.cpp installation: ")

	if orgVI.Current == orgVI.Latest {
		fmt.Println("✓")
		fmt.Printf("  - latest version : %s\n  - current version: %s\n", orgVI.Latest, orgVI.Current)
		return nil
	}

	fmt.Println("✓")

	vi, err := install.Libraries(libPath, download.CPU, true)
	if err != nil {
		fmt.Println("x")

		if _, err := install.InstalledVersion(libPath); err != nil {
			return fmt.Errorf("failed to install llama: %q: error: %w", libPath, err)
		}

		fmt.Println("  - failed to install new version, using current version")
	}

	f := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		fmt.Println("lib:", path)
		return nil
	}

	if err := filepath.Walk(libPath, f); err != nil {
		fmt.Println("x")
		return fmt.Errorf("error walking model path: %v", err)
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

	localPath, err := install.Model(modelURL, modelPath)
	if err != nil {
		fmt.Println("X")
		return "", fmt.Errorf("unable to download model: %w", err)
	}

	fmt.Println("✓")

	return localPath, nil
}
