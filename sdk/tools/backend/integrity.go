package backend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const (
	// BundleManifestVersion identifies the canonical runtime-bundle manifest
	// encoding used to calculate BundleManifest.Digest.
	BundleManifestVersion = "kronk-runtime-v1"

	bundleFileKind    = "file"
	bundleSymlinkKind = "symlink"
)

// BundleFile identifies one path in a verified native runtime bundle.
type BundleFile struct {
	Name          string
	Kind          string
	Size          int64
	SHA256        string
	SymlinkTarget string
}

// BundleManifest provides a deterministic identity for a verified native
// runtime bundle.
type BundleManifest struct {
	Version string
	Digest  string
	Files   []BundleFile
}

// BuildBundleManifest hashes the supplied verified runtime paths into a
// deterministic manifest. Names must be slash-separated paths relative to
// root. Regular files contribute their path, size, and SHA-256; symbolic links
// contribute their path and link target.
func BuildBundleManifest(ctx context.Context, root string, names []string) (BundleManifest, error) {
	names = slices.Clone(names)
	slices.Sort(names)

	manifest := BundleManifest{
		Version: BundleManifestVersion,
		Files:   make([]BundleFile, 0, len(names)),
	}

	for _, name := range names {
		file, err := inspectBundleFile(ctx, root, name)
		if err != nil {
			return BundleManifest{}, err
		}
		manifest.Files = append(manifest.Files, file)
	}

	h := sha256.New()
	writeCanonicalField(h, manifest.Version)
	for _, file := range manifest.Files {
		writeCanonicalField(h, file.Kind)
		writeCanonicalField(h, file.Name)
		switch file.Kind {
		case bundleFileKind:
			writeCanonicalField(h, strconv.FormatInt(file.Size, 10))
			writeCanonicalField(h, file.SHA256)
		case bundleSymlinkKind:
			writeCanonicalField(h, file.SymlinkTarget)
		}
	}

	manifest.Digest = "sha256:" + hex.EncodeToString(h.Sum(nil))
	return manifest, nil
}

func inspectBundleFile(ctx context.Context, root string, name string) (BundleFile, error) {
	if err := ctx.Err(); err != nil {
		return BundleFile{}, fmt.Errorf("build-bundle-manifest: %w", err)
	}

	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(name)))
	if name == "" || clean != name || filepath.IsAbs(filepath.FromSlash(name)) || clean == ".." || strings.HasPrefix(clean, "../") {
		return BundleFile{}, fmt.Errorf("build-bundle-manifest: invalid relative path %q", name)
	}

	path := filepath.Join(root, filepath.FromSlash(name))
	info, err := os.Lstat(path)
	if err != nil {
		return BundleFile{}, fmt.Errorf("build-bundle-manifest: inspect %q: %w", name, err)
	}

	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return BundleFile{}, fmt.Errorf("build-bundle-manifest: read link %q: %w", name, err)
		}
		return BundleFile{Name: name, Kind: bundleSymlinkKind, SymlinkTarget: filepath.ToSlash(target)}, nil
	}
	if !info.Mode().IsRegular() {
		return BundleFile{}, fmt.Errorf("build-bundle-manifest: unsupported file type for %q", name)
	}

	f, err := os.Open(path)
	if err != nil {
		return BundleFile{}, fmt.Errorf("build-bundle-manifest: open %q: %w", name, err)
	}
	defer f.Close()

	h := sha256.New()
	buffer := make([]byte, 64*1024)
	for {
		if err := ctx.Err(); err != nil {
			return BundleFile{}, fmt.Errorf("build-bundle-manifest: hash %q: %w", name, err)
		}

		n, err := f.Read(buffer)
		if n > 0 {
			if _, writeErr := h.Write(buffer[:n]); writeErr != nil {
				return BundleFile{}, fmt.Errorf("build-bundle-manifest: hash %q: %w", name, writeErr)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return BundleFile{}, fmt.Errorf("build-bundle-manifest: read %q: %w", name, err)
		}
	}

	return BundleFile{
		Name:   name,
		Kind:   bundleFileKind,
		Size:   info.Size(),
		SHA256: hex.EncodeToString(h.Sum(nil)),
	}, nil
}

func writeCanonicalField(h hash.Hash, value string) {
	fmt.Fprintf(h, "%d:", len(value))
	h.Write([]byte(value))
}
