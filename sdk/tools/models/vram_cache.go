package models

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	yaml "go.yaml.in/yaml/v2"
)

const (
	vramHeaderCacheTTL       = 14 * 24 * time.Hour
	vramHeaderCacheMaxBytes  = 4 * 1024 * 1024 * 1024
	vramHeaderCacheIndexFile = "index.yaml"
)

type vramHeaderCache struct {
	mu        sync.Mutex
	dir       string
	indexPath string
	ttl       time.Duration
	maxBytes  int64
	index     vramHeaderCacheIndex
}

type vramHeaderCacheIndex struct {
	Entries map[string]vramHeaderCacheEntry `yaml:"entries"`
}

type vramHeaderCacheEntry struct {
	URL            string    `yaml:"url"`
	FileSize       int64     `yaml:"file_size"`
	HeaderSize     int64     `yaml:"header_size"`
	FetchedAt      time.Time `yaml:"fetched_at"`
	LastAccessedAt time.Time `yaml:"last_accessed_at"`
}

var (
	vramHeaderCacheBaseDir = defaults.BaseDir("")
	vramHeaderCacheOnce    sync.Once
	vramHeaderCacheInst    *vramHeaderCache
)

// fetchGGUFHeaderBytes fetches GGUF header bytes for the given URL, using the
// VRAM header cache when available.
func fetchGGUFHeaderBytes(ctx context.Context, rawURL string) ([]byte, int64, error) {
	fetchURL := rawURL
	if canonicalURL, err := canonicalizeVRAMCacheURL(rawURL); err == nil {
		fetchURL = canonicalURL
	}

	cache := getVRAMHeaderCache()
	if cache != nil {
		if data, fileSize, ok, err := cache.get(fetchURL); err == nil && ok {
			return data, fileSize, nil
		}
	}

	var client http.Client
	data, fileSize, err := fetchRange(ctx, &client, fetchURL, 0, ggufHeaderFetchSize-1)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch-gguf-header-bytes: failed to fetch header data: %w", err)
	}

	// Best-effort cache write.
	if cache != nil && isValidGGUFHeaderBytes(data) {
		_ = cache.put(fetchURL, data, fileSize)
	}

	return data, fileSize, nil
}

func getVRAMHeaderCache() *vramHeaderCache {
	vramHeaderCacheOnce.Do(func() {
		cache, err := newVRAMHeaderCache(vramHeaderCacheBaseDir)
		if err == nil {
			vramHeaderCacheInst = cache
		}
	})

	return vramHeaderCacheInst
}

func newVRAMHeaderCache(baseDir string) (*vramHeaderCache, error) {
	dir := filepath.Join(baseDir, "cache", "vram")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("new-vram-header-cache: create cache directory: %w", err)
	}

	c := &vramHeaderCache{
		dir:       dir,
		indexPath: filepath.Join(dir, vramHeaderCacheIndexFile),
		ttl:       vramHeaderCacheTTL,
		maxBytes:  vramHeaderCacheMaxBytes,
		index: vramHeaderCacheIndex{
			Entries: make(map[string]vramHeaderCacheEntry),
		},
	}

	if err := c.loadIndex(); err != nil {
		return nil, fmt.Errorf("new-vram-header-cache: load index: %w", err)
	}

	return c, nil
}

func (c *vramHeaderCache) get(rawURL string) ([]byte, int64, bool, error) {
	key, _, err := vramCacheKey(rawURL)
	if err != nil {
		return nil, 0, false, fmt.Errorf("vram-cache-get: build cache key: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().UTC()
	changed := c.pruneLocked(now)

	entry, ok := c.index.Entries[key]
	if !ok {
		if changed {
			_ = c.saveIndexLocked()
		}
		return nil, 0, false, nil
	}

	data, err := os.ReadFile(c.entryPath(key))
	if err != nil || !isValidGGUFHeaderBytes(data) {
		c.deleteEntryLocked(key)
		_ = c.saveIndexLocked()
		return nil, 0, false, nil
	}

	entry.LastAccessedAt = now
	c.index.Entries[key] = entry

	if err := c.saveIndexLocked(); err != nil {
		return nil, 0, false, fmt.Errorf("vram-cache-get: save index after cache hit: %w", err)
	}

	return data, entry.FileSize, true, nil
}

func (c *vramHeaderCache) put(rawURL string, data []byte, fileSize int64) error {
	if !isValidGGUFHeaderBytes(data) {
		return nil
	}

	key, canonicalURL, err := vramCacheKey(rawURL)
	if err != nil {
		return fmt.Errorf("vram-cache-put: build cache key: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().UTC()
	c.pruneLocked(now)

	if err := writeFileAtomic(c.entryPath(key), data, 0644); err != nil {
		return fmt.Errorf("vram-cache-put: write cache file: %w", err)
	}

	c.index.Entries[key] = vramHeaderCacheEntry{
		URL:            canonicalURL,
		FileSize:       fileSize,
		HeaderSize:     int64(len(data)),
		FetchedAt:      now,
		LastAccessedAt: now,
	}

	c.pruneLocked(now)

	if err := c.saveIndexLocked(); err != nil {
		return fmt.Errorf("vram-cache-put: save index: %w", err)
	}

	return nil
}

func (c *vramHeaderCache) loadIndex() error {
	data, err := os.ReadFile(c.indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("load-index: read file: %w", err)
	}

	var index vramHeaderCacheIndex
	if err := yaml.Unmarshal(data, &index); err != nil {
		_ = os.Remove(c.indexPath)
		c.index = vramHeaderCacheIndex{
			Entries: make(map[string]vramHeaderCacheEntry),
		}
		return nil
	}

	if index.Entries == nil {
		index.Entries = make(map[string]vramHeaderCacheEntry)
	}

	c.index = index
	return nil
}

func (c *vramHeaderCache) saveIndexLocked() error {
	if c.index.Entries == nil {
		c.index.Entries = make(map[string]vramHeaderCacheEntry)
	}

	data, err := yaml.Marshal(&c.index)
	if err != nil {
		return fmt.Errorf("save-index: marshal yaml: %w", err)
	}

	if err := writeFileAtomic(c.indexPath, data, 0644); err != nil {
		return fmt.Errorf("save-index: write file: %w", err)
	}

	return nil
}

func (c *vramHeaderCache) pruneLocked(now time.Time) bool {
	changed := false

	for key, entry := range c.index.Entries {
		if entry.FetchedAt.IsZero() || entry.HeaderSize <= 0 {
			c.deleteEntryLocked(key)
			changed = true
			continue
		}

		if entry.LastAccessedAt.IsZero() {
			entry.LastAccessedAt = entry.FetchedAt
			c.index.Entries[key] = entry
			changed = true
		}

		if now.Sub(entry.FetchedAt) > c.ttl {
			c.deleteEntryLocked(key)
			changed = true
			continue
		}

		info, err := os.Stat(c.entryPath(key))
		if err != nil || info.IsDir() || info.Size() != entry.HeaderSize {
			c.deleteEntryLocked(key)
			changed = true
			continue
		}
	}

	total := c.totalBytesLocked()
	if total <= c.maxBytes {
		return changed
	}

	type lruItem struct {
		key       string
		when      time.Time
		fileSize  int64
		fetchedAt time.Time
	}

	items := make([]lruItem, 0, len(c.index.Entries))
	for key, entry := range c.index.Entries {
		when := entry.LastAccessedAt
		if when.IsZero() {
			when = entry.FetchedAt
		}

		items = append(items, lruItem{
			key:       key,
			when:      when,
			fileSize:  entry.HeaderSize,
			fetchedAt: entry.FetchedAt,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].when.Equal(items[j].when) {
			if items[i].fetchedAt.Equal(items[j].fetchedAt) {
				return items[i].key < items[j].key
			}
			return items[i].fetchedAt.Before(items[j].fetchedAt)
		}
		return items[i].when.Before(items[j].when)
	})

	for _, item := range items {
		if total <= c.maxBytes {
			break
		}

		c.deleteEntryLocked(item.key)
		total -= item.fileSize
		changed = true
	}

	return changed
}

func (c *vramHeaderCache) totalBytesLocked() int64 {
	var total int64
	for _, entry := range c.index.Entries {
		total += entry.HeaderSize
	}
	return total
}

func (c *vramHeaderCache) deleteEntryLocked(key string) {
	_ = os.Remove(c.entryPath(key))
	delete(c.index.Entries, key)
}

func (c *vramHeaderCache) entryPath(key string) string {
	return filepath.Join(c.dir, "v1-"+key+".bin")
}

func vramCacheKey(rawURL string) (string, string, error) {
	canonicalURL, err := canonicalizeVRAMCacheURL(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("vram-cache-key: %w", err)
	}

	sum := sha256.Sum256([]byte(canonicalURL))
	return hex.EncodeToString(sum[:]), canonicalURL, nil
}

func canonicalizeVRAMCacheURL(rawURL string) (string, error) {
	s := strings.TrimSpace(rawURL)
	lower := strings.ToLower(s)

	if !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "http://") {
		s = NormalizeHuggingFaceDownloadURL(s)
	}

	u, err := neturl.Parse(s)
	if err != nil {
		return "", fmt.Errorf("canonicalize-vram-cache-url: parse url: %w", err)
	}

	u.Scheme = strings.ToLower(u.Scheme)

	host := strings.ToLower(u.Host)
	if host == "hf.co" {
		host = "huggingface.co"
	}
	u.Host = host

	u.Fragment = ""

	return u.String(), nil
}

func isValidGGUFHeaderBytes(data []byte) bool {
	if len(data) < 4 {
		return false
	}

	return binary.LittleEndian.Uint32(data[:4]) == ggufMagic
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	f, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("write-file-atomic: create temp file: %w", err)
	}
	tmpPath := f.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write-file-atomic: write temp file: %w", err)
	}

	if err := f.Chmod(perm); err != nil {
		_ = f.Close()
		return fmt.Errorf("write-file-atomic: chmod temp file: %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("write-file-atomic: close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("write-file-atomic: rename temp file: %w", err)
	}

	return nil
}
