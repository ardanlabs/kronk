package models

import "fmt"

// IntegrityStatus describes the available verification evidence for a model or artifact.
type IntegrityStatus struct{ value string }

var integrityStatuses = make(map[string]IntegrityStatus)

var (
	// IntegrityVerified means the published digest and persisted verification match current metadata.
	IntegrityVerified = newIntegrityStatus("verified")
	// IntegrityUnverified means a published digest exists but no persisted verification exists.
	IntegrityUnverified = newIntegrityStatus("unverified")
	// IntegrityStale means verification evidence or current metadata no longer matches.
	IntegrityStale = newIntegrityStatus("stale")
	// IntegrityUnavailable means no trustworthy published digest is available.
	IntegrityUnavailable = newIntegrityStatus("unavailable")
)

func newIntegrityStatus(value string) IntegrityStatus {
	t := IntegrityStatus{value: value}
	integrityStatuses[value] = t
	return t
}

// String returns the canonical name.
func (t IntegrityStatus) String() string { return t.value }

// Equal provides support for comparison and testing.
func (t IntegrityStatus) Equal(other IntegrityStatus) bool { return t.value == other.value }

// IsZero reports whether the value is unset.
func (t IntegrityStatus) IsZero() bool { return t.value == "" }

// MarshalText provides support for logging and serialization.
func (t IntegrityStatus) MarshalText() ([]byte, error) { return []byte(t.value), nil }

// UnmarshalText parses a registered value.
func (t *IntegrityStatus) UnmarshalText(data []byte) error {
	parsed, err := ParseIntegrityStatus(string(data))
	if err != nil {
		return err
	}
	*t = parsed
	return nil
}

// ParseIntegrityStatus parses a registered name or the unset value.
func ParseIntegrityStatus(value string) (IntegrityStatus, error) {
	if value == "" {
		return IntegrityStatus{}, nil
	}
	t, exists := integrityStatuses[value]
	if !exists {
		return IntegrityStatus{}, fmt.Errorf("invalid integrity status %q", value)
	}
	return t, nil
}

// MustParseIntegrityStatus parses a registered name and panics for an unknown name.
func MustParseIntegrityStatus(value string) IntegrityStatus {
	t, err := ParseIntegrityStatus(value)
	if err != nil {
		panic(err)
	}
	return t
}

// IntegrityRole identifies a model artifact's role.
type IntegrityRole struct{ value string }

var integrityRoles = make(map[string]IntegrityRole)

var (
	// IntegrityRoleWeights identifies a model weight file or shard.
	IntegrityRoleWeights = newIntegrityRole("weights")
	// IntegrityRoleProjection identifies a multimodal projection artifact.
	IntegrityRoleProjection = newIntegrityRole("projection")
	// IntegrityRoleMTP identifies an MTP drafter artifact.
	IntegrityRoleMTP = newIntegrityRole("mtp")
)

func newIntegrityRole(value string) IntegrityRole {
	t := IntegrityRole{value: value}
	integrityRoles[value] = t
	return t
}

// String returns the canonical name.
func (t IntegrityRole) String() string { return t.value }

// Equal provides support for comparison and testing.
func (t IntegrityRole) Equal(other IntegrityRole) bool { return t.value == other.value }

// IsZero reports whether the value is unset.
func (t IntegrityRole) IsZero() bool { return t.value == "" }

// MarshalText provides support for logging and serialization.
func (t IntegrityRole) MarshalText() ([]byte, error) { return []byte(t.value), nil }

// UnmarshalText parses a registered value.
func (t *IntegrityRole) UnmarshalText(data []byte) error {
	parsed, err := ParseIntegrityRole(string(data))
	if err != nil {
		return err
	}
	*t = parsed
	return nil
}

// ParseIntegrityRole parses a registered name or the unset value.
func ParseIntegrityRole(value string) (IntegrityRole, error) {
	if value == "" {
		return IntegrityRole{}, nil
	}
	t, exists := integrityRoles[value]
	if !exists {
		return IntegrityRole{}, fmt.Errorf("invalid integrity role %q", value)
	}
	return t, nil
}

// MustParseIntegrityRole parses a registered name and panics for an unknown name.
func MustParseIntegrityRole(value string) IntegrityRole {
	t, err := ParseIntegrityRole(value)
	if err != nil {
		panic(err)
	}
	return t
}

// IntegrityReason identifies why verification evidence is unavailable or stale.
type IntegrityReason struct{ value string }

var integrityReasons = make(map[string]IntegrityReason)

var (
	// IntegrityReasonDigestUnavailable means the Git LFS pointer is absent or malformed.
	IntegrityReasonDigestUnavailable = newIntegrityReason("digest_unavailable")
	// IntegrityReasonFileMetadataChanged means verification evidence does not match the digest or metadata.
	IntegrityReasonFileMetadataChanged = newIntegrityReason("file_metadata_changed")
)

func newIntegrityReason(value string) IntegrityReason {
	t := IntegrityReason{value: value}
	integrityReasons[value] = t
	return t
}

// String returns the canonical name.
func (t IntegrityReason) String() string { return t.value }

// Equal provides support for comparison and testing.
func (t IntegrityReason) Equal(other IntegrityReason) bool { return t.value == other.value }

// IsZero reports whether the value is unset.
func (t IntegrityReason) IsZero() bool { return t.value == "" }

// MarshalText provides support for logging and serialization.
func (t IntegrityReason) MarshalText() ([]byte, error) { return []byte(t.value), nil }

// UnmarshalText parses a registered value.
func (t *IntegrityReason) UnmarshalText(data []byte) error {
	parsed, err := ParseIntegrityReason(string(data))
	if err != nil {
		return err
	}
	*t = parsed
	return nil
}

// ParseIntegrityReason parses a registered name or the unset value.
func ParseIntegrityReason(value string) (IntegrityReason, error) {
	if value == "" {
		return IntegrityReason{}, nil
	}
	t, exists := integrityReasons[value]
	if !exists {
		return IntegrityReason{}, fmt.Errorf("invalid integrity reason %q", value)
	}
	return t, nil
}

// MustParseIntegrityReason parses a registered name and panics for an unknown name.
func MustParseIntegrityReason(value string) IntegrityReason {
	t, err := ParseIntegrityReason(value)
	if err != nil {
		panic(err)
	}
	return t
}
