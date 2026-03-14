package models

import (
	"context"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func testGGUFHeaderBytes(size int) []byte {
	if size < 4 {
		size = 4
	}

	data := make([]byte, size)
	binary.LittleEndian.PutUint32(data[:4], ggufMagic)
	return data
}

func resetVRAMHeaderCacheSingletonForTest(t *testing.T, baseDir string) {
	t.Helper()

	oldBaseDir := vramHeaderCacheBaseDir
	oldInst := vramHeaderCacheInst

	vramHeaderCacheBaseDir = baseDir
	vramHeaderCacheInst = nil
	vramHeaderCacheOnce = sync.Once{}

	t.Cleanup(func() {
		vramHeaderCacheBaseDir = oldBaseDir
		vramHeaderCacheInst = oldInst
		vramHeaderCacheOnce = sync.Once{}
	})
}

func TestCanonicalizeVRAMCacheURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "trim lowercase host strip fragment",
			in:   "  https://HF.CO/owner/repo/resolve/main/model.gguf#section  ",
			want: "https://huggingface.co/owner/repo/resolve/main/model.gguf",
		},
		{
			name: "short huggingface path becomes download url",
			in:   "owner/repo/model.gguf",
			want: "https://huggingface.co/owner/repo/resolve/main/model.gguf",
		},
		{
			name: "lowercase scheme and host only",
			in:   "HTTP://HUGGINGFACE.CO/owner/repo/resolve/main/model.gguf#x",
			want: "http://huggingface.co/owner/repo/resolve/main/model.gguf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := canonicalizeVRAMCacheURL(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVRAMCacheKeyEquivalentURLs(t *testing.T) {
	key1, _, err := vramCacheKey("  https://HF.CO/owner/repo/resolve/main/model.gguf#section  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	key2, _, err := vramCacheKey("https://huggingface.co/owner/repo/resolve/main/model.gguf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if key1 != key2 {
		t.Fatalf("equivalent URLs produced different keys: %s vs %s", key1, key2)
	}
}

func TestVRAMHeaderCachePutGet(t *testing.T) {
	baseDir := t.TempDir()

	cache, err := newVRAMHeaderCache(baseDir)
	if err != nil {
		t.Fatalf("newVRAMHeaderCache error: %v", err)
	}

	url := "https://huggingface.co/owner/repo/resolve/main/model.gguf"
	wantData := testGGUFHeaderBytes(128)
	wantFileSize := int64(123456)

	if err := cache.put(url, wantData, wantFileSize); err != nil {
		t.Fatalf("put error: %v", err)
	}

	gotData, gotFileSize, ok, err := cache.get(url)
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if !ok {
		t.Fatal("expected cache hit")
	}
	if gotFileSize != wantFileSize {
		t.Fatalf("got file size %d, want %d", gotFileSize, wantFileSize)
	}
	if string(gotData) != string(wantData) {
		t.Fatal("cached bytes did not round-trip")
	}
}

func TestVRAMHeaderCacheGetUpdatesLastAccessedAt(t *testing.T) {
	baseDir := t.TempDir()

	cache, err := newVRAMHeaderCache(baseDir)
	if err != nil {
		t.Fatalf("newVRAMHeaderCache error: %v", err)
	}

	url := "https://huggingface.co/owner/repo/resolve/main/model.gguf"
	data := testGGUFHeaderBytes(128)

	if err := cache.put(url, data, 999); err != nil {
		t.Fatalf("put error: %v", err)
	}

	key, _, err := vramCacheKey(url)
	if err != nil {
		t.Fatalf("vramCacheKey error: %v", err)
	}

	oldTime := time.Now().Add(-1 * time.Hour).UTC()
	cache.mu.Lock()
	entry := cache.index.Entries[key]
	entry.LastAccessedAt = oldTime
	cache.index.Entries[key] = entry
	cache.mu.Unlock()

	_, _, ok, err := cache.get(url)
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if !ok {
		t.Fatal("expected cache hit")
	}

	cache.mu.Lock()
	updatedEntry := cache.index.Entries[key]
	cache.mu.Unlock()

	if !updatedEntry.LastAccessedAt.After(oldTime) {
		t.Fatalf("LastAccessedAt was not updated: got %v, old was %v", updatedEntry.LastAccessedAt, oldTime)
	}
}

func TestVRAMHeaderCacheExpiredEntryRemoved(t *testing.T) {
	baseDir := t.TempDir()

	cache, err := newVRAMHeaderCache(baseDir)
	if err != nil {
		t.Fatalf("newVRAMHeaderCache error: %v", err)
	}

	url := "https://huggingface.co/owner/repo/resolve/main/model.gguf"
	data := testGGUFHeaderBytes(128)

	if err := cache.put(url, data, 999); err != nil {
		t.Fatalf("put error: %v", err)
	}

	key, _, err := vramCacheKey(url)
	if err != nil {
		t.Fatalf("vramCacheKey error: %v", err)
	}

	// Set FetchedAt to beyond TTL.
	cache.mu.Lock()
	entry := cache.index.Entries[key]
	entry.FetchedAt = time.Now().Add(-(vramHeaderCacheTTL + time.Hour)).UTC()
	cache.index.Entries[key] = entry
	cache.mu.Unlock()

	_, _, ok, err := cache.get(url)
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if ok {
		t.Fatal("expected cache miss for expired entry")
	}

	cache.mu.Lock()
	_, exists := cache.index.Entries[key]
	cache.mu.Unlock()

	if exists {
		t.Fatal("expired entry should have been removed from index")
	}

	binPath := cache.entryPath(key)
	if _, err := os.Stat(binPath); !os.IsNotExist(err) {
		t.Fatal("expired .bin file should have been removed from disk")
	}
}

func TestVRAMHeaderCacheMissingBinRemovesEntry(t *testing.T) {
	baseDir := t.TempDir()

	cache, err := newVRAMHeaderCache(baseDir)
	if err != nil {
		t.Fatalf("newVRAMHeaderCache error: %v", err)
	}

	url := "https://huggingface.co/owner/repo/resolve/main/model.gguf"
	key, canonicalURL, err := vramCacheKey(url)
	if err != nil {
		t.Fatalf("vramCacheKey error: %v", err)
	}

	now := time.Now().UTC()
	cache.mu.Lock()
	cache.index.Entries[key] = vramHeaderCacheEntry{
		URL:            canonicalURL,
		FileSize:       999,
		HeaderSize:     128,
		FetchedAt:      now,
		LastAccessedAt: now,
	}
	cache.mu.Unlock()

	_, _, ok, err := cache.get(url)
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if ok {
		t.Fatal("expected cache miss for missing .bin")
	}

	cache.mu.Lock()
	_, exists := cache.index.Entries[key]
	cache.mu.Unlock()

	if exists {
		t.Fatal("stale index entry should have been removed")
	}
}

func TestVRAMHeaderCacheCorruptBinRemovesEntry(t *testing.T) {
	baseDir := t.TempDir()

	cache, err := newVRAMHeaderCache(baseDir)
	if err != nil {
		t.Fatalf("newVRAMHeaderCache error: %v", err)
	}

	url := "https://huggingface.co/owner/repo/resolve/main/model.gguf"
	key, canonicalURL, err := vramCacheKey(url)
	if err != nil {
		t.Fatalf("vramCacheKey error: %v", err)
	}

	// Write corrupt data (no GGUF magic).
	corruptData := make([]byte, 128)
	binPath := cache.entryPath(key)
	if err := os.WriteFile(binPath, corruptData, 0644); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	now := time.Now().UTC()
	cache.mu.Lock()
	cache.index.Entries[key] = vramHeaderCacheEntry{
		URL:            canonicalURL,
		FileSize:       999,
		HeaderSize:     128,
		FetchedAt:      now,
		LastAccessedAt: now,
	}
	cache.mu.Unlock()

	_, _, ok, err := cache.get(url)
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if ok {
		t.Fatal("expected cache miss for corrupt .bin")
	}

	cache.mu.Lock()
	_, exists := cache.index.Entries[key]
	cache.mu.Unlock()

	if exists {
		t.Fatal("corrupt entry should have been removed from index")
	}

	if _, err := os.Stat(binPath); !os.IsNotExist(err) {
		t.Fatal("corrupt .bin file should have been deleted from disk")
	}
}

func TestVRAMHeaderCacheLRUEviction(t *testing.T) {
	baseDir := t.TempDir()

	cache, err := newVRAMHeaderCache(baseDir)
	if err != nil {
		t.Fatalf("newVRAMHeaderCache error: %v", err)
	}

	cache.maxBytes = 2 * 1024

	urlA := "https://huggingface.co/owner/repo/resolve/main/a.gguf"
	urlB := "https://huggingface.co/owner/repo/resolve/main/b.gguf"
	urlC := "https://huggingface.co/owner/repo/resolve/main/c.gguf"

	keyA, canonA, _ := vramCacheKey(urlA)
	keyB, canonB, _ := vramCacheKey(urlB)
	keyC, canonC, _ := vramCacheKey(urlC)

	now := time.Now().UTC()

	dataA := testGGUFHeaderBytes(1024)
	dataB := testGGUFHeaderBytes(1024)
	dataC := testGGUFHeaderBytes(1024)

	if err := os.WriteFile(cache.entryPath(keyA), dataA, 0644); err != nil {
		t.Fatalf("write A: %v", err)
	}
	if err := os.WriteFile(cache.entryPath(keyB), dataB, 0644); err != nil {
		t.Fatalf("write B: %v", err)
	}
	if err := os.WriteFile(cache.entryPath(keyC), dataC, 0644); err != nil {
		t.Fatalf("write C: %v", err)
	}

	cache.mu.Lock()

	cache.index.Entries[keyA] = vramHeaderCacheEntry{
		URL:            canonA,
		FileSize:       9999,
		HeaderSize:     1024,
		FetchedAt:      now.Add(-3 * time.Hour),
		LastAccessedAt: now.Add(-3 * time.Hour),
	}
	cache.index.Entries[keyB] = vramHeaderCacheEntry{
		URL:            canonB,
		FileSize:       9999,
		HeaderSize:     1024,
		FetchedAt:      now.Add(-2 * time.Hour),
		LastAccessedAt: now.Add(-2 * time.Hour),
	}
	cache.index.Entries[keyC] = vramHeaderCacheEntry{
		URL:            canonC,
		FileSize:       9999,
		HeaderSize:     1024,
		FetchedAt:      now.Add(-1 * time.Hour),
		LastAccessedAt: now.Add(-1 * time.Hour),
	}

	changed := cache.pruneLocked(now)
	cache.mu.Unlock()

	if !changed {
		t.Fatal("expected prune to evict an entry")
	}

	cache.mu.Lock()
	_, aExists := cache.index.Entries[keyA]
	_, bExists := cache.index.Entries[keyB]
	_, cExists := cache.index.Entries[keyC]
	remainingBytes := cache.totalBytesLocked()
	cache.mu.Unlock()

	if aExists {
		t.Fatal("entry A (oldest) should have been evicted")
	}
	if !bExists {
		t.Fatal("entry B should remain")
	}
	if !cExists {
		t.Fatal("entry C should remain")
	}
	if remainingBytes > cache.maxBytes {
		t.Fatalf("remaining bytes %d exceeds max %d", remainingBytes, cache.maxBytes)
	}
}

func TestFetchGGUFHeaderBytesUsesCache(t *testing.T) {
	baseDir := t.TempDir()
	resetVRAMHeaderCacheSingletonForTest(t, baseDir)

	var mu sync.Mutex
	hits := 0
	body := testGGUFHeaderBytes(ggufHeaderFetchSize)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()

		cr := "bytes 0-" + fmt.Sprintf("%d/%d", ggufHeaderFetchSize-1, ggufHeaderFetchSize*2)
		w.Header().Set("Content-Range", cr)
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	_, _, err := fetchGGUFHeaderBytes(context.Background(), srv.URL+"/model.gguf")
	if err != nil {
		t.Fatalf("first fetch error: %v", err)
	}

	_, _, err = fetchGGUFHeaderBytes(context.Background(), srv.URL+"/model.gguf")
	if err != nil {
		t.Fatalf("second fetch error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Fatalf("expected 1 network request, got %d", hits)
	}
}

func TestFetchGGUFHeaderBytesInvalidMagicNotCached(t *testing.T) {
	baseDir := t.TempDir()
	resetVRAMHeaderCacheSingletonForTest(t, baseDir)

	var mu sync.Mutex
	hits := 0
	body := make([]byte, ggufHeaderFetchSize) // No GGUF magic.

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()

		cr := fmt.Sprintf("bytes 0-%d/%d", ggufHeaderFetchSize-1, ggufHeaderFetchSize*2)
		w.Header().Set("Content-Range", cr)
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	_, _, _ = fetchGGUFHeaderBytes(context.Background(), srv.URL+"/model.gguf")
	_, _, _ = fetchGGUFHeaderBytes(context.Background(), srv.URL+"/model.gguf")

	mu.Lock()
	defer mu.Unlock()
	if hits != 2 {
		t.Fatalf("expected 2 network requests (invalid data not cached), got %d", hits)
	}
}

func TestVRAMHeaderCacheCorruptIndexRecovery(t *testing.T) {
	baseDir := t.TempDir()

	cacheDir := filepath.Join(baseDir, "cache", "vram")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	indexPath := filepath.Join(cacheDir, "index.yaml")
	if err := os.WriteFile(indexPath, []byte("{{{{not yaml"), 0644); err != nil {
		t.Fatalf("write corrupt index: %v", err)
	}

	cache, err := newVRAMHeaderCache(baseDir)
	if err != nil {
		t.Fatalf("newVRAMHeaderCache should succeed with corrupt index: %v", err)
	}

	if len(cache.index.Entries) != 0 {
		t.Fatalf("expected empty entries after corrupt index recovery, got %d", len(cache.index.Entries))
	}
}
