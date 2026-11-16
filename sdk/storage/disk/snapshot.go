package disk

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/ardanlabs/kronk/sdk/kronk/imc"
)

const snapshotMagic = "KIMCDSK1"
const maxMetadata = 64 << 20

type metadata struct {
	Record imc.Record
	Target int64
	Draft  int64
	Media  []int64
}

func writeSnapshot(ctx context.Context, directory string, snapshot imc.Snapshot) (name string, retErr error) {
	m := metadata{Record: snapshot.Record, Target: int64(len(snapshot.Target)), Draft: int64(len(snapshot.Draft))}
	for _, media := range snapshot.Media {
		m.Media = append(m.Media, int64(len(media)))
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("disk: encode record: %w", err)
	}
	if len(data) > maxMetadata {
		return "", errors.New("disk: record exceeds 64 MiB")
	}
	f, err := os.CreateTemp(directory, "snapshot-*.bin")
	if err != nil {
		return "", err
	}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, removeFile(f.Name()))
		}
	}()
	header := make([]byte, 48)
	copy(header, snapshotMagic)
	binary.LittleEndian.PutUint64(header[8:16], uint64(len(data)))
	digest := sha256.Sum256(data)
	copy(header[16:], digest[:])
	if _, err := f.Write(header); err != nil {
		return "", errors.Join(err, f.Close())
	}
	if _, err := f.Write(data); err != nil {
		return "", errors.Join(err, f.Close())
	}
	z, err := gzip.NewWriterLevel(f, gzip.BestSpeed)
	if err != nil {
		return "", errors.Join(err, f.Close())
	}
	write := func(data []byte) error {
		for chunk := range slices.Chunk(data, 64<<10) {
			if err := ctx.Err(); err != nil {
				return err
			}
			if _, err := z.Write(chunk); err != nil {
				return err
			}
		}
		return nil
	}
	writeErr := write(snapshot.Target)
	if writeErr == nil {
		writeErr = write(snapshot.Draft)
	}
	for _, media := range snapshot.Media {
		if writeErr != nil {
			break
		}
		writeErr = write(media)
	}
	if err := errors.Join(writeErr, z.Close(), f.Sync(), f.Close(), ctx.Err()); err != nil {
		return "", err
	}
	if err := syncDirectory(directory); err != nil {
		return "", err
	}
	return filepath.Base(f.Name()), nil
}

func readJSONFile(name string, limit int64, destination any) (retErr error) {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, f.Close()) }()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > limit {
		return imc.ErrCorrupt
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return errors.Join(imc.ErrCorrupt, err)
	}
	return nil
}

type session struct {
	store  *Store
	file   *os.File
	blob   string
	meta   metadata
	offset int64
	closed bool
}

var _ imc.Session = (*session)(nil)

// Open pins exactly the requested latest revision. Replaced or missing revisions
// return fs.ErrNotExist. Metadata is verified without decompressing payloads.
func (s *Store) Open(ctx context.Context, ref imc.Ref) (imc.Session, error) {
	if ref.Key.Model == "" || ref.Key.ID == "" || ref.Revision == 0 {
		return nil, errors.New("disk: invalid session reference")
	}
	if err := s.acquire(ctx); err != nil {
		return nil, err
	}
	defer s.release()
	e, exists := s.entries[ref.Key]
	if !exists || e.Summary.Ref != ref {
		return nil, fs.ErrNotExist
	}
	f, err := os.Open(filepath.Join(s.directory, e.Blob))
	if err != nil {
		return nil, err
	}
	h, err := openSession(ctx, f, e)
	if err != nil {
		return nil, errors.Join(err, f.Close())
	}
	h.store = s
	s.pins[e.Blob]++
	return h, nil
}

func openSession(ctx context.Context, f *os.File, e entry) (*session, error) {
	var header [48]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		return nil, errors.Join(imc.ErrCorrupt, err)
	}
	if string(header[:8]) != snapshotMagic {
		return nil, imc.ErrVersion
	}
	length := binary.LittleEndian.Uint64(header[8:16])
	if length == 0 || length > maxMetadata {
		return nil, imc.ErrCorrupt
	}
	data := make([]byte, int(length))
	if _, err := io.ReadFull(contextReader{ctx, f}, data); err != nil {
		return nil, errors.Join(imc.ErrCorrupt, err)
	}
	digest := sha256.Sum256(data)
	if !bytes.Equal(digest[:], header[16:]) {
		return nil, imc.ErrCorrupt
	}
	var m metadata
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, errors.Join(imc.ErrCorrupt, err)
	}
	if m.Record.Version != 1 {
		return nil, imc.ErrVersion
	}
	summary := m.Record.Summary
	if !summary.CreatedAt.Equal(e.Summary.CreatedAt) || !summary.SavedAt.Equal(e.Summary.SavedAt) {
		return nil, imc.ErrCorrupt
	}
	summary.CreatedAt, summary.SavedAt = e.Summary.CreatedAt, e.Summary.SavedAt
	if summary != e.Summary || m.Target <= 0 || m.Draft < 0 {
		return nil, imc.ErrCorrupt
	}
	for _, size := range m.Media {
		if size < 0 {
			return nil, imc.ErrCorrupt
		}
	}
	h := session{file: f, blob: e.Blob, meta: m, offset: 48 + int64(length)}
	return &h, nil
}

func (s *session) Record() imc.Record {
	r := s.meta.Record
	r.Request = slices.Clone(r.Request)
	r.Restore = slices.Clone(r.Restore)
	return r
}

func (s *session) Sizes() (int64, int64, []int64) {
	return s.meta.Target, s.meta.Draft, slices.Clone(s.meta.Media)
}

func (s *session) Read(ctx context.Context, target, draft []byte, media [][]byte) (retErr error) {
	if s.closed {
		return fs.ErrClosed
	}
	if int64(len(target)) != s.meta.Target || int64(len(draft)) != s.meta.Draft || len(media) != len(s.meta.Media) {
		return errors.New("disk: incorrect payload buffer sizes")
	}
	for i, data := range media {
		if int64(len(data)) != s.meta.Media[i] {
			return errors.New("disk: incorrect media buffer size")
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := s.file.Seek(s.offset, io.SeekStart); err != nil {
		return err
	}
	z, err := gzip.NewReader(contextReader{ctx, s.file})
	if err != nil {
		return errors.Join(imc.ErrCorrupt, err)
	}
	defer func() { retErr = errors.Join(retErr, z.Close()) }()
	read := func(data []byte) error {
		_, err := io.ReadFull(contextReader{ctx, z}, data)
		if err != nil {
			return errors.Join(imc.ErrCorrupt, err)
		}
		return nil
	}
	if err := read(target); err != nil {
		return err
	}
	if err := read(draft); err != nil {
		return err
	}
	for _, data := range media {
		if err := read(data); err != nil {
			return err
		}
	}
	// Read through the trailer to check gzip CRC and reject extra payload bytes.
	var extra [1]byte
	n, err := z.Read(extra[:])
	if n != 0 || !errors.Is(err, io.EOF) {
		return errors.Join(imc.ErrCorrupt, err)
	}
	return ctx.Err()
}

func (s *session) Close() error {
	if s.closed {
		return nil
	}
	s.store.gate <- struct{}{}
	defer s.store.release()
	s.closed = true
	err := s.file.Close()
	s.store.pins[s.blob]--
	if s.store.pins[s.blob] == 0 {
		delete(s.store.pins, s.blob)
		if s.store.retired[s.blob] {
			delete(s.store.retired, s.blob)
			err = errors.Join(err, removeFile(filepath.Join(s.store.directory, s.blob)))
		}
	}
	return err
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p[:min(len(p), 64<<10)])
}
