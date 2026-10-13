package models

import "fmt"

// FileRole identifies a model component's configuration role.
type FileRole struct{ value string }

var fileRoles = make(map[string]FileRole)

var (
	// RoleModel identifies model.
	RoleModel = newFileRole("model")
	// RoleDiffusion identifies diffusion.
	RoleDiffusion = newFileRole("diffusion")
	// RoleVAE identifies vae.
	RoleVAE = newFileRole("vae")
	// RoleClipL identifies clip_l.
	RoleClipL = newFileRole("clip_l")
	// RoleClipG identifies clip_g.
	RoleClipG = newFileRole("clip_g")
	// RoleT5XXL identifies t5xxl.
	RoleT5XXL = newFileRole("t5xxl")
	// RoleLLM identifies llm.
	RoleLLM = newFileRole("llm")
	// RoleLLMVision identifies llm_vision.
	RoleLLMVision = newFileRole("llm_vision")
	// RoleTokenizer identifies tokenizer.
	RoleTokenizer = newFileRole("tokenizer")
	// RoleControlNet identifies control_net.
	RoleControlNet = newFileRole("control_net")
	// RoleTAESD identifies taesd.
	RoleTAESD = newFileRole("taesd")
	// RolePhotoMaker identifies photo_maker.
	RolePhotoMaker = newFileRole("photo_maker")
	// RoleClipVision identifies clip_vision.
	RoleClipVision = newFileRole("clip_vision")
	// RoleHighNoise identifies high_noise.
	RoleHighNoise = newFileRole("high_noise")
	// RoleEmbeddingsConn identifies embeddings_conn.
	RoleEmbeddingsConn = newFileRole("embeddings_conn")
	// RoleMotionModule identifies motion_module.
	RoleMotionModule = newFileRole("motion_module")
	// RoleUpscaler identifies upscaler.
	RoleUpscaler = newFileRole("upscaler")
	// RoleADetailer identifies adetailer.
	RoleADetailer = newFileRole("adetailer")
	// RoleAudioEncoder identifies audio_encoder.
	RoleAudioEncoder = newFileRole("audio_encoder")
)

func newFileRole(value string) FileRole {
	t := FileRole{value: value}
	fileRoles[value] = t
	return t
}

// String returns the canonical name.
func (t FileRole) String() string { return t.value }

// Equal provides support for comparison and testing.
func (t FileRole) Equal(other FileRole) bool { return t.value == other.value }

// IsZero reports whether the value is unset.
func (t FileRole) IsZero() bool { return t.value == "" }

// MarshalText provides support for logging and serialization.
func (t FileRole) MarshalText() ([]byte, error) { return []byte(t.value), nil }

// UnmarshalText parses a registered value.
func (t *FileRole) UnmarshalText(data []byte) error {
	parsed, err := ParseFileRole(string(data))
	if err != nil {
		return err
	}
	*t = parsed
	return nil
}

// ParseFileRole parses a registered name or the unset value.
func ParseFileRole(value string) (FileRole, error) {
	if value == "" {
		return FileRole{}, nil
	}
	t, exists := fileRoles[value]
	if !exists {
		return FileRole{}, fmt.Errorf("invalid file role %q", value)
	}
	return t, nil
}

// MustParseFileRole parses a registered name and panics for an unknown name.
func MustParseFileRole(value string) FileRole {
	t, err := ParseFileRole(value)
	if err != nil {
		panic(err)
	}
	return t
}
