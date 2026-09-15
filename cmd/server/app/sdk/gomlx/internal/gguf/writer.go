// Package gguf implements the bounded GGUF subset needed by Moxie's model
// importers and LoRA adapter exporters.
package gguf

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"slices"
)

const (
	Version   = 3
	Alignment = 32
)

type metadata struct {
	key string
	typ uint32
	val any
}

// Tensor is an F32 tensor. Dimensions are in GGML order and Data contains
// row-major little-endian float32 values.
type Tensor struct {
	Name       string
	Dimensions []uint64
	Data       []float32
}

// Writer accumulates an ordered, deterministic GGUF document.
type Writer struct {
	metadata []metadata
	tensors  []Tensor
}

func (w *Writer) String(key, value string) { w.metadata = append(w.metadata, metadata{key, 8, value}) }
func (w *Writer) Uint32(key string, value uint32) {
	w.metadata = append(w.metadata, metadata{key, 4, value})
}
func (w *Writer) Float32(key string, value float32) {
	w.metadata = append(w.metadata, metadata{key, 6, value})
}
func (w *Writer) Bool(key string, value bool) {
	w.metadata = append(w.metadata, metadata{key, 7, value})
}
func (w *Writer) Strings(key string, value []string) {
	w.metadata = append(w.metadata, metadata{key, 9, slices.Clone(value)})
}
func (w *Writer) Float32s(key string, value []float32) {
	w.metadata = append(w.metadata, metadata{key, 9, slices.Clone(value)})
}
func (w *Writer) Int32s(key string, value []int32) {
	w.metadata = append(w.metadata, metadata{key, 9, slices.Clone(value)})
}
func (w *Writer) AddTensor(tensor Tensor) { w.tensors = append(w.tensors, tensor) }

// WriteFile creates path without overwriting an existing file.
func (w *Writer) WriteFile(path string) error {
	if path == "" {
		return fmt.Errorf("GGUF output path must not be empty")
	}
	data, err := w.Bytes()
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create GGUF %q: %w", path, err)
	}
	written := false
	defer func() {
		if !written {
			_ = os.Remove(path)
		}
	}()
	if _, err = file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write GGUF %q: %w", path, err)
	}
	if err = file.Close(); err != nil {
		return fmt.Errorf("close GGUF %q: %w", path, err)
	}
	written = true
	return nil
}

// Bytes validates and encodes the document.
func (w *Writer) Bytes() ([]byte, error) {
	if err := w.validate(); err != nil {
		return nil, err
	}
	var data bytes.Buffer
	data.WriteString("GGUF")
	put(&data, uint32(Version))
	put(&data, uint64(len(w.tensors)))
	put(&data, uint64(len(w.metadata)))
	for _, item := range w.metadata {
		writeString(&data, item.key)
		put(&data, item.typ)
		switch value := item.val.(type) {
		case string:
			writeString(&data, value)
		case uint32, float32, bool:
			put(&data, value)
		case []string:
			put(&data, uint32(8))
			put(&data, uint64(len(value)))
			for _, entry := range value {
				writeString(&data, entry)
			}
		case []float32:
			put(&data, uint32(6))
			put(&data, uint64(len(value)))
			for _, entry := range value {
				put(&data, entry)
			}
		case []int32:
			put(&data, uint32(5))
			put(&data, uint64(len(value)))
			for _, entry := range value {
				put(&data, entry)
			}
		}
	}

	offset := uint64(0)
	for _, tensor := range w.tensors {
		writeString(&data, tensor.Name)
		put(&data, uint32(len(tensor.Dimensions)))
		for _, dimension := range tensor.Dimensions {
			put(&data, dimension)
		}
		put(&data, uint32(0)) // GGML F32.
		put(&data, offset)
		offset = align(offset + uint64(len(tensor.Data))*4)
	}
	pad(&data)
	for _, tensor := range w.tensors {
		for _, value := range tensor.Data {
			put(&data, math.Float32bits(value))
		}
		pad(&data)
	}
	return data.Bytes(), nil
}

func (w *Writer) validate() error {
	keys := make(map[string]bool, len(w.metadata))
	for _, item := range w.metadata {
		if item.key == "" {
			return fmt.Errorf("metadata key must not be empty")
		}
		if keys[item.key] {
			return fmt.Errorf("duplicate metadata key %q", item.key)
		}
		keys[item.key] = true
	}
	names := make(map[string]bool, len(w.tensors))
	for _, tensor := range w.tensors {
		if tensor.Name == "" {
			return fmt.Errorf("tensor name must not be empty")
		}
		if names[tensor.Name] {
			return fmt.Errorf("duplicate tensor name %q", tensor.Name)
		}
		names[tensor.Name] = true
		if len(tensor.Dimensions) == 0 {
			return fmt.Errorf("tensor %q has no dimensions", tensor.Name)
		}
		count := uint64(1)
		for _, dimension := range tensor.Dimensions {
			if dimension == 0 || count > math.MaxUint64/dimension {
				return fmt.Errorf("tensor %q has invalid dimensions %v", tensor.Name, tensor.Dimensions)
			}
			count *= dimension
		}
		if count != uint64(len(tensor.Data)) {
			return fmt.Errorf("tensor %q dimensions %v contain %d values, got %d", tensor.Name, tensor.Dimensions, count, len(tensor.Data))
		}
	}
	return nil
}

func put(data *bytes.Buffer, value any) { _ = binary.Write(data, binary.LittleEndian, value) }
func writeString(data *bytes.Buffer, value string) {
	put(data, uint64(len(value)))
	data.WriteString(value)
}
func align(size uint64) uint64 { return (size + Alignment - 1) &^ (Alignment - 1) }
func pad(data *bytes.Buffer) {
	data.Write(slices.Repeat([]byte{0}, int(align(uint64(data.Len()))-uint64(data.Len()))))
}
