package libs

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/sdk/tools/downloader"
)

const (
	// BundleZipFile is the canonical filename of the cached zip archive
	// inside a bundle directory.
	BundleZipFile = "bundle.zip"

	// BundleSHAFile is the canonical filename of the sha256 digest written
	// alongside BundleZipFile.
	BundleSHAFile = "bundle.zip.sha256"
)

// bundleBuildLocks serializes concurrent BuildBundleZip requests for the
// same (os, arch, processor) triple so that a workshop full of clients
// pulling the same bundle does not produce N parallel zip builds.
var (
	bundleBuildLocksMu sync.Mutex
	bundleBuildLocks   = map[string]*sync.Mutex{}
)

func bundleBuildLock(key string) *sync.Mutex {
	bundleBuildLocksMu.Lock()
	defer bundleBuildLocksMu.Unlock()

	mu, ok := bundleBuildLocks[key]
	if !ok {
		mu = &sync.Mutex{}
		bundleBuildLocks[key] = mu
	}

	return mu
}

// BundleArtifacts describes the on-disk artifacts associated with a single
// installed bundle. Size and SHA256 are populated only when the cached zip
// already exists.
type BundleArtifacts struct {
	Arch      string
	OS        string
	Processor string
	Version   string
	ZipPath   string
	SHAPath   string
	Size      int64
	SHA256    string
}

// BundleArtifacts returns the on-disk paths and metadata of the cached zip
// for the supplied (arch, os, processor) triple. If the bundle directory or
// its version.json cannot be read, an error is returned. Size and SHA256
// are non-zero only when the cached zip already exists.
func (lib *Libs) BundleArtifacts(arch string, opSys string, processor string) (BundleArtifacts, error) {
	a, o, p, err := parseTriple(arch, opSys, processor)
	if err != nil {
		return BundleArtifacts{}, fmt.Errorf("bundle-artifacts: %w", err)
	}

	bundleDir := installPathFor(lib.root, a, o, p)
	tag, err := readVersionFile(bundleDir)
	if err != nil {
		return BundleArtifacts{}, fmt.Errorf("bundle-artifacts: %w", err)
	}

	out := BundleArtifacts{
		Arch:      tag.Arch,
		OS:        tag.OS,
		Processor: tag.Processor,
		Version:   tag.Version,
		ZipPath:   filepath.Join(bundleDir, BundleZipFile),
		SHAPath:   filepath.Join(bundleDir, BundleSHAFile),
	}

	if info, err := os.Stat(out.ZipPath); err == nil {
		out.Size = info.Size()
	}

	if data, err := os.ReadFile(out.SHAPath); err == nil {
		out.SHA256 = strings.TrimSpace(string(data))
	}

	return out, nil
}

// BuildBundleZip ensures a zip archive of the installed bundle for the
// supplied (arch, os, processor) triple exists on disk at
// <bundleDir>/bundle.zip, with its sha256 digest written to
// <bundleDir>/bundle.zip.sha256. If the bundle is not installed, an error
// is returned. Concurrent calls for the same triple are serialized.
func (lib *Libs) BuildBundleZip(arch string, opSys string, processor string) (BundleArtifacts, error) {
	a, o, p, err := parseTriple(arch, opSys, processor)
	if err != nil {
		return BundleArtifacts{}, fmt.Errorf("build-bundle-zip: %w", err)
	}

	bundleDir := installPathFor(lib.root, a, o, p)

	tag, err := readVersionFile(bundleDir)
	if err != nil {
		return BundleArtifacts{}, fmt.Errorf("build-bundle-zip: bundle not installed: %w", err)
	}

	key := bundleDir
	mu := bundleBuildLock(key)
	mu.Lock()
	defer mu.Unlock()

	zipPath := filepath.Join(bundleDir, BundleZipFile)
	shaPath := filepath.Join(bundleDir, BundleSHAFile)

	// Cached: zip + sha already exist.
	if zipInfo, zerr := os.Stat(zipPath); zerr == nil {
		if shaData, serr := os.ReadFile(shaPath); serr == nil {
			return BundleArtifacts{
				Arch:      tag.Arch,
				OS:        tag.OS,
				Processor: tag.Processor,
				Version:   tag.Version,
				ZipPath:   zipPath,
				SHAPath:   shaPath,
				Size:      zipInfo.Size(),
				SHA256:    strings.TrimSpace(string(shaData)),
			}, nil
		}
	}

	tmpZip := zipPath + ".tmp"
	tmpSHA := shaPath + ".tmp"
	os.Remove(tmpZip)
	os.Remove(tmpSHA)

	sum, size, err := writeBundleZip(bundleDir, tmpZip)
	if err != nil {
		os.Remove(tmpZip)
		return BundleArtifacts{}, fmt.Errorf("build-bundle-zip: %w", err)
	}

	if err := os.WriteFile(tmpSHA, []byte(sum+"\n"), 0o644); err != nil {
		os.Remove(tmpZip)
		os.Remove(tmpSHA)
		return BundleArtifacts{}, fmt.Errorf("build-bundle-zip: write sha: %w", err)
	}

	if err := os.Rename(tmpZip, zipPath); err != nil {
		os.Remove(tmpZip)
		os.Remove(tmpSHA)
		return BundleArtifacts{}, fmt.Errorf("build-bundle-zip: rename zip: %w", err)
	}
	if err := os.Rename(tmpSHA, shaPath); err != nil {
		os.Remove(tmpSHA)
		return BundleArtifacts{}, fmt.Errorf("build-bundle-zip: rename sha: %w", err)
	}

	return BundleArtifacts{
		Arch:      tag.Arch,
		OS:        tag.OS,
		Processor: tag.Processor,
		Version:   tag.Version,
		ZipPath:   zipPath,
		SHAPath:   shaPath,
		Size:      size,
		SHA256:    sum,
	}, nil
}

// writeBundleZip builds a STORE-only zip archive of every regular file in
// bundleDir except the cached zip/sha artifacts themselves. It returns the
// hex-encoded sha256 digest of the resulting file along with its size in
// bytes.
func writeBundleZip(bundleDir string, dst string) (string, int64, error) {
	entries, err := collectBundleEntries(bundleDir)
	if err != nil {
		return "", 0, fmt.Errorf("collect entries: %w", err)
	}

	f, err := os.Create(dst)
	if err != nil {
		return "", 0, fmt.Errorf("create zip: %w", err)
	}
	defer f.Close()

	hasher := sha256.New()
	mw := io.MultiWriter(f, hasher)

	zw := zip.NewWriter(mw)

	for _, rel := range entries {
		full := filepath.Join(bundleDir, rel)
		info, err := os.Stat(full)
		if err != nil {
			return "", 0, fmt.Errorf("stat %s: %w", rel, err)
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return "", 0, fmt.Errorf("header %s: %w", rel, err)
		}
		header.Name = filepath.ToSlash(rel)
		// These are already-compressed binaries; skip deflate.
		header.Method = zip.Store

		w, err := zw.CreateHeader(header)
		if err != nil {
			return "", 0, fmt.Errorf("create header %s: %w", rel, err)
		}

		src, err := os.Open(full)
		if err != nil {
			return "", 0, fmt.Errorf("open %s: %w", rel, err)
		}

		if _, err := io.Copy(w, src); err != nil {
			src.Close()
			return "", 0, fmt.Errorf("copy %s: %w", rel, err)
		}

		src.Close()
	}

	if err := zw.Close(); err != nil {
		return "", 0, fmt.Errorf("close zip writer: %w", err)
	}

	if err := f.Sync(); err != nil {
		return "", 0, fmt.Errorf("sync zip: %w", err)
	}

	info, err := f.Stat()
	if err != nil {
		return "", 0, fmt.Errorf("stat zip: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), info.Size(), nil
}

// collectBundleEntries returns a sorted list of file paths (relative to
// bundleDir) that should be included in the zip archive. The cached zip
// and sha files, plus any temp folder used during installation, are
// excluded.
func collectBundleEntries(bundleDir string) ([]string, error) {
	var out []string

	err := filepath.WalkDir(bundleDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == bundleDir {
			return nil
		}

		rel, err := filepath.Rel(bundleDir, path)
		if err != nil {
			return err
		}

		if d.IsDir() {
			if rel == "temp" {
				return filepath.SkipDir
			}
			return nil
		}

		switch rel {
		case BundleZipFile, BundleZipFile + ".tmp",
			BundleSHAFile, BundleSHAFile + ".tmp":
			return nil
		}

		out = append(out, rel)
		return nil
	})

	if err != nil {
		return nil, err
	}

	sort.Strings(out)
	return out, nil
}

// =============================================================================

// PeerBundleListURL returns the URL on the peer that lists installed
// bundles. Host should be in the form "ip:port".
func PeerBundleListURL(host string) string {
	return fmt.Sprintf("http://%s/download/libs", host)
}

// PeerBundleZipURL returns the URL on the peer that serves the bundle zip
// for the supplied triple. Host should be in the form "ip:port".
func PeerBundleZipURL(host string, arch string, opSys string, processor string) string {
	return fmt.Sprintf("http://%s/download/libs/%s/%s/%s/bundle.zip",
		host,
		url.PathEscape(opSys),
		url.PathEscape(arch),
		url.PathEscape(processor),
	)
}

// PullBundleProgress reports incremental progress of a peer pull. Phase is
// a short status string; for "downloading" events the byte counts are set.
type PullBundleProgress struct {
	Phase       string
	Total       int64
	Current     int64
	MBPerSecond float64
	SHA256      string
	Size        int64
}

// PullBundleFromPeer downloads the bundle zip for the supplied triple from
// a peer Kronk server (running with download enabled), verifies its
// sha256 digest, unzips it into the appropriate bundle directory, and
// writes a fresh version.json. The progress callback receives one event
// when bundle metadata becomes available, periodic events as bytes flow,
// and final events for verification, unzipping, and completion.
func (lib *Libs) PullBundleFromPeer(ctx context.Context, host string, arch string, opSys string, processor string, progress func(PullBundleProgress)) (VersionTag, error) {
	if lib.readOnly {
		return VersionTag{}, fmt.Errorf("libs: pull-bundle-from-peer: %w", ErrReadOnly)
	}

	if !IsSupported(arch, opSys, processor) {
		return VersionTag{}, fmt.Errorf("libs: pull-bundle-from-peer: unsupported combination arch=%s os=%s processor=%s", arch, opSys, processor)
	}

	host = strings.TrimSpace(host)
	if host == "" {
		return VersionTag{}, errors.New("libs: pull-bundle-from-peer: host is required")
	}

	a, o, p, err := parseTriple(arch, opSys, processor)
	if err != nil {
		return VersionTag{}, fmt.Errorf("libs: pull-bundle-from-peer: %w", err)
	}

	dest := installPathFor(lib.root, a, o, p)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return VersionTag{}, fmt.Errorf("libs: pull-bundle-from-peer: mkdir dest: %w", err)
	}

	tempPath := filepath.Join(dest, "temp")
	os.RemoveAll(tempPath)
	if err := os.MkdirAll(tempPath, 0o755); err != nil {
		return VersionTag{}, fmt.Errorf("libs: pull-bundle-from-peer: mkdir temp: %w", err)
	}

	tmpZip := filepath.Join(tempPath, BundleZipFile)

	zipURL := PeerBundleZipURL(host, arch, opSys, processor)

	if progress != nil {
		progress(PullBundleProgress{Phase: "connecting"})
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, zipURL, nil)
	if err != nil {
		os.RemoveAll(tempPath)
		return VersionTag{}, fmt.Errorf("libs: pull-bundle-from-peer: build request: %w", err)
	}

	httpClient := &http.Client{Timeout: 0}
	resp, err := httpClient.Do(req)
	if err != nil {
		os.RemoveAll(tempPath)
		return VersionTag{}, fmt.Errorf("libs: pull-bundle-from-peer: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		os.RemoveAll(tempPath)
		return VersionTag{}, fmt.Errorf("libs: pull-bundle-from-peer: peer returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	expectedSHA := strings.TrimSpace(resp.Header.Get("X-Bundle-SHA256"))

	if progress != nil {
		progress(PullBundleProgress{
			Phase:  "metadata",
			Size:   resp.ContentLength,
			SHA256: expectedSHA,
		})
	}

	out, err := os.Create(tmpZip)
	if err != nil {
		os.RemoveAll(tempPath)
		return VersionTag{}, fmt.Errorf("libs: pull-bundle-from-peer: create temp zip: %w", err)
	}

	hasher := sha256.New()
	pr := downloader.NewProgressReader(func(_ string, current int64, total int64, mbPerSec float64, complete bool) {
		if progress == nil {
			return
		}
		phase := "downloading"
		if complete {
			phase = "downloaded"
		}
		progress(PullBundleProgress{
			Phase:       phase,
			Current:     current,
			Total:       total,
			MBPerSecond: mbPerSec,
		})
	}, downloader.SizeIntervalMB10)
	pr.TrackProgress(zipURL, 0, resp.ContentLength, resp.Body)

	if _, err := io.Copy(io.MultiWriter(out, hasher), pr); err != nil {
		out.Close()
		pr.Close()
		os.RemoveAll(tempPath)
		return VersionTag{}, fmt.Errorf("libs: pull-bundle-from-peer: stream: %w", err)
	}
	pr.Close()

	if err := out.Close(); err != nil {
		os.RemoveAll(tempPath)
		return VersionTag{}, fmt.Errorf("libs: pull-bundle-from-peer: close temp zip: %w", err)
	}

	gotSHA := hex.EncodeToString(hasher.Sum(nil))
	if progress != nil {
		progress(PullBundleProgress{Phase: "verifying", SHA256: gotSHA})
	}

	if expectedSHA != "" && !strings.EqualFold(expectedSHA, gotSHA) {
		os.RemoveAll(tempPath)
		return VersionTag{}, fmt.Errorf("libs: pull-bundle-from-peer: sha256 mismatch: expected %s got %s", expectedSHA, gotSHA)
	}

	if progress != nil {
		progress(PullBundleProgress{Phase: "unzipping"})
	}

	if err := unzipInto(tmpZip, tempPath); err != nil {
		os.RemoveAll(tempPath)
		return VersionTag{}, fmt.Errorf("libs: pull-bundle-from-peer: unzip: %w", err)
	}

	// Drop the zip itself so it's not part of the swap. We may regenerate
	// a fresh local zip on demand if this server is itself queried by a
	// downstream peer.
	os.Remove(tmpZip)

	if progress != nil {
		progress(PullBundleProgress{Phase: "swapping"})
	}

	if err := swapTempForLibAt(dest, tempPath); err != nil {
		os.RemoveAll(tempPath)
		return VersionTag{}, fmt.Errorf("libs: pull-bundle-from-peer: swap: %w", err)
	}

	// Read the version.json that came across in the bundle. If absent,
	// synthesize one from the requested triple so that List/InstalledFor
	// continue to work.
	tag, err := readVersionFile(dest)
	if err != nil {
		if err := writeVersionFile(dest, "", a, o, p); err != nil {
			return VersionTag{}, fmt.Errorf("libs: pull-bundle-from-peer: write version.json: %w", err)
		}
		tag, _ = readVersionFile(dest)
	}

	if progress != nil {
		progress(PullBundleProgress{Phase: "complete"})
	}

	return tag, nil
}

// unzipInto extracts the contents of zipPath into destDir. Existing files
// are overwritten. Path-traversal attempts (entries that resolve outside
// destDir) are rejected.
func unzipInto(zipPath string, destDir string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()

	absDest, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("abs dest: %w", err)
	}

	for _, f := range zr.File {
		// Reject entries that try to escape the destination.
		clean := filepath.Clean(f.Name)
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			return fmt.Errorf("invalid zip entry %q", f.Name)
		}

		target := filepath.Join(destDir, clean)
		absTarget, err := filepath.Abs(target)
		if err != nil {
			return fmt.Errorf("abs target: %w", err)
		}

		if !strings.HasPrefix(absTarget, absDest+string(filepath.Separator)) && absTarget != absDest {
			return fmt.Errorf("invalid zip entry %q", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("mkdir %s: %w", target, err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("mkdir parent %s: %w", target, err)
		}

		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, f.Mode())
		if err != nil {
			return fmt.Errorf("create %s: %w", target, err)
		}

		rc, err := f.Open()
		if err != nil {
			out.Close()
			return fmt.Errorf("open entry %s: %w", f.Name, err)
		}

		if _, err := io.Copy(out, rc); err != nil {
			rc.Close()
			out.Close()
			return fmt.Errorf("copy %s: %w", target, err)
		}
		rc.Close()
		out.Close()
	}

	return nil
}

// =============================================================================

// peerHTTPClient is the http.Client used for short JSON requests against
// peer Kronk servers. The 30s timeout keeps the BUI snappy if a peer is
// unreachable.
var peerHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).DialContext,
	},
}

// PeerBundle is the metadata of a single bundle advertised by a peer.
type PeerBundle struct {
	Arch      string `json:"arch"`
	OS        string `json:"os"`
	Processor string `json:"processor"`
	Version   string `json:"version"`
	Size      int64  `json:"size,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
}

// peerBundleListResponse mirrors the JSON shape returned by /download/libs.
type peerBundleListResponse struct {
	Bundles []PeerBundle `json:"bundles"`
}

// FetchPeerBundles fetches the list of bundles advertised by the peer at
// host (in the form "ip:port").
func FetchPeerBundles(ctx context.Context, host string) ([]PeerBundle, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, errors.New("libs: fetch-peer-bundles: host is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, PeerBundleListURL(host), nil)
	if err != nil {
		return nil, fmt.Errorf("libs: fetch-peer-bundles: build request: %w", err)
	}

	resp, err := peerHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("libs: fetch-peer-bundles: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("libs: fetch-peer-bundles: peer returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var list peerBundleListResponse
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("libs: fetch-peer-bundles: decode: %w", err)
	}

	return list.Bundles, nil
}
