package gguf

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// Bound both the scan and the eventual allocation independently of the model's
// tensor payload size. Individual strings retain the parser's 1 MiB limit.
const maxLocalHeaderSize = 256 * 1024 * 1024

// headerScanner tracks a bounded stream position without allocating values.
type headerScanner struct {
	reader    *bufio.Reader
	remaining int64
}

func (s *headerScanner) Read(p []byte) (int, error) {
	n, err := s.reader.Read(p)
	s.remaining -= int64(n)
	return n, err
}

func (s *headerScanner) skip(n uint64) error {
	if n > uint64(s.remaining) {
		return fmt.Errorf("header extent exceeds available bytes or %d-byte limit", maxLocalHeaderSize)
	}
	nRead, err := s.reader.Discard(int(n))
	s.remaining -= int64(nRead)
	return err
}

func (s *headerScanner) skipString() error {
	var length uint64
	if err := binary.Read(s, binary.LittleEndian, &length); err != nil {
		return err
	}
	if length > 1*1024*1024 {
		return fmt.Errorf("string length too large: %d", length)
	}
	return s.skip(length)
}

func (s *headerScanner) skipValue(valueType uint32, depth int) error {
	var size uint64
	switch valueType {
	case MetadataValueTypeUInt8, MetadataValueTypeInt8, MetadataValueTypeBool:
		size = 1
	case MetadataValueTypeUInt16, MetadataValueTypeInt16:
		size = 2
	case MetadataValueTypeUInt32, MetadataValueTypeInt32, MetadataValueTypeFloat32:
		size = 4
	case MetadataValueTypeUInt64, MetadataValueTypeInt64, MetadataValueTypeFloat64:
		size = 8
	case MetadataValueTypeString:
		return s.skipString()
	case MetadataValueTypeArray:
		if depth >= 32 {
			return fmt.Errorf("metadata array nesting exceeds 32 levels")
		}
		var arrayType uint32
		if err := binary.Read(s, binary.LittleEndian, &arrayType); err != nil {
			return err
		}
		var count uint64
		if err := binary.Read(s, binary.LittleEndian, &count); err != nil {
			return err
		}
		if count > uint64(s.remaining) {
			return fmt.Errorf("array length %d exceeds remaining header bytes %d", count, s.remaining)
		}
		for range count {
			if err := s.skipValue(arrayType, depth+1); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported metadata value type: %d", valueType)
	}
	return s.skip(size)
}

// localHeaderExtent finds the end of the tensor descriptors, not tensor data
// or alignment padding. SectionReader leaves the caller's file position intact.
func localHeaderExtent(f *os.File) (int64, error) {
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	limit := min(info.Size(), int64(maxLocalHeaderSize))
	s := headerScanner{
		reader:    bufio.NewReader(io.NewSectionReader(f, 0, limit)),
		remaining: limit,
	}
	var header Header
	if err := binary.Read(&s, binary.LittleEndian, &header); err != nil {
		return 0, err
	}
	// Empty-key metadata entries need at least 13 bytes; empty-name tensor
	// descriptors need at least 24. Check counts before iterating.
	if header.MetadataKvCount > uint64(s.remaining/13) || header.TensorCount > uint64(s.remaining/24) {
		return 0, fmt.Errorf("entry counts exceed available header bytes")
	}
	for i := range header.MetadataKvCount {
		if err := s.skipString(); err != nil {
			return 0, fmt.Errorf("metadata key %d: %w", i, err)
		}
		var valueType uint32
		if err := binary.Read(&s, binary.LittleEndian, &valueType); err != nil {
			return 0, err
		}
		if err := s.skipValue(valueType, 0); err != nil {
			return 0, fmt.Errorf("metadata value %d: %w", i, err)
		}
	}
	for i := range header.TensorCount {
		if err := s.skipString(); err != nil {
			return 0, fmt.Errorf("tensor name %d: %w", i, err)
		}
		var dims uint32
		if err := binary.Read(&s, binary.LittleEndian, &dims); err != nil {
			return 0, err
		}
		// Dimension lengths, type, and offset only; never follow tensor offsets.
		if err := s.skip(uint64(dims)*8 + 12); err != nil {
			return 0, fmt.Errorf("tensor descriptor %d: %w", i, err)
		}
	}
	return limit - s.remaining, nil
}
