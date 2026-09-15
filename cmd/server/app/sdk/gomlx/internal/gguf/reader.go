package gguf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
)

const (
	metadataUint8 = iota
	metadataInt8
	metadataUint16
	metadataInt16
	metadataUint32
	metadataInt32
	metadataFloat32
	metadataBool
	metadataString
	metadataArray
	metadataUint64
	metadataInt64
	metadataFloat64
)

// TensorInfo describes a tensor without loading its payload. Offset is relative
// to the aligned GGUF data section.
type TensorInfo struct {
	Name       string
	Dimensions []uint64
	Type       uint32
	Offset     uint64
}

type arrayValue struct {
	typ    uint32
	length uint64
}

// Reader provides bounded, random-access reading of a GGUF v3 file.
type Reader struct {
	file         *os.File
	size         uint64
	version      uint32
	dataOffset   uint64
	metadata     map[string]any
	tensors      []TensorInfo
	tensorByName map[string]int
}

// OpenReader parses metadata and tensor descriptors without loading tensor data.
func OpenReader(path string) (*Reader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open GGUF %q: %w", path, err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("stat GGUF %q: %w", path, err)
	}
	reader := &Reader{
		file: file, size: uint64(info.Size()), metadata: make(map[string]any), tensorByName: make(map[string]int),
	}
	if err := reader.parse(); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("parse GGUF %q: %w", path, err)
	}
	return reader, nil
}

func (r *Reader) Close() error    { return r.file.Close() }
func (r *Reader) Version() uint32 { return r.version }
func (r *Reader) Tensors() []TensorInfo {
	result := slices.Clone(r.tensors)
	for index := range result {
		result[index].Dimensions = slices.Clone(result[index].Dimensions)
	}
	return result
}

func (r *Reader) String(key string) (string, error)   { return scalar[string](r, key) }
func (r *Reader) Uint32(key string) (uint32, error)   { return scalar[uint32](r, key) }
func (r *Reader) Float32(key string) (float32, error) { return scalar[float32](r, key) }
func (r *Reader) Bool(key string) (bool, error)       { return scalar[bool](r, key) }
func (r *Reader) HasMetadata(key string) bool {
	_, exists := r.metadata[key]
	return exists
}
func (r *Reader) ArrayLength(key string) (uint64, error) {
	value, exists := r.metadata[key]
	if !exists {
		return 0, fmt.Errorf("missing metadata %q", key)
	}
	array, ok := value.(arrayValue)
	if !ok {
		return 0, fmt.Errorf("metadata %q is not an array", key)
	}
	return array.length, nil
}

func scalar[T any](reader *Reader, key string) (T, error) {
	var zero T
	value, exists := reader.metadata[key]
	if !exists {
		return zero, fmt.Errorf("missing metadata %q", key)
	}
	result, ok := value.(T)
	if !ok {
		return zero, fmt.Errorf("metadata %q has unexpected type", key)
	}
	return result, nil
}

// ReadTensorF32 loads one F32 payload or dequantizes one Q8_0 payload.
func (r *Reader) ReadTensorF32(name string) ([]float32, error) {
	index, exists := r.tensorByName[name]
	if !exists {
		return nil, fmt.Errorf("missing tensor %q", name)
	}
	tensor := r.tensors[index]
	count := uint64(1)
	for _, dimension := range tensor.Dimensions {
		if dimension == 0 || count > math.MaxUint64/dimension {
			return nil, fmt.Errorf("tensor %q has invalid dimensions", name)
		}
		count *= dimension
	}
	if count > uint64(maxInt()) {
		return nil, fmt.Errorf("tensor %q payload is too large", name)
	}

	var length uint64
	switch tensor.Type {
	case 0: // F32.
		if count > math.MaxUint64/4 {
			return nil, fmt.Errorf("tensor %q payload is too large", name)
		}
		length = count * 4
	case 8: // Q8_0.
		if tensor.Dimensions[0]%32 != 0 {
			return nil, fmt.Errorf("tensor %q Q8_0 first dimension %d is not divisible by 32", name, tensor.Dimensions[0])
		}
		blocks := count / 32
		if blocks > math.MaxUint64/34 {
			return nil, fmt.Errorf("tensor %q payload is too large", name)
		}
		length = blocks * 34
	default:
		return nil, fmt.Errorf("tensor %q has unsupported GGML type %d: conversion/dequantization is not implemented", name, tensor.Type)
	}
	if length > uint64(maxInt()) {
		return nil, fmt.Errorf("tensor %q payload is too large", name)
	}
	absolute, ok := add(r.dataOffset, tensor.Offset)
	if !ok || absolute > r.size || length > r.size-absolute {
		return nil, fmt.Errorf("tensor %q payload is truncated or out of bounds", name)
	}
	buffer := make([]byte, int(length))
	if _, err := r.file.ReadAt(buffer, int64(absolute)); err != nil {
		return nil, fmt.Errorf("read tensor %q: %w", name, err)
	}
	data := make([]float32, int(count))
	if tensor.Type == 0 {
		for index := range data {
			data[index] = math.Float32frombits(binary.LittleEndian.Uint32(buffer[index*4:]))
		}
		return data, nil
	}
	for block := range len(data) / 32 {
		start := block * 34
		scale := float16ToFloat32(binary.LittleEndian.Uint16(buffer[start:]))
		for index, quantized := range buffer[start+2 : start+34] {
			data[block*32+index] = scale * float32(int8(quantized))
		}
	}
	return data, nil
}

func float16ToFloat32(value uint16) float32 {
	sign := uint32(value&0x8000) << 16
	exponent := uint32(value>>10) & 0x1f
	fraction := uint32(value & 0x03ff)
	switch exponent {
	case 0:
		if fraction == 0 {
			return math.Float32frombits(sign)
		}
		exponent = 113
		for fraction&0x0400 == 0 {
			fraction <<= 1
			exponent--
		}
		fraction &= 0x03ff
	case 0x1f:
		exponent = 0xff
	default:
		exponent += 112
	}
	return math.Float32frombits(sign | exponent<<23 | fraction<<13)
}

func (r *Reader) parse() error {
	parser := parser{file: r.file, size: r.size}
	magic, err := parser.bytes(4)
	if err != nil || string(magic) != "GGUF" {
		return fmt.Errorf("invalid or truncated GGUF magic")
	}
	if r.version, err = parser.u32(); err != nil {
		return err
	}
	tensorCount, err := parser.u64()
	if err != nil {
		return err
	}
	metadataCount, err := parser.u64()
	if err != nil {
		return err
	}
	if tensorCount > 10_000_000 || metadataCount > 10_000_000 {
		return fmt.Errorf("unreasonable header counts")
	}
	for range metadataCount {
		key, err := parser.string()
		if err != nil {
			return err
		}
		if key == "" {
			return fmt.Errorf("empty metadata key")
		}
		if _, exists := r.metadata[key]; exists {
			return fmt.Errorf("duplicate metadata key %q", key)
		}
		typ, err := parser.u32()
		if err != nil {
			return err
		}
		value, err := parser.metadataValue(typ, true)
		if err != nil {
			return fmt.Errorf("metadata %q: %w", key, err)
		}
		r.metadata[key] = value
	}
	r.tensors = make([]TensorInfo, 0, int(tensorCount))
	for range tensorCount {
		name, err := parser.string()
		if err != nil {
			return err
		}
		if name == "" {
			return fmt.Errorf("empty tensor name")
		}
		if _, exists := r.tensorByName[name]; exists {
			return fmt.Errorf("duplicate tensor %q", name)
		}
		rank, err := parser.u32()
		if err != nil {
			return err
		}
		if rank == 0 || rank > 8 {
			return fmt.Errorf("tensor %q has invalid rank %d", name, rank)
		}
		dimensions := make([]uint64, rank)
		for index := range dimensions {
			if dimensions[index], err = parser.u64(); err != nil {
				return err
			}
			if dimensions[index] == 0 {
				return fmt.Errorf("tensor %q has zero dimension", name)
			}
		}
		typ, err := parser.u32()
		if err != nil {
			return err
		}
		offset, err := parser.u64()
		if err != nil {
			return err
		}
		r.tensorByName[name] = len(r.tensors)
		r.tensors = append(r.tensors, TensorInfo{Name: name, Dimensions: dimensions, Type: typ, Offset: offset})
	}

	alignment := uint64(Alignment)
	if value, exists := r.metadata["general.alignment"]; exists {
		configured, ok := value.(uint32)
		if !ok || configured == 0 || configured&(configured-1) != 0 {
			return fmt.Errorf("invalid general.alignment")
		}
		alignment = uint64(configured)
	}
	if parser.offset > math.MaxUint64-(alignment-1) {
		return fmt.Errorf("data section offset overflow")
	}
	r.dataOffset = (parser.offset + alignment - 1) &^ (alignment - 1)
	if r.dataOffset > r.size {
		return fmt.Errorf("truncated before tensor data section")
	}
	return nil
}

type parser struct {
	file         *os.File
	size, offset uint64
}

func (p *parser) bytes(size uint64) ([]byte, error) {
	if size > uint64(maxInt()) || p.offset > p.size || size > p.size-p.offset {
		return nil, io.ErrUnexpectedEOF
	}
	data := make([]byte, int(size))
	_, err := p.file.ReadAt(data, int64(p.offset))
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	p.offset += size
	return data, nil
}

func (p *parser) u32() (uint32, error) {
	data, err := p.bytes(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(data), nil
}

func (p *parser) u64() (uint64, error) {
	data, err := p.bytes(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(data), nil
}

func (p *parser) string() (string, error) {
	size, err := p.u64()
	if err != nil {
		return "", err
	}
	data, err := p.bytes(size)
	return string(data), err
}

func (p *parser) metadataValue(typ uint32, retain bool) (any, error) {
	sizes := [...]uint64{1, 1, 2, 2, 4, 4, 4, 1, 0, 0, 8, 8, 8}
	if typ > metadataFloat64 {
		return nil, fmt.Errorf("invalid metadata type %d", typ)
	}
	if typ == metadataString {
		return p.string()
	}
	if typ == metadataArray {
		element, err := p.u32()
		if err != nil {
			return nil, err
		}
		length, err := p.u64()
		if err != nil {
			return nil, err
		}
		if element > metadataFloat64 {
			return nil, fmt.Errorf("invalid array element type %d", element)
		}
		minimum := sizes[element]
		if element == metadataString {
			minimum = 8
		} else if element == metadataArray {
			minimum = 12
		}
		if p.offset > p.size || length > (p.size-p.offset)/minimum {
			return nil, io.ErrUnexpectedEOF
		}
		for range length {
			if _, err = p.metadataValue(element, false); err != nil {
				return nil, err
			}
		}
		return arrayValue{typ: element, length: length}, nil
	}
	data, err := p.bytes(sizes[typ])
	if err != nil {
		return nil, err
	}
	if !retain {
		return nil, nil
	}
	switch typ {
	case metadataUint32:
		return binary.LittleEndian.Uint32(data), nil
	case metadataFloat32:
		return math.Float32frombits(binary.LittleEndian.Uint32(data)), nil
	case metadataBool:
		if data[0] > 1 {
			return nil, fmt.Errorf("invalid bool")
		}
		return data[0] == 1, nil
	}
	return slices.Clone(data), nil
}

func add(left, right uint64) (uint64, bool) {
	if left > math.MaxUint64-right {
		return 0, false
	}
	return left + right, true
}

func maxInt() int { return int(^uint(0) >> 1) }
