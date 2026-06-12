package models

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// ggmlFileMagic is the 4-byte little-endian magic ("lmgg") that begins
// every whisper.cpp ggml model file. See whisper.cpp's
// whisper_model_load and the GGML_FILE_MAGIC constant in ggml-common.h.
const ggmlFileMagic uint32 = 0x67676d6c

// ggmlQuantVersionFactor mirrors GGML_QNT_VERSION_FACTOR in ggml.h. It
// is used to split the on-disk ftype field into the quantization
// version and the real ftype: qntvr = ftype / factor, ftype %= factor.
const ggmlQuantVersionFactor int32 = 1000

// headerSize is the byte count of the fixed-size hparams prefix every
// whisper ggml file begins with: 4-byte magic plus 11 int32 fields.
const headerSize = 4 + 11*4

// headerCacheDir is the name of the per-id header cache directory
// kept under the models path. Each entry holds the 48-byte hparams
// prefix so future details lookups never need disk or network access.
const headerCacheDir = ".header_cache"

// Header carries the static hyperparameters parsed from the leading
// bytes of a whisper.cpp ggml model file. The fields are read directly
// from disk in the order whisper.cpp's whisper_model_load expects.
// Values are sufficient to identify the model variant, vocabulary
// language coverage, and quantization without loading the model into
// the whisper runtime.
type Header struct {
	NVocab      int32 // Vocabulary size (51864 = english-only, 51865 = multilingual, 51866 = large-v3).
	NAudioCtx   int32 // Encoder context length (typically 1500).
	NAudioState int32 // Encoder hidden state size.
	NAudioHead  int32 // Encoder attention heads.
	NAudioLayer int32 // Encoder layers (4=tiny, 6=base, 12=small, 24=medium, 32=large).
	NTextCtx    int32 // Decoder context length (typically 448).
	NTextState  int32 // Decoder hidden state size.
	NTextHead   int32 // Decoder attention heads.
	NTextLayer  int32 // Decoder layers.
	NMels       int32 // Mel filter bank count (80 or 128).
	FType       int32 // ggml ftype enum after stripping the quantization version.
	QntVersion  int32 // Quantization version (ftype / GGML_QNT_VERSION_FACTOR).
}

// =============================================================================

// ReadHeader parses the leading hparams block of a whisper.cpp ggml
// model file at path. It reads only the first headerSize bytes,
// verifies the magic number, and returns the parsed Header. The file
// is closed before returning. The function does not load the model
// weights and does not depend on the whisper.cpp runtime.
func ReadHeader(path string) (Header, error) {
	f, err := os.Open(path)
	if err != nil {
		return Header{}, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	buf := make([]byte, headerSize)
	if _, err := io.ReadFull(f, buf); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			return Header{}, fmt.Errorf("read: truncated header")
		}
		return Header{}, fmt.Errorf("read: %w", err)
	}

	return parseHeader(buf)
}

// FetchHeader fetches the leading hparams block of a whisper.cpp ggml
// model file at rawURL via an HTTP Range request and returns the
// parsed Header. Accepts a 206 Partial Content response (the common
// case) or a 200 OK response that returns the full file (some HF
// storage backends do not honor Range); in the 200 case only the
// first headerSize bytes are read before the body is closed.
func FetchHeader(ctx context.Context, rawURL string) (Header, error) {
	buf, err := fetchHeaderBytes(ctx, rawURL)
	if err != nil {
		return Header{}, err
	}

	return parseHeader(buf)
}

// ModelType returns the human-readable variant name derived from the
// encoder layer count and (for large) the vocabulary size. Matches the
// model name reported by whisper.cpp's WHISPER_LOG_INFO output.
func (h Header) ModelType() string {
	switch h.NAudioLayer {
	case 4:
		return "tiny"
	case 6:
		return "base"
	case 12:
		return "small"
	case 24:
		return "medium"
	case 32:
		if h.NVocab == 51866 {
			return "large-v3"
		}
		return "large"
	}
	return "unknown"
}

// IsMultilingual reports whether the model carries multilingual
// vocabulary entries. Derived from NVocab: 51864 is the english-only
// vocabulary; 51865 and 51866 are multilingual.
func (h Header) IsMultilingual() bool {
	return h.NVocab != 51864
}

// QuantizationName returns the ggml ftype enum value as a readable
// label (for example "F16", "Q5_1", "Q8_0"). Unknown ftypes are
// rendered as "ftype-<n>" so the caller can still surface them.
func (h Header) QuantizationName() string {
	switch h.FType {
	case 0:
		return "F32"
	case 1:
		return "F16"
	case 2:
		return "Q4_0"
	case 3:
		return "Q4_1"
	case 7:
		return "Q8_0"
	case 8:
		return "Q5_0"
	case 9:
		return "Q5_1"
	case 10:
		return "Q2_K"
	case 11:
		return "Q3_K"
	case 12:
		return "Q4_K"
	case 13:
		return "Q5_K"
	case 14:
		return "Q6_K"
	}
	return fmt.Sprintf("ftype-%d", h.FType)
}

// =============================================================================

// Header reads the on-disk ggml header for the installed model
// identified by modelID. modelID accepts the same forms as
// FullPath ("tiny", "ggml-tiny", "ggml-tiny.bin"). The model must
// already be present locally; this method does not download.
func (m *Models) Header(modelID string) (Header, error) {
	p, err := m.FullPath(modelID)
	if err != nil {
		return Header{}, fmt.Errorf("resolve path: %w", err)
	}
	if len(p.ModelFiles) == 0 {
		return Header{}, fmt.Errorf("model %q has no on-disk file", modelID)
	}

	return ReadHeader(p.ModelFiles[0])
}

// CatalogHeader returns the parsed ggml Header for any model in the
// bundled catalog identified by its short name ("tiny", "ggml-tiny.bin",
// "large-v3"). Lookup order mirrors the kronk GGUF flow:
//
//  1. Per-id header cache under <modelsPath>/.header_cache/<id>.hdr.
//  2. The local on-disk model file when already downloaded — the bytes
//     are written through to the cache so the next call is a hit.
//  3. An HTTP Range request to the catalog URL — the bytes are written
//     through to the cache so the next call avoids the network.
//
// The full model is never downloaded by this method; the network path
// reads only headerSize bytes.
func (m *Models) CatalogHeader(ctx context.Context, modelID string) (Header, error) {
	short := normalizeShortName(modelID)

	entry, ok := catalog[short]
	if !ok {
		return Header{}, fmt.Errorf("catalog-header: unknown model %q", modelID)
	}

	cacheFile := m.headerCacheFile(short)

	if data, err := os.ReadFile(cacheFile); err == nil && isValidHeaderBytes(data) {
		return parseHeader(data)
	}

	if p, err := m.FullPath(short); err == nil && len(p.ModelFiles) > 0 {
		if data, rerr := readHeaderBytes(p.ModelFiles[0]); rerr == nil && isValidHeaderBytes(data) {
			_ = writeHeaderCache(cacheFile, data)
			return parseHeader(data)
		}
	}

	data, err := fetchHeaderBytes(ctx, entry.URL)
	if err != nil {
		return Header{}, fmt.Errorf("catalog-header: fetch %s: %w", entry.URL, err)
	}

	_ = writeHeaderCache(cacheFile, data)

	return parseHeader(data)
}

// =============================================================================

func (m *Models) headerCacheFile(modelID string) string {
	return filepath.Join(m.modelsPath, headerCacheDir, modelID+".hdr")
}

// cacheHeaderFromFile copies the first headerSize bytes of an on-disk
// whisper model into the per-id header cache. Called opportunistically
// after a successful Download so subsequent CatalogHeader calls avoid
// the file open entirely. Errors are returned but callers typically
// log and continue.
func (m *Models) cacheHeaderFromFile(modelID, localFile string) error {
	short := normalizeShortName(modelID)
	if short == "" {
		return fmt.Errorf("cache-header: empty id")
	}

	data, err := readHeaderBytes(localFile)
	if err != nil {
		return fmt.Errorf("cache-header: read %s: %w", localFile, err)
	}
	if !isValidHeaderBytes(data) {
		return fmt.Errorf("cache-header: bad magic in %s", localFile)
	}

	return writeHeaderCache(m.headerCacheFile(short), data)
}

// removeHeaderCache deletes the per-id header cache for modelID. Called
// when a model is removed from disk so the cache does not drift.
func (m *Models) removeHeaderCache(modelID string) error {
	short := normalizeShortName(modelID)
	if short == "" {
		return nil
	}

	if err := os.Remove(m.headerCacheFile(short)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove-header-cache: %w", err)
	}

	return nil
}

// =============================================================================

// readHeaderBytes reads the first headerSize bytes of path. The file
// is closed before returning. No magic validation is performed; pair
// with isValidHeaderBytes.
func readHeaderBytes(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	buf := make([]byte, headerSize)
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, err
	}

	return buf, nil
}

// writeHeaderCache writes data to path atomically via temp + rename so
// a partial write never replaces a valid cache file. Parent directories
// are created on demand. data must already pass isValidHeaderBytes.
func writeHeaderCache(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmp := f.Name()
	defer os.Remove(tmp)

	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}

	return os.Rename(tmp, path)
}

// isValidHeaderBytes reports whether data starts with the whisper ggml
// magic number and is long enough to hold the hparams prefix.
func isValidHeaderBytes(data []byte) bool {
	if len(data) < headerSize {
		return false
	}
	return binary.LittleEndian.Uint32(data[:4]) == ggmlFileMagic
}

// parseHeader decodes a headerSize byte buffer into a Header. The
// caller is responsible for validating the magic with
// isValidHeaderBytes when reading from an untrusted source.
func parseHeader(buf []byte) (Header, error) {
	if len(buf) < headerSize {
		return Header{}, fmt.Errorf("parse: short buffer: got %d, want %d", len(buf), headerSize)
	}

	r := bytes.NewReader(buf)

	var magic uint32
	if err := binary.Read(r, binary.LittleEndian, &magic); err != nil {
		return Header{}, fmt.Errorf("parse magic: %w", err)
	}
	if magic != ggmlFileMagic {
		return Header{}, fmt.Errorf("bad magic: got %#x, want %#x", magic, ggmlFileMagic)
	}

	var h Header
	fields := []*int32{
		&h.NVocab,
		&h.NAudioCtx,
		&h.NAudioState,
		&h.NAudioHead,
		&h.NAudioLayer,
		&h.NTextCtx,
		&h.NTextState,
		&h.NTextHead,
		&h.NTextLayer,
		&h.NMels,
		&h.FType,
	}
	for _, dst := range fields {
		if err := binary.Read(r, binary.LittleEndian, dst); err != nil {
			return Header{}, fmt.Errorf("parse hparams: %w", err)
		}
	}

	h.QntVersion = h.FType / ggmlQuantVersionFactor
	h.FType %= ggmlQuantVersionFactor

	return h, nil
}

// fetchHeaderBytes performs an HTTP Range request for the first
// headerSize bytes of rawURL. Accepts a 206 Partial Content response
// (the common case) or a 200 OK response that returns the full file
// (some HF storage backends do not honor Range); in the 200 case only
// the first headerSize bytes are read before the body is closed.
func fetchHeaderBytes(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", headerSize-1))

	var client http.Client
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	buf := make([]byte, headerSize)
	if _, err := io.ReadFull(resp.Body, buf); err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return buf, nil
}
