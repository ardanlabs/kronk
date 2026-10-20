package modelprofile

import (
	"encoding/json"
	"fmt"
	"strconv"
)

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
func (r Role) String() string { return r.value }

// Equal reports whether the values match.
func (r Role) Equal(other Role) bool { return r == other }

// IsZero reports whether the value is unset.
func (r Role) IsZero() bool { return r.value == "" }

// MarshalText implements encoding.TextMarshaler.
func (r Role) MarshalText() ([]byte, error) { return []byte(r.value), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (r *Role) UnmarshalText(data []byte) error {
	v, err := ParseRole(string(data))
	if err != nil {
		return err
	}
	*r = v
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
func (p Purpose) String() string { return p.value }

// Equal reports whether the values match.
func (p Purpose) Equal(other Purpose) bool { return p == other }

// IsZero reports whether the value is unset.
func (p Purpose) IsZero() bool { return p.value == "" }

// MarshalText implements encoding.TextMarshaler.
func (p Purpose) MarshalText() ([]byte, error) { return []byte(p.value), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (p *Purpose) UnmarshalText(data []byte) error {
	v, err := ParsePurpose(string(data))
	if err != nil {
		return err
	}
	*p = v
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
func (ms MemorySemantics) String() string { return ms.value }

// Equal reports whether the values match.
func (ms MemorySemantics) Equal(other MemorySemantics) bool { return ms == other }

// IsZero reports whether the value is unset.
func (ms MemorySemantics) IsZero() bool { return ms.value == "" }

// MarshalText implements encoding.TextMarshaler.
func (ms MemorySemantics) MarshalText() ([]byte, error) { return []byte(ms.value), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (ms *MemorySemantics) UnmarshalText(data []byte) error {
	v, err := ParseMemorySemantics(string(data))
	if err != nil {
		return err
	}
	*ms = v
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
func (ma MTPArchitecture) String() string { return ma.value }

// Equal reports whether the values match.
func (ma MTPArchitecture) Equal(other MTPArchitecture) bool { return ma == other }

// IsZero reports whether no MTP architecture is set.
func (ma MTPArchitecture) IsZero() bool { return ma.value == "" }

// MarshalText implements encoding.TextMarshaler.
func (ma MTPArchitecture) MarshalText() ([]byte, error) { return []byte(ma.value), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (ma *MTPArchitecture) UnmarshalText(data []byte) error {
	v, err := ParseMTPArchitecture(string(data))
	if err != nil {
		return err
	}
	*ma = v
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
func (ck ComputeKind) Value() uint8 { return ck.value }

// String returns the decimal allocation-family ID.
func (ck ComputeKind) String() string { return strconv.FormatUint(uint64(ck.value), 10) }

// Equal reports whether the values match.
func (ck ComputeKind) Equal(other ComputeKind) bool { return ck == other }

// IsZero reports whether the generic allowance is selected.
func (ck ComputeKind) IsZero() bool { return ck.value == 0 }

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
func (ck ComputeKind) MarshalJSON() ([]byte, error) { return json.Marshal(ck.value) }

// UnmarshalJSON parses a registered numeric JSON value.
func (ck *ComputeKind) UnmarshalJSON(data []byte) error {
	value := ck.value
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	v, err := ParseComputeKind(value)
	if err != nil {
		return err
	}
	*ck = v
	return nil
}

// MarshalYAML preserves the numeric YAML representation.
func (ck ComputeKind) MarshalYAML() (any, error) { return ck.value, nil }

// UnmarshalYAML parses a registered numeric YAML value.
func (ck *ComputeKind) UnmarshalYAML(unmarshal func(any) error) error {
	value := ck.value
	if err := unmarshal(&value); err != nil {
		return err
	}
	v, err := ParseComputeKind(value)
	if err != nil {
		return err
	}
	*ck = v
	return nil
}
