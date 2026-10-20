package speculation

import "fmt"

// Mode selects the speculative-decoding implementation for a model.
type Mode struct {
	value string
}

var modes = make(map[string]Mode)

var (
	// ModeAuto selects a classic drafter when explicitly configured, otherwise
	// a companion or embedded MTP implementation when available.
	ModeAuto = newMode("auto")

	// ModeDisabled runs target-only generation.
	ModeDisabled = newMode("disabled")

	// ModeClassic requires an explicitly configured separate draft model.
	ModeClassic = newMode("classic")

	// ModeMTP requires a companion or embedded MTP implementation.
	ModeMTP = newMode("mtp")
)

func newMode(value string) Mode {
	mode := Mode{value: value}
	modes[value] = mode
	return mode
}

// String returns the name of the speculation mode.
func (m Mode) String() string { return m.value }

// Equal provides support for comparison and testing.
func (m Mode) Equal(other Mode) bool { return m.value == other.value }

// IsZero reports whether the mode is unset.
func (m Mode) IsZero() bool { return m.value == "" }

// MarshalText provides support for logging and serialization.
func (m Mode) MarshalText() ([]byte, error) { return []byte(m.value), nil }

// UnmarshalText parses a known speculation mode.
func (m *Mode) UnmarshalText(data []byte) error {
	mode, err := ParseMode(string(data))
	if err != nil {
		return err
	}
	*m = mode
	return nil
}

// ParseMode parses a known speculation mode or an unset value.
func ParseMode(value string) (Mode, error) {
	if value == "" {
		return Mode{}, nil
	}
	mode, exists := modes[value]
	if !exists {
		return Mode{}, fmt.Errorf("invalid speculation mode %q", value)
	}
	return mode, nil
}

// MustParseMode parses a mode and panics for an unknown name.
func MustParseMode(value string) Mode {
	mode, err := ParseMode(value)
	if err != nil {
		panic(err)
	}
	return mode
}

// Source identifies the broad speculation engine selected for a model.
type Source struct{ value uint8 }

var (
	// SourceNone selects target-only generation.
	SourceNone = newSource("none", 0)

	// SourceClassic selects a separate-GGUF drafter.
	SourceClassic = newSource("classic", 1)

	// SourceMTP selects an architecture-specific MTP backend.
	SourceMTP = newSource("mtp", 2)
)

// MTPArchitecture identifies the model-family runtime contract. It is kept
// separate from MTPArtifact because embedded and companion Qwen35 heads use
// the same own-KV synchronization and rollback behavior.
type MTPArchitecture struct{ value uint8 }

var (
	// MTPArchitectureNone identifies no MTP runtime.
	MTPArchitectureNone = newMTPArchitecture("none", 0)
	// MTPArchitectureQwen35OwnKV identifies Qwen35 own-KV synchronization.
	MTPArchitectureQwen35OwnKV = newMTPArchitecture("qwen35-own-kv", 1)
	// MTPArchitectureQwen4ExpOwnKV identifies Qwen4Exp own-KV synchronization.
	MTPArchitectureQwen4ExpOwnKV = newMTPArchitecture("qwen4exp-own-kv", 2)
	// MTPArchitectureNemotronOwnKV identifies Nemotron own-KV synchronization.
	MTPArchitectureNemotronOwnKV = newMTPArchitecture("nemotron-own-kv", 3)
	// MTPArchitectureGemmaSharedKV identifies Gemma shared-KV synchronization.
	MTPArchitectureGemmaSharedKV = newMTPArchitecture("gemma-shared-kv", 4)
	// MTPArchitectureGLM5Next identifies GLM5 NextN synchronization.
	MTPArchitectureGLM5Next = newMTPArchitecture("glm5-next", 5)
)

// String returns the architecture name.
func (ma MTPArchitecture) String() string {
	switch ma {
	case MTPArchitectureQwen35OwnKV:
		return "qwen35-own-kv"
	case MTPArchitectureQwen4ExpOwnKV:
		return "qwen4exp-own-kv"
	case MTPArchitectureNemotronOwnKV:
		return "nemotron-own-kv"
	case MTPArchitectureGemmaSharedKV:
		return "gemma-shared-kv"
	case MTPArchitectureGLM5Next:
		return "glm5-next"
	default:
		return "none"
	}
}

// MTPArtifact identifies where an MTP head's weights are stored.
type MTPArtifact struct{ value uint8 }

var (
	// MTPArtifactNone identifies no MTP weights.
	MTPArtifactNone = newMTPArtifact("none", 0)
	// MTPArtifactEmbedded identifies weights in the target model.
	MTPArtifactEmbedded = newMTPArtifact("embedded", 1)
	// MTPArtifactCompanion identifies separate-file weights.
	MTPArtifactCompanion = newMTPArtifact("companion", 2)
)

var sources = make(map[string]Source)
var mtpArchitectures = make(map[string]MTPArchitecture)
var mtpArtifacts = make(map[string]MTPArtifact)

func newSource(name string, value uint8) Source {
	t := Source{value: value}
	sources[name] = t
	return t
}
func newMTPArchitecture(name string, value uint8) MTPArchitecture {
	t := MTPArchitecture{value: value}
	mtpArchitectures[name] = t
	return t
}
func newMTPArtifact(name string, value uint8) MTPArtifact {
	t := MTPArtifact{value: value}
	mtpArtifacts[name] = t
	return t
}

// String returns the source name.
func (s Source) String() string {
	switch s {
	case SourceClassic:
		return "classic"
	case SourceMTP:
		return "mtp"
	default:
		return "none"
	}
}

// Equal reports whether the values match.
func (s Source) Equal(other Source) bool { return s == other }

// IsZero reports whether the source is none.
func (s Source) IsZero() bool { return s.value == 0 }

// MarshalText serializes the source name.
func (s Source) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// UnmarshalText parses a registered source.
func (s *Source) UnmarshalText(data []byte) error {
	v, err := ParseSource(string(data))
	if err != nil {
		return err
	}
	*s = v
	return nil
}

// ParseSource parses a registered source name.
func ParseSource(value string) (Source, error) {
	t, ok := sources[value]
	if !ok {
		return Source{}, fmt.Errorf("invalid speculation source %q", value)
	}
	return t, nil
}

// MustParseSource parses a source and panics if it is invalid.
func MustParseSource(value string) Source {
	t, err := ParseSource(value)
	if err != nil {
		panic(err)
	}
	return t
}

// Equal reports whether the values match.
func (ma MTPArchitecture) Equal(other MTPArchitecture) bool { return ma == other }

// IsZero reports whether the architecture is none.
func (ma MTPArchitecture) IsZero() bool { return ma.value == 0 }

// MarshalText serializes the architecture name.
func (ma MTPArchitecture) MarshalText() ([]byte, error) { return []byte(ma.String()), nil }

// UnmarshalText parses a registered architecture.
func (ma *MTPArchitecture) UnmarshalText(data []byte) error {
	v, err := ParseMTPArchitecture(string(data))
	if err != nil {
		return err
	}
	*ma = v
	return nil
}

// ParseMTPArchitecture parses a registered architecture name.
func ParseMTPArchitecture(value string) (MTPArchitecture, error) {
	t, ok := mtpArchitectures[value]
	if !ok {
		return MTPArchitecture{}, fmt.Errorf("invalid MTP architecture %q", value)
	}
	return t, nil
}

// MustParseMTPArchitecture parses an architecture and panics if it is invalid.
func MustParseMTPArchitecture(value string) MTPArchitecture {
	t, err := ParseMTPArchitecture(value)
	if err != nil {
		panic(err)
	}
	return t
}

// String returns the artifact name.
func (ma MTPArtifact) String() string {
	switch ma {
	case MTPArtifactEmbedded:
		return "embedded"
	case MTPArtifactCompanion:
		return "companion"
	default:
		return "none"
	}
}

// Equal reports whether the values match.
func (ma MTPArtifact) Equal(other MTPArtifact) bool { return ma == other }

// IsZero reports whether the artifact is none.
func (ma MTPArtifact) IsZero() bool { return ma.value == 0 }

// MarshalText serializes the artifact name.
func (ma MTPArtifact) MarshalText() ([]byte, error) { return []byte(ma.String()), nil }

// UnmarshalText parses a registered artifact.
func (ma *MTPArtifact) UnmarshalText(data []byte) error {
	v, err := ParseMTPArtifact(string(data))
	if err != nil {
		return err
	}
	*ma = v
	return nil
}

// ParseMTPArtifact parses a registered artifact name.
func ParseMTPArtifact(value string) (MTPArtifact, error) {
	t, ok := mtpArtifacts[value]
	if !ok {
		return MTPArtifact{}, fmt.Errorf("invalid MTP artifact %q", value)
	}
	return t, nil
}

// MustParseMTPArtifact parses an artifact and panics if it is invalid.
func MustParseMTPArtifact(value string) MTPArtifact {
	t, err := ParseMTPArtifact(value)
	if err != nil {
		panic(err)
	}
	return t
}

// Config contains the capabilities needed to resolve one speculation plan.
type Config struct {
	Mode                     Mode
	ClassicConfigured        bool
	ClassicNDraft            int
	MTPNDraft                int
	EmbeddedMTPArchitecture  MTPArchitecture
	CompanionMTPArchitecture MTPArchitecture
	MTPAvailable             bool
	MTPArchitectureEnabled   bool
}

// Plan is the immutable decision shared by model loading, context sizing, and
// runtime controller construction.
type Plan struct {
	Mode            Mode
	Source          Source
	MTPArchitecture MTPArchitecture
	MTPArtifact     MTPArtifact
	NDraft          int
	LoadMTP         bool
	Available       bool
}
