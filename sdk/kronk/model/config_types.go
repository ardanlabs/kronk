package model

import (
	"encoding/json"
	"fmt"
	"strings"

	internalspec "github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// GGMLType represents a ggml data type for the KV cache. Its zero value is auto.
type GGMLType struct {
	value string
}

var ggmlTypes = make(map[string]GGMLType)

var (
	// GGMLTypeAuto uses the native default.
	GGMLTypeAuto = newGGMLType("auto")
	// GGMLTypeF32 uses 32-bit floating point.
	GGMLTypeF32 = newGGMLType("f32")
	// GGMLTypeF16 uses 16-bit floating point.
	GGMLTypeF16 = newGGMLType("f16")
	// GGMLTypeQ4_0 uses 4-bit quantization (type 0).
	GGMLTypeQ4_0 = newGGMLType("q4_0")
	// GGMLTypeQ4_1 uses 4-bit quantization (type 1).
	GGMLTypeQ4_1 = newGGMLType("q4_1")
	// GGMLTypeQ5_0 uses 5-bit quantization (type 0).
	GGMLTypeQ5_0 = newGGMLType("q5_0")
	// GGMLTypeQ5_1 uses 5-bit quantization (type 1).
	GGMLTypeQ5_1 = newGGMLType("q5_1")
	// GGMLTypeQ8_0 uses 8-bit quantization.
	GGMLTypeQ8_0 = newGGMLType("q8_0")
	// GGMLTypeBF16 uses brain floating point 16-bit.
	GGMLTypeBF16 = newGGMLType("bf16")
)

func newGGMLType(value string) GGMLType {
	t := GGMLType{value: value}
	if value == "auto" {
		t = GGMLType{}
	}
	ggmlTypes[value] = t
	return t
}

// String returns the canonical cache type name.
func (t GGMLType) String() string {
	if t.IsZero() {
		return "auto"
	}
	return t.value
}

// Equal provides support for comparison and testing.
func (t GGMLType) Equal(other GGMLType) bool { return t.value == other.value }

// IsZero reports whether the cache type is unset.
func (t GGMLType) IsZero() bool { return t.value == "" }

// MarshalText provides support for logging and serialization.
func (t GGMLType) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

// UnmarshalText parses a known cache type.
func (t *GGMLType) UnmarshalText(data []byte) error {
	parsed, err := ParseGGMLType(string(data))
	if err != nil {
		return err
	}
	*t = parsed
	return nil
}

// MarshalJSON implements json.Marshaler.
func (t GGMLType) MarshalJSON() ([]byte, error) { return json.Marshal(t.String()) }

// UnmarshalJSON implements json.Unmarshaler.
func (t *GGMLType) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	return t.UnmarshalText([]byte(value))
}

// MarshalYAML implements yaml.Marshaler.
func (t GGMLType) MarshalYAML() (any, error) { return t.String(), nil }

// UnmarshalYAML implements yaml.Unmarshaler.
func (t *GGMLType) UnmarshalYAML(unmarshal func(any) error) error {
	var value string
	if err := unmarshal(&value); err != nil {
		return err
	}
	return t.UnmarshalText([]byte(value))
}

// ParseGGMLType parses a cache type name or a supported alias.
func ParseGGMLType(value string) (GGMLType, error) {
	name := strings.ToLower(strings.TrimSpace(value))
	switch name {
	case "":
		name = "auto"
	case "fp32":
		name = "f32"
	case "fp16":
		name = "f16"
	case "q4":
		name = "q4_0"
	case "q5":
		name = "q5_0"
	case "f8", "q8":
		name = "q8_0"
	case "bfloat16":
		name = "bf16"
	}
	t, exists := ggmlTypes[name]
	if !exists {
		return GGMLTypeAuto, fmt.Errorf("unknown ggml type: %s", value)
	}
	return t, nil
}

// MustParseGGMLType parses a cache type and panics for an unknown name.
func MustParseGGMLType(value string) GGMLType {
	t, err := ParseGGMLType(value)
	if err != nil {
		panic(err)
	}
	return t
}

// ToYZMAType converts to the native cache type; auto is the native -1 sentinel.
func (t GGMLType) ToYZMAType() llama.GGMLType {
	switch t {
	case GGMLTypeF32:
		return llama.GGMLTypeF32
	case GGMLTypeF16:
		return llama.GGMLTypeF16
	case GGMLTypeQ4_0:
		return llama.GGMLTypeQ4_0
	case GGMLTypeQ4_1:
		return llama.GGMLTypeQ4_1
	case GGMLTypeQ5_0:
		return llama.GGMLTypeQ5_0
	case GGMLTypeQ5_1:
		return llama.GGMLTypeQ5_1
	case GGMLTypeQ8_0:
		return llama.GGMLTypeQ8_0
	case GGMLTypeBF16:
		return llama.GGMLTypeBF16
	default:
		return llama.GGMLType(-1)
	}
}

// ToVRAMType returns the cache type ID used by Kronk's memory estimators.
// Unlike native GGML IDs, 0 means auto and 50 means F32.
func (t GGMLType) ToVRAMType() int32 {
	switch t {
	case GGMLTypeAuto:
		return 0
	case GGMLTypeF32:
		return 50
	default:
		return int32(t.ToYZMAType())
	}
}

// GGMLTypeFromYZMA converts a supported native cache type, defaulting to auto.
func GGMLTypeFromYZMA(t llama.GGMLType) GGMLType {
	switch t {
	case llama.GGMLTypeF32:
		return GGMLTypeF32
	case llama.GGMLTypeF16:
		return GGMLTypeF16
	case llama.GGMLTypeQ4_0:
		return GGMLTypeQ4_0
	case llama.GGMLTypeQ4_1:
		return GGMLTypeQ4_1
	case llama.GGMLTypeQ5_0:
		return GGMLTypeQ5_0
	case llama.GGMLTypeQ5_1:
		return GGMLTypeQ5_1
	case llama.GGMLTypeQ8_0:
		return GGMLTypeQ8_0
	case llama.GGMLTypeBF16:
		return GGMLTypeBF16
	default:
		return GGMLTypeAuto
	}
}

// =============================================================================

// FlashAttentionType controls Flash Attention. Its zero value is enabled.
type FlashAttentionType struct {
	value string
}

var flashAttentionTypes = make(map[string]FlashAttentionType)

var (
	// FlashAttentionEnabled forces Flash Attention on.
	FlashAttentionEnabled = newFlashAttentionType("enabled")
	// FlashAttentionDisabled forces Flash Attention off.
	FlashAttentionDisabled = newFlashAttentionType("disabled")
	// FlashAttentionAuto lets llama.cpp decide.
	FlashAttentionAuto = newFlashAttentionType("auto")
)

func newFlashAttentionType(value string) FlashAttentionType {
	t := FlashAttentionType{value: value}
	flashAttentionTypes[value] = t
	return t
}

// String returns the canonical Flash Attention name.
func (t FlashAttentionType) String() string {
	if t.IsZero() {
		return "enabled"
	}
	return t.value
}

// Equal provides support for comparison and testing.
func (t FlashAttentionType) Equal(other FlashAttentionType) bool { return t.String() == other.String() }

// IsZero reports whether the mode is unset.
func (t FlashAttentionType) IsZero() bool { return t.value == "" }

// MarshalText provides support for logging and serialization.
func (t FlashAttentionType) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

// UnmarshalText parses a known Flash Attention mode.
func (t *FlashAttentionType) UnmarshalText(data []byte) error {
	parsed, err := ParseFlashAttentionType(string(data))
	if err != nil {
		return err
	}
	*t = parsed
	return nil
}

// MarshalJSON implements json.Marshaler.
func (t FlashAttentionType) MarshalJSON() ([]byte, error) { return json.Marshal(t.String()) }

// UnmarshalJSON implements json.Unmarshaler.
func (t *FlashAttentionType) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	return t.UnmarshalText([]byte(value))
}

// MarshalYAML implements yaml.Marshaler.
func (t FlashAttentionType) MarshalYAML() (any, error) { return t.String(), nil }

// UnmarshalYAML implements yaml.Unmarshaler.
func (t *FlashAttentionType) UnmarshalYAML(unmarshal func(any) error) error {
	var value string
	if err := unmarshal(&value); err != nil {
		return err
	}
	return t.UnmarshalText([]byte(value))
}

// ParseFlashAttentionType parses a mode name or a supported boolean alias.
func ParseFlashAttentionType(value string) (FlashAttentionType, error) {
	name := strings.ToLower(strings.TrimSpace(value))
	switch name {
	case "on", "true", "1":
		name = "enabled"
	case "off", "false", "0":
		name = "disabled"
	case "":
		name = "auto"
	}
	t, exists := flashAttentionTypes[name]
	if !exists {
		return FlashAttentionType{}, fmt.Errorf("unknown flash attention type: %s", value)
	}
	return t, nil
}

// MustParseFlashAttentionType parses a mode and panics for an unknown name.
func MustParseFlashAttentionType(value string) FlashAttentionType {
	t, err := ParseFlashAttentionType(value)
	if err != nil {
		panic(err)
	}
	return t
}

func (t FlashAttentionType) toYZMAType() llama.FlashAttentionType {
	switch t {
	case FlashAttentionDisabled:
		return llama.FlashAttentionTypeDisabled
	case FlashAttentionAuto:
		return llama.FlashAttentionTypeAuto
	default:
		return llama.FlashAttentionTypeEnabled
	}
}

// DerefFlashAttention returns the mode, defaulting to auto when nil.
func DerefFlashAttention(p *FlashAttentionType) FlashAttentionType {
	if p == nil {
		return FlashAttentionAuto
	}
	return *p
}

// =============================================================================

// LazyMode controls on-demand tensor reading. Its zero value is auto.
type LazyMode struct {
	value string
}

var lazyModes = make(map[string]LazyMode)

var (
	// LazyModeAuto enables lazy reading for eligible tensors larger than 4 GiB
	// when all selected devices support mmap.
	LazyModeAuto = newLazyMode("auto")
	// LazyModeOff reads eligible tensors up front like ordinary weights.
	LazyModeOff = newLazyMode("off")
	// LazyModeOn requests lazy reading regardless of eligible tensor size.
	LazyModeOn = newLazyMode("on")
)

func newLazyMode(value string) LazyMode {
	m := LazyMode{value: value}
	lazyModes[value] = m
	return m
}

// String returns the canonical lazy mode name.
func (m LazyMode) String() string {
	if m.IsZero() {
		return "auto"
	}
	return m.value
}

// Equal provides support for comparison and testing.
func (m LazyMode) Equal(other LazyMode) bool { return m.String() == other.String() }

// IsZero reports whether the mode is unset.
func (m LazyMode) IsZero() bool { return m.value == "" }

// MarshalText provides support for logging and serialization.
func (m LazyMode) MarshalText() ([]byte, error) { return []byte(m.String()), nil }

// UnmarshalText parses a known lazy mode.
func (m *LazyMode) UnmarshalText(data []byte) error {
	parsed, err := ParseLazyMode(string(data))
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

// MarshalJSON implements json.Marshaler.
func (m LazyMode) MarshalJSON() ([]byte, error) { return json.Marshal(m.String()) }

// UnmarshalJSON implements json.Unmarshaler.
func (m *LazyMode) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	return m.UnmarshalText([]byte(value))
}

// MarshalYAML implements yaml.Marshaler.
func (m LazyMode) MarshalYAML() (any, error) { return m.String(), nil }

// UnmarshalYAML implements yaml.Unmarshaler.
func (m *LazyMode) UnmarshalYAML(unmarshal func(any) error) error {
	var value string
	if err := unmarshal(&value); err != nil {
		return err
	}
	return m.UnmarshalText([]byte(value))
}

// ParseLazyMode parses a lazy mode name.
func ParseLazyMode(value string) (LazyMode, error) {
	name := strings.ToLower(strings.TrimSpace(value))
	if name == "" {
		name = "auto"
	}
	m, exists := lazyModes[name]
	if !exists {
		return LazyModeAuto, fmt.Errorf("parse-lazy-mode: unknown lazy mode: %s (valid: auto, off, on)", value)
	}
	return m, nil
}

// MustParseLazyMode parses a mode and panics for an unknown name.
func MustParseLazyMode(value string) LazyMode {
	m, err := ParseLazyMode(value)
	if err != nil {
		panic(err)
	}
	return m
}

// ToYZMAType converts to the native lazy mode.
func (m LazyMode) ToYZMAType() llama.LazyMode {
	switch m {
	case LazyModeOff:
		return llama.LazyModeOff
	case LazyModeOn:
		return llama.LazyModeOn
	default:
		return llama.LazyModeAuto
	}
}

// DerefLazyMode returns the mode, defaulting to auto when nil.
func DerefLazyMode(p *LazyMode) LazyMode {
	if p == nil {
		return LazyModeAuto
	}
	return *p
}

// =============================================================================

// KVCacheMode controls whether slots use dedicated partitions or shared capacity.
// The zero value is unset; Config defaults a nil pointer to partitioned.
type KVCacheMode struct {
	value string
}

var kvCacheModes = make(map[string]KVCacheMode)

var (
	// KVCachePartitioned reserves a context-window-sized partition per slot.
	KVCachePartitioned = newKVCacheMode("partitioned")
	// KVCacheShared shares aggregate capacity while Kronk limits each request.
	KVCacheShared = newKVCacheMode("shared")
)

func newKVCacheMode(value string) KVCacheMode {
	m := KVCacheMode{value: value}
	kvCacheModes[value] = m
	return m
}

// String returns the cache layout name.
func (m KVCacheMode) String() string { return m.value }

// Equal provides support for comparison and testing.
func (m KVCacheMode) Equal(other KVCacheMode) bool { return m.value == other.value }

// IsZero reports whether the layout is unset.
func (m KVCacheMode) IsZero() bool { return m.value == "" }

// MarshalText provides support for logging and serialization.
func (m KVCacheMode) MarshalText() ([]byte, error) { return []byte(m.value), nil }

// UnmarshalText parses a known cache layout.
func (m *KVCacheMode) UnmarshalText(data []byte) error {
	parsed, err := ParseKVCacheMode(string(data))
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

// MarshalJSON implements json.Marshaler.
func (m KVCacheMode) MarshalJSON() ([]byte, error) { return json.Marshal(m.value) }

// UnmarshalJSON implements json.Unmarshaler.
func (m *KVCacheMode) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	return m.UnmarshalText([]byte(value))
}

// MarshalYAML implements yaml.Marshaler.
func (m KVCacheMode) MarshalYAML() (any, error) { return m.value, nil }

// UnmarshalYAML implements yaml.Unmarshaler.
func (m *KVCacheMode) UnmarshalYAML(unmarshal func(any) error) error {
	var value string
	if err := unmarshal(&value); err != nil {
		return err
	}
	return m.UnmarshalText([]byte(value))
}

// ParseKVCacheMode validates a cache layout name. Names are case sensitive.
func ParseKVCacheMode(value string) (KVCacheMode, error) {
	m, exists := kvCacheModes[value]
	if !exists {
		return KVCacheMode{}, fmt.Errorf("parse-kv-cache-mode: unknown mode %q (valid: partitioned, shared)", value)
	}
	return m, nil
}

// MustParseKVCacheMode parses a layout and panics for an unknown name.
func MustParseKVCacheMode(value string) KVCacheMode {
	m, err := ParseKVCacheMode(value)
	if err != nil {
		panic(err)
	}
	return m
}

// =============================================================================

// LoadMode controls model weight loading. Its zero value is auto.
type LoadMode struct {
	value string
}

var loadModes = make(map[string]LoadMode)

var (
	// LoadModeAuto uses mmap when every selected device supports it.
	LoadModeAuto = newLoadMode("auto")
	// LoadModeMMap forces memory mapped weights.
	LoadModeMMap = newLoadMode("mmap")
	// LoadModeNone loads without mmap, mlock, or direct I/O.
	LoadModeNone = newLoadMode("none")
	// LoadModeMLock requests resident pages without forcing mmap.
	LoadModeMLock = newLoadMode("mlock")
	// LoadModeDirectIO bypasses the page cache where supported.
	LoadModeDirectIO = newLoadMode("direct-io")
	// LoadModeMMapMLock combines mmap and resident pages.
	LoadModeMMapMLock = newLoadMode("mmap+mlock")
)

func newLoadMode(value string) LoadMode {
	m := LoadMode{value: value}
	loadModes[value] = m
	return m
}

// String returns the canonical load mode name.
func (m LoadMode) String() string {
	if m.IsZero() {
		return "auto"
	}
	return m.value
}

// Equal provides support for comparison and testing.
func (m LoadMode) Equal(other LoadMode) bool { return m.String() == other.String() }

// IsZero reports whether the mode is unset.
func (m LoadMode) IsZero() bool { return m.value == "" }

// MarshalText provides support for logging and serialization.
func (m LoadMode) MarshalText() ([]byte, error) { return []byte(m.String()), nil }

// UnmarshalText parses a known load mode.
func (m *LoadMode) UnmarshalText(data []byte) error {
	parsed, err := ParseLoadMode(string(data))
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

// MarshalJSON implements json.Marshaler.
func (m LoadMode) MarshalJSON() ([]byte, error) { return json.Marshal(m.String()) }

// UnmarshalJSON implements json.Unmarshaler.
func (m *LoadMode) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	return m.UnmarshalText([]byte(value))
}

// MarshalYAML implements yaml.Marshaler.
func (m LoadMode) MarshalYAML() (any, error) { return m.String(), nil }

// UnmarshalYAML implements yaml.Unmarshaler.
func (m *LoadMode) UnmarshalYAML(unmarshal func(any) error) error {
	var value string
	if err := unmarshal(&value); err != nil {
		return err
	}
	return m.UnmarshalText([]byte(value))
}

// ParseLoadMode parses a load mode name or direct-I/O alias.
func ParseLoadMode(value string) (LoadMode, error) {
	name := strings.ToLower(strings.TrimSpace(value))
	switch name {
	case "":
		name = "auto"
	case "directio", "dio":
		name = "direct-io"
	}
	m, exists := loadModes[name]
	if !exists {
		return LoadModeAuto, fmt.Errorf("parse-load-mode: unknown load mode: %s (valid: auto, mmap, none, mlock, mmap+mlock, direct-io)", value)
	}
	return m, nil
}

// MustParseLoadMode parses a mode and panics for an unknown name.
func MustParseLoadMode(value string) LoadMode {
	m, err := ParseLoadMode(value)
	if err != nil {
		panic(err)
	}
	return m
}

// ToYZMAType converts to the native load mode.
func (m LoadMode) ToYZMAType() llama.LoadMode {
	switch m {
	case LoadModeMMap:
		return llama.LoadModeMmap
	case LoadModeNone:
		return llama.LoadModeNone
	case LoadModeMLock:
		return llama.LoadModeMlock
	case LoadModeMMapMLock:
		return llama.LoadModeMmapMlock
	case LoadModeDirectIO:
		return llama.LoadModeDirectIO
	default:
		return llama.LoadModeAuto
	}
}

// DerefLoadMode returns the mode, defaulting to auto when nil.
func DerefLoadMode(p *LoadMode) LoadMode {
	if p == nil {
		return LoadModeAuto
	}
	return *p
}

// =============================================================================

// SplitMode controls splitting across GPUs. Its zero value is none.
type SplitMode struct {
	value string
}

var splitModes = make(map[string]SplitMode)

var (
	// SplitModeNone uses a single GPU.
	SplitModeNone = newSplitMode("none")
	// SplitModeLayer splits layers and KV across GPUs.
	SplitModeLayer = newSplitMode("layer")
	// SplitModeRow uses legacy row-split parallelism.
	SplitModeRow = newSplitMode("row")
	// SplitModeTensor uses tensor parallelism.
	SplitModeTensor = newSplitMode("tensor")
)

func newSplitMode(value string) SplitMode {
	m := SplitMode{value: value}
	splitModes[value] = m
	return m
}

// String returns the canonical split mode name.
func (m SplitMode) String() string {
	if m.IsZero() {
		return "none"
	}
	return m.value
}

// Equal provides support for comparison and testing.
func (m SplitMode) Equal(other SplitMode) bool { return m.String() == other.String() }

// IsZero reports whether the mode is unset.
func (m SplitMode) IsZero() bool { return m.value == "" }

// MarshalText provides support for logging and serialization.
func (m SplitMode) MarshalText() ([]byte, error) { return []byte(m.String()), nil }

// UnmarshalText parses a known split mode.
func (m *SplitMode) UnmarshalText(data []byte) error {
	parsed, err := ParseSplitMode(string(data))
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

// MarshalJSON implements json.Marshaler.
func (m SplitMode) MarshalJSON() ([]byte, error) { return json.Marshal(m.String()) }

// UnmarshalJSON implements json.Unmarshaler.
func (m *SplitMode) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	return m.UnmarshalText([]byte(value))
}

// MarshalYAML implements yaml.Marshaler.
func (m SplitMode) MarshalYAML() (any, error) { return m.String(), nil }

// UnmarshalYAML implements yaml.Unmarshaler.
func (m *SplitMode) UnmarshalYAML(unmarshal func(any) error) error {
	var value string
	if err := unmarshal(&value); err != nil {
		return err
	}
	return m.UnmarshalText([]byte(value))
}

// ParseSplitMode parses a split mode name or a supported alias.
func ParseSplitMode(value string) (SplitMode, error) {
	name := strings.ToLower(strings.TrimSpace(value))
	switch name {
	case "single", "0", "":
		name = "none"
	case "1":
		name = "layer"
	case "2":
		name = "row"
	case "tensor-parallel", "expert-parallel", "3":
		name = "tensor"
	}
	m, exists := splitModes[name]
	if !exists {
		return SplitModeNone, fmt.Errorf("parse-split-mode: unknown split mode: %s (valid: none, layer, row, tensor)", value)
	}
	return m, nil
}

// MustParseSplitMode parses a mode and panics for an unknown name.
func MustParseSplitMode(value string) SplitMode {
	m, err := ParseSplitMode(value)
	if err != nil {
		panic(err)
	}
	return m
}

// ToYZMAType converts to the native split mode.
func (m SplitMode) ToYZMAType() llama.SplitMode {
	switch m {
	case SplitModeLayer:
		return llama.SplitModeLayer
	case SplitModeRow:
		return llama.SplitModeRow
	case SplitModeTensor:
		return llama.SplitModeTensor
	default:
		return llama.SplitModeNone
	}
}

// SplitModeFromYZMA converts a supported native split mode, defaulting to none.
func SplitModeFromYZMA(mode llama.SplitMode) SplitMode {
	switch mode {
	case llama.SplitModeLayer:
		return SplitModeLayer
	case llama.SplitModeRow:
		return SplitModeRow
	case llama.SplitModeTensor:
		return SplitModeTensor
	default:
		return SplitModeNone
	}
}

// DefaultSplitMode returns the native layer-split default. The GPU count is
// retained for API compatibility.
func DefaultSplitMode(_ int) SplitMode { return SplitModeLayer }

// =============================================================================

// RopeScalingType controls RoPE scaling. Its zero value is none.
type RopeScalingType struct {
	value string
}

var ropeScalingTypes = make(map[string]RopeScalingType)

var (
	// RopeScalingNone leaves native scaling defaults unchanged.
	RopeScalingNone = newRopeScalingType("none")
	// RopeScalingLinear uses linear interpolation scaling.
	RopeScalingLinear = newRopeScalingType("linear")
	// RopeScalingYaRN uses YaRN scaling.
	RopeScalingYaRN = newRopeScalingType("yarn")
)

func newRopeScalingType(value string) RopeScalingType {
	t := RopeScalingType{value: value}
	if value == "none" {
		t = RopeScalingType{}
	}
	ropeScalingTypes[value] = t
	return t
}

// String returns the canonical scaling name.
func (t RopeScalingType) String() string {
	if t.IsZero() {
		return "none"
	}
	return t.value
}

// Equal provides support for comparison and testing.
func (t RopeScalingType) Equal(other RopeScalingType) bool { return t.value == other.value }

// IsZero reports whether the scaling mode is unset.
func (t RopeScalingType) IsZero() bool { return t.value == "" }

// MarshalText provides support for logging and serialization.
func (t RopeScalingType) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

// UnmarshalText parses a known scaling mode.
func (t *RopeScalingType) UnmarshalText(data []byte) error {
	parsed, err := ParseRopeScalingType(string(data))
	if err != nil {
		return err
	}
	*t = parsed
	return nil
}

// MarshalJSON implements json.Marshaler.
func (t RopeScalingType) MarshalJSON() ([]byte, error) { return json.Marshal(t.String()) }

// UnmarshalJSON implements json.Unmarshaler.
func (t *RopeScalingType) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	return t.UnmarshalText([]byte(value))
}

// MarshalYAML implements yaml.Marshaler.
func (t RopeScalingType) MarshalYAML() (any, error) { return t.String(), nil }

// UnmarshalYAML implements yaml.Unmarshaler.
func (t *RopeScalingType) UnmarshalYAML(unmarshal func(any) error) error {
	var value string
	if err := unmarshal(&value); err != nil {
		return err
	}
	return t.UnmarshalText([]byte(value))
}

// ParseRopeScalingType parses a scaling name or numeric alias.
func ParseRopeScalingType(value string) (RopeScalingType, error) {
	name := strings.ToLower(strings.TrimSpace(value))
	switch name {
	case "0", "":
		name = "none"
	case "1":
		name = "linear"
	case "2":
		name = "yarn"
	}
	t, exists := ropeScalingTypes[name]
	if !exists {
		return RopeScalingNone, fmt.Errorf("parse-rope-scaling-type: unknown type: %s (valid: none, linear, yarn)", value)
	}
	return t, nil
}

// MustParseRopeScalingType parses a mode and panics for an unknown name.
func MustParseRopeScalingType(value string) RopeScalingType {
	t, err := ParseRopeScalingType(value)
	if err != nil {
		panic(err)
	}
	return t
}

// ToYZMAType converts to the native scaling mode.
func (t RopeScalingType) ToYZMAType() llama.RopeScalingType {
	switch t {
	case RopeScalingLinear:
		return llama.RopeScalingTypeLinear
	case RopeScalingYaRN:
		return llama.RopeScalingTypeYARN
	default:
		return llama.RopeScalingTypeNone
	}
}

// =============================================================================

// Set of known decision protocols.
var decisionProtocols = make(map[string]DecisionProtocol)

var (
	// DecisionProtocolOpenJEV identifies the OpenJEV letter-readout protocol.
	DecisionProtocolOpenJEV = newDecisionProtocol("openjev")

	// DecisionProtocolJevStyle identifies the macjev-render-v1 protocol.
	DecisionProtocolJevStyle = newDecisionProtocol("jev-style")

	// DecisionProtocolLaya identifies the Laya and Julia marker-embedding protocol.
	DecisionProtocolLaya = newDecisionProtocol("laya")

	// DecisionProtocolLev identifies the Lev label-readout protocol.
	DecisionProtocolLev = newDecisionProtocol("lev")

	// DecisionProtocolKev identifies the Kev pointer-head protocol.
	DecisionProtocolKev = newDecisionProtocol("kev")

	// DecisionProtocolNimble identifies the Nimble label-readout protocol.
	DecisionProtocolNimble = newDecisionProtocol("nimble")

	// DecisionProtocolClef identifies the Clef joint-head protocol.
	DecisionProtocolClef = newDecisionProtocol("clef")

	// DecisionProtocolPplxDecider identifies the Perplexity label-readout protocol.
	DecisionProtocolPplxDecider = newDecisionProtocol("pplx-decider")

	// DecisionProtocolLfm2D1 identifies the LFM2 grouped-label protocol.
	DecisionProtocolLfm2D1 = newDecisionProtocol("lfm2-d1")

	// DecisionProtocolVllmDecision2 identifies the single-question decision-head protocol.
	DecisionProtocolVllmDecision2 = newDecisionProtocol("vllm-decision2")

	// DecisionProtocolVllmD3 identifies the non-causal whole-prompt label protocol.
	DecisionProtocolVllmD3 = newDecisionProtocol("vllm-d3")
)

// DecisionProtocol identifies the prompt and readout protocol used by a
// decision model.
type DecisionProtocol struct {
	value string
}

func newDecisionProtocol(value string) DecisionProtocol {
	protocol := DecisionProtocol{value: value}
	decisionProtocols[value] = protocol
	return protocol
}

// String returns the name of the decision protocol.
func (dp DecisionProtocol) String() string {
	return dp.value
}

// Equal provides support for the go-cmp package and testing.
func (dp DecisionProtocol) Equal(dp2 DecisionProtocol) bool {
	return dp.value == dp2.value
}

// IsZero reports whether the decision protocol is unset.
func (dp DecisionProtocol) IsZero() bool {
	return dp.value == ""
}

// MarshalText provides support for logging and serialization.
func (dp DecisionProtocol) MarshalText() ([]byte, error) {
	return []byte(dp.value), nil
}

// UnmarshalText parses serialized text into a known DecisionProtocol.
func (dp *DecisionProtocol) UnmarshalText(data []byte) error {
	protocol, err := ParseDecisionProtocol(string(data))
	if err != nil {
		return err
	}

	*dp = protocol
	return nil
}

// ParseDecisionProtocol parses value and returns the corresponding
// DecisionProtocol when it exists.
func ParseDecisionProtocol(value string) (DecisionProtocol, error) {
	if value == "" {
		return DecisionProtocol{}, nil
	}

	protocol, exists := decisionProtocols[value]
	if !exists {
		return DecisionProtocol{}, fmt.Errorf("invalid decision protocol %q", value)
	}

	return protocol, nil
}

// MustParseDecisionProtocol parses value and returns the corresponding
// DecisionProtocol. It panics when value does not identify a known protocol.
func MustParseDecisionProtocol(value string) DecisionProtocol {
	protocol, err := ParseDecisionProtocol(value)
	if err != nil {
		panic(err)
	}

	return protocol
}

// =============================================================================

// Set of known MoE modes.
var moeModes = make(map[string]MoEMode)

var (
	// MoEModeAuto uses catalog defaults.
	MoEModeAuto = newMoEMode("auto")

	// MoEModeExpertsCPU places all routed expert tensors on CPU.
	// Recommended for VRAM-constrained setups.
	MoEModeExpertsCPU = newMoEMode("experts_cpu")

	// MoEModeExpertsGPU keeps all expert tensors on GPU.
	// Requires sufficient VRAM for the full model.
	MoEModeExpertsGPU = newMoEMode("experts_gpu")

	// MoEModeKeepTopN keeps routed experts on GPU for the top N layers.
	// All other expert layers go to CPU.
	MoEModeKeepTopN = newMoEMode("keep_top_n")

	// MoEModeCustom defers to TensorBuftOverrides for expert placement.
	MoEModeCustom = newMoEMode("custom")
)

// MoEMode controls expert placement strategy for Mixture of Experts models.
type MoEMode struct {
	value string
}

func newMoEMode(value string) MoEMode {
	mode := MoEMode{value: value}
	moeModes[value] = mode
	return mode
}

// String returns the name of the MoE mode.
func (mm MoEMode) String() string {
	return mm.value
}

// Equal provides support for the go-cmp package and testing.
func (mm MoEMode) Equal(mm2 MoEMode) bool {
	return mm.value == mm2.value
}

// IsZero reports whether the MoE mode is unset.
func (mm MoEMode) IsZero() bool {
	return mm.value == ""
}

// MarshalText provides support for logging and serialization.
func (mm MoEMode) MarshalText() ([]byte, error) {
	return []byte(mm.value), nil
}

// UnmarshalText parses serialized text into a known MoEMode.
func (mm *MoEMode) UnmarshalText(data []byte) error {
	mode, err := ParseMoEMode(string(data))
	if err != nil {
		return err
	}

	*mm = mode
	return nil
}

// ParseMoEMode parses value and returns the corresponding MoEMode when it
// exists.
func ParseMoEMode(value string) (MoEMode, error) {
	mode, exists := moeModes[value]
	if !exists {
		return MoEMode{}, fmt.Errorf("invalid MoE mode %q", value)
	}

	return mode, nil
}

// MustParseMoEMode parses value and returns the corresponding MoEMode. It
// panics when value does not identify a known MoE mode.
func MustParseMoEMode(value string) MoEMode {
	mode, err := ParseMoEMode(value)
	if err != nil {
		panic(err)
	}

	return mode
}

// =============================================================================

// SpeculationMode selects the speculative-decoding implementation for a model.
type SpeculationMode = internalspec.Mode

var (
	// SpeculationAuto selects a configured classic drafter or available MTP.
	SpeculationAuto = internalspec.ModeAuto
	// SpeculationDisabled runs target-only generation.
	SpeculationDisabled = internalspec.ModeDisabled
	// SpeculationClassic requires a separate draft model.
	SpeculationClassic = internalspec.ModeClassic
	// SpeculationMTP requires a companion or embedded MTP implementation.
	SpeculationMTP = internalspec.ModeMTP
)

// ParseSpeculationMode parses a known speculation mode or an unset value.
func ParseSpeculationMode(value string) (SpeculationMode, error) {
	return internalspec.ParseMode(value)
}

// MustParseSpeculationMode parses a mode and panics for an unknown name.
func MustParseSpeculationMode(value string) SpeculationMode {
	return internalspec.MustParseMode(value)
}
