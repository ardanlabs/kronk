package modelprofile

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Class describes the execution-state shape of a model. Its zero value is unset.
type Class struct{ value string }

var classes = make(map[string]Class)

var (
	// ClassDense identifies dense models.
	ClassDense = newClass("dense")
	// ClassMoE identifies mixture-of-experts models.
	ClassMoE = newClass("moe")
	// ClassHybrid identifies hybrid recurrent models.
	ClassHybrid = newClass("hybrid")
)

func newClass(value string) Class {
	t := Class{value: value}
	classes[value] = t
	return t
}

// String returns the wire value.
func (t Class) String() string { return t.value }

// Equal reports whether the values match.
func (t Class) Equal(other Class) bool { return t == other }

// IsZero reports whether the value is unset.
func (t Class) IsZero() bool { return t.value == "" }

// MarshalText implements encoding.TextMarshaler.
func (t Class) MarshalText() ([]byte, error) { return []byte(t.value), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (t *Class) UnmarshalText(data []byte) error {
	v, err := ParseClass(string(data))
	if err != nil {
		return err
	}
	*t = v
	return nil
}

// ParseClass parses a registered wire value or the unset value.
func ParseClass(value string) (Class, error) {
	if value == "" {
		return Class{}, nil
	}
	t, ok := classes[value]
	if !ok {
		return Class{}, fmt.Errorf("invalid class %q", value)
	}
	return t, nil
}

// MustParseClass parses a value and panics if it is invalid.
func MustParseClass(value string) Class {
	t, err := ParseClass(value)
	if err != nil {
		panic(err)
	}
	return t
}

// =============================================================================

// Role describes an artifact's runtime role. Its zero value is unset.
type Role struct{ value string }

var roles = make(map[string]Role)

var (
	// RoleLanguage identifies a language model.
	RoleLanguage = newRole("language")
	// RoleVisionEncoder identifies a projection artifact.
	RoleVisionEncoder = newRole("vision_encoder")
)

func newRole(value string) Role {
	t := Role{value: value}
	roles[value] = t
	return t
}

// String returns the wire value.
func (t Role) String() string { return t.value }

// Equal reports whether the values match.
func (t Role) Equal(other Role) bool { return t == other }

// IsZero reports whether the value is unset.
func (t Role) IsZero() bool { return t.value == "" }

// MarshalText implements encoding.TextMarshaler.
func (t Role) MarshalText() ([]byte, error) { return []byte(t.value), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (t *Role) UnmarshalText(data []byte) error {
	v, err := ParseRole(string(data))
	if err != nil {
		return err
	}
	*t = v
	return nil
}

// ParseRole parses a registered wire value or the unset value.
func ParseRole(value string) (Role, error) {
	if value == "" {
		return Role{}, nil
	}
	t, ok := roles[value]
	if !ok {
		return Role{}, fmt.Errorf("invalid role %q", value)
	}
	return t, nil
}

// MustParseRole parses a value and panics if it is invalid.
func MustParseRole(value string) Role {
	t, err := ParseRole(value)
	if err != nil {
		panic(err)
	}
	return t
}

// =============================================================================

// Purpose describes the endpoint a model serves. Its zero value is unset.
type Purpose struct{ value string }

var purposes = make(map[string]Purpose)

var (
	// PurposeGeneration identifies text generation.
	PurposeGeneration = newPurpose("generation")
	// PurposeEmbedding identifies embedding generation.
	PurposeEmbedding = newPurpose("embedding")
	// PurposeRerank identifies reranking.
	PurposeRerank = newPurpose("rerank")
)

func newPurpose(value string) Purpose {
	t := Purpose{value: value}
	purposes[value] = t
	return t
}

// String returns the wire value.
func (t Purpose) String() string { return t.value }

// Equal reports whether the values match.
func (t Purpose) Equal(other Purpose) bool { return t == other }

// IsZero reports whether the value is unset.
func (t Purpose) IsZero() bool { return t.value == "" }

// MarshalText implements encoding.TextMarshaler.
func (t Purpose) MarshalText() ([]byte, error) { return []byte(t.value), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (t *Purpose) UnmarshalText(data []byte) error {
	v, err := ParsePurpose(string(data))
	if err != nil {
		return err
	}
	*t = v
	return nil
}

// ParsePurpose parses a registered wire value or the unset value.
func ParsePurpose(value string) (Purpose, error) {
	if value == "" {
		return Purpose{}, nil
	}
	t, ok := purposes[value]
	if !ok {
		return Purpose{}, fmt.Errorf("invalid purpose %q", value)
	}
	return t, nil
}

// MustParsePurpose parses a value and panics if it is invalid.
func MustParsePurpose(value string) Purpose {
	t, err := ParsePurpose(value)
	if err != nil {
		panic(err)
	}
	return t
}

// =============================================================================

// MemorySemantics describes sequence state management. Its zero value is unset.
type MemorySemantics struct{ value string }

var memorySemantics = make(map[string]MemorySemantics)

var (
	// MemoryAttention identifies attention cache state.
	MemoryAttention = newMemorySemantics("attention")
	// MemoryRecurrent identifies recurrent state.
	MemoryRecurrent = newMemorySemantics("recurrent")
)

func newMemorySemantics(value string) MemorySemantics {
	t := MemorySemantics{value: value}
	memorySemantics[value] = t
	return t
}

// String returns the wire value.
func (t MemorySemantics) String() string { return t.value }

// Equal reports whether the values match.
func (t MemorySemantics) Equal(other MemorySemantics) bool { return t == other }

// IsZero reports whether the value is unset.
func (t MemorySemantics) IsZero() bool { return t.value == "" }

// MarshalText implements encoding.TextMarshaler.
func (t MemorySemantics) MarshalText() ([]byte, error) { return []byte(t.value), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (t *MemorySemantics) UnmarshalText(data []byte) error {
	v, err := ParseMemorySemantics(string(data))
	if err != nil {
		return err
	}
	*t = v
	return nil
}

// ParseMemorySemantics parses a registered wire value or the unset value.
func ParseMemorySemantics(value string) (MemorySemantics, error) {
	if value == "" {
		return MemorySemantics{}, nil
	}
	t, ok := memorySemantics[value]
	if !ok {
		return MemorySemantics{}, fmt.Errorf("invalid memory semantics %q", value)
	}
	return t, nil
}

// MustParseMemorySemantics parses a value and panics if it is invalid.
func MustParseMemorySemantics(value string) MemorySemantics {
	t, err := ParseMemorySemantics(value)
	if err != nil {
		panic(err)
	}
	return t
}

// =============================================================================

// MTPArchitecture identifies an MTP runtime contract. Its zero value is none.
type MTPArchitecture struct{ value string }

var mtpArchitectures = make(map[string]MTPArchitecture)

var (
	// MTPArchitectureNone identifies an absent MTP head.
	MTPArchitectureNone = newMTPArchitecture("")
	// MTPArchitectureQwen35OwnKV identifies Qwen3.5 own-KV MTP.
	MTPArchitectureQwen35OwnKV = newMTPArchitecture("qwen35-own-kv")
	// MTPArchitectureQwen4ExpOwnKV identifies Qwen4Exp own-KV MTP.
	MTPArchitectureQwen4ExpOwnKV = newMTPArchitecture("qwen4exp-own-kv")
	// MTPArchitectureNemotronOwnKV identifies Nemotron own-KV MTP.
	MTPArchitectureNemotronOwnKV = newMTPArchitecture("nemotron-own-kv")
	// MTPArchitectureGemmaSharedKV identifies Gemma shared-KV MTP.
	MTPArchitectureGemmaSharedKV = newMTPArchitecture("gemma-shared-kv")
	// MTPArchitectureGLM5Next identifies GLM5 Next MTP.
	MTPArchitectureGLM5Next = newMTPArchitecture("glm5-next")
)

func newMTPArchitecture(value string) MTPArchitecture {
	t := MTPArchitecture{value: value}
	mtpArchitectures[value] = t
	return t
}

// String returns the wire value.
func (t MTPArchitecture) String() string { return t.value }

// Equal reports whether the values match.
func (t MTPArchitecture) Equal(other MTPArchitecture) bool { return t == other }

// IsZero reports whether no MTP architecture is set.
func (t MTPArchitecture) IsZero() bool { return t.value == "" }

// MarshalText implements encoding.TextMarshaler.
func (t MTPArchitecture) MarshalText() ([]byte, error) { return []byte(t.value), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (t *MTPArchitecture) UnmarshalText(data []byte) error {
	v, err := ParseMTPArchitecture(string(data))
	if err != nil {
		return err
	}
	*t = v
	return nil
}

// ParseMTPArchitecture parses a registered wire value.
func ParseMTPArchitecture(value string) (MTPArchitecture, error) {
	t, ok := mtpArchitectures[value]
	if !ok {
		return MTPArchitecture{}, fmt.Errorf("invalid MTP architecture %q", value)
	}
	return t, nil
}

// MustParseMTPArchitecture parses a value and panics if it is invalid.
func MustParseMTPArchitecture(value string) MTPArchitecture {
	t, err := ParseMTPArchitecture(value)
	if err != nil {
		panic(err)
	}
	return t
}

// MTPEnabled reports whether Kronk enables an identified MTP runtime.
func MTPEnabled(architecture MTPArchitecture) bool {
	switch architecture {
	case MTPArchitectureQwen35OwnKV, MTPArchitectureQwen4ExpOwnKV, MTPArchitectureNemotronOwnKV, MTPArchitectureGemmaSharedKV:
		return true
	default:
		return false
	}
}

// =============================================================================

// ComputeKind identifies the workspace allocation family. Its zero value is generic.
type ComputeKind struct{ value uint8 }

var computeKinds = make(map[uint8]ComputeKind)

var (
	// ComputeGeneric uses the generic allowance for unvalidated architectures.
	ComputeGeneric = newComputeKind(0)
	// ComputeIndexed identifies Qwen4Exp indexed-attention workspaces.
	ComputeIndexed = newComputeKind(1)
	// ComputeGemma4 identifies Gemma4 target and shared-KV assistant workspaces.
	ComputeGemma4 = newComputeKind(2)
	// ComputeQwen35 identifies Qwen3.5 target and own-KV MTP workspaces.
	ComputeQwen35 = newComputeKind(3)
	// ComputeGPTOSS identifies GPT-OSS alternating full/SWA workspaces.
	ComputeGPTOSS = newComputeKind(4)
	// ComputeMistral3 identifies Mistral3 dense full-attention workspaces.
	ComputeMistral3 = newComputeKind(5)
	// ComputeLFM2 identifies LFM2 attention and short-convolution workspaces.
	ComputeLFM2 = newComputeKind(6)
	// ComputeGLM4MoE identifies GLM4 MoE attention and routed FFN workspaces.
	ComputeGLM4MoE = newComputeKind(7)
	// ComputeGemma3 identifies Gemma3 non-SWA attention workspaces.
	ComputeGemma3 = newComputeKind(8)
	// ComputeLlama identifies Llama full-attention GQA workspaces.
	ComputeLlama = newComputeKind(9)
	// ComputeClef identifies Clef batch-local decision workspaces.
	ComputeClef = newComputeKind(10)
	// ComputeModernBERT identifies ModernBERT batch-local encoder workspaces.
	ComputeModernBERT = newComputeKind(11)
)

func newComputeKind(value uint8) ComputeKind {
	t := ComputeKind{value: value}
	computeKinds[value] = t
	return t
}

// Value returns the numeric wire and runtime allocation-family ID.
func (t ComputeKind) Value() uint8 { return t.value }

// String returns the decimal allocation-family ID.
func (t ComputeKind) String() string { return strconv.FormatUint(uint64(t.value), 10) }

// Equal reports whether the values match.
func (t ComputeKind) Equal(other ComputeKind) bool { return t == other }

// IsZero reports whether the generic allowance is selected.
func (t ComputeKind) IsZero() bool { return t.value == 0 }

// ParseComputeKind parses a registered numeric allocation-family ID.
func ParseComputeKind(value uint8) (ComputeKind, error) {
	t, ok := computeKinds[value]
	if !ok {
		return ComputeKind{}, fmt.Errorf("invalid compute kind %d", value)
	}
	return t, nil
}

// MustParseComputeKind parses an ID and panics if it is invalid.
func MustParseComputeKind(value uint8) ComputeKind {
	t, err := ParseComputeKind(value)
	if err != nil {
		panic(err)
	}
	return t
}

// MarshalJSON preserves the numeric JSON representation.
func (t ComputeKind) MarshalJSON() ([]byte, error) { return json.Marshal(t.value) }

// UnmarshalJSON parses a registered numeric JSON value.
func (t *ComputeKind) UnmarshalJSON(data []byte) error {
	value := t.value
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	v, err := ParseComputeKind(value)
	if err != nil {
		return err
	}
	*t = v
	return nil
}

// MarshalYAML preserves the numeric YAML representation.
func (t ComputeKind) MarshalYAML() (any, error) { return t.value, nil }

// UnmarshalYAML parses a registered numeric YAML value.
func (t *ComputeKind) UnmarshalYAML(unmarshal func(any) error) error {
	value := t.value
	if err := unmarshal(&value); err != nil {
		return err
	}
	v, err := ParseComputeKind(value)
	if err != nil {
		return err
	}
	*t = v
	return nil
}
