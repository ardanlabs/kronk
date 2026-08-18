package models

import (
	"cmp"
	"fmt"
	"slices"
)

// Set of known curated bundle names.
var bundleNames = make(map[string]BundleName)

// The curated model bundles supported by Kronk's Malina SDK.
var (
	// BundleSD15 identifies the Stable Diffusion 1.5 bundle.
	BundleSD15 = newBundleName("sd-1.5")

	// BundleSDXLBase10 identifies the Stable Diffusion XL base 1.0 bundle.
	BundleSDXLBase10 = newBundleName("sdxl-base-1.0")

	// BundleFlux2Klein4B identifies the FLUX.2 Klein 4B bundle.
	BundleFlux2Klein4B = newBundleName("flux2-klein-4b")

	// BundleFlux2Klein9B identifies the FLUX.2 Klein 9B bundle.
	BundleFlux2Klein9B = newBundleName("flux2-klein-9b")
)

// BundleName identifies a curated model bundle.
type BundleName struct {
	value string
}

func newBundleName(value string) BundleName {
	name := BundleName{value: value}
	bundleNames[value] = name
	return name
}

// String returns the bundle name.
func (bn BundleName) String() string {
	return bn.value
}

// Equal provides support for the go-cmp package and testing.
func (bn BundleName) Equal(bn2 BundleName) bool {
	return bn.value == bn2.value
}

// IsZero reports whether the bundle name is unset.
func (bn BundleName) IsZero() bool {
	return bn.value == ""
}

// MarshalText provides support for logging and serialization.
func (bn BundleName) MarshalText() ([]byte, error) {
	return []byte(bn.value), nil
}

// UnmarshalText parses serialized text into a known BundleName.
func (bn *BundleName) UnmarshalText(data []byte) error {
	name, err := ParseBundleName(string(data))
	if err != nil {
		return err
	}

	*bn = name
	return nil
}

// ParseBundleName parses value and returns the corresponding BundleName when
// it exists.
func ParseBundleName(value string) (BundleName, error) {
	name, exists := bundleNames[value]
	if !exists {
		return BundleName{}, fmt.Errorf("invalid Malina bundle name %q", value)
	}

	return name, nil
}

// MustParseBundleName parses value and returns the corresponding BundleName.
// It panics when value does not identify a known bundle.
func MustParseBundleName(value string) BundleName {
	name, err := ParseBundleName(value)
	if err != nil {
		panic(err)
	}

	return name
}

// SupportedBundles returns the curated bundle names in stable sorted order.
func SupportedBundles() []BundleName {
	names := make([]BundleName, 0, len(bundleNames))
	for _, name := range bundleNames {
		names = append(names, name)
	}
	slices.SortFunc(names, func(a, b BundleName) int {
		return cmp.Compare(a.value, b.value)
	})
	return names
}

// FileRole identifies a model component's configuration role.
type FileRole string

// Stable-diffusion model component roles.
const (
	RoleModel          FileRole = "model"
	RoleDiffusion      FileRole = "diffusion"
	RoleVAE            FileRole = "vae"
	RoleClipL          FileRole = "clip_l"
	RoleClipG          FileRole = "clip_g"
	RoleT5XXL          FileRole = "t5xxl"
	RoleLLM            FileRole = "llm"
	RoleLLMVision      FileRole = "llm_vision"
	RoleControlNet     FileRole = "control_net"
	RoleTAESD          FileRole = "taesd"
	RolePhotoMaker     FileRole = "photo_maker"
	RoleClipVision     FileRole = "clip_vision"
	RoleHighNoise      FileRole = "high_noise"
	RoleEmbeddingsConn FileRole = "embeddings_conn"
)

// BundleFile describes one bundle file.
type BundleFile struct {
	Role     FileRole
	Filename string
	URL      string
	Size     string
}

// Bundle describes a curated model bundle.
type Bundle struct {
	Name        BundleName
	Description string
	License     string
	Gated       bool
	Files       []BundleFile
}

// Validate reports whether a bundle has the metadata required by the
// downloader and contains no duplicate component roles.
func (b Bundle) Validate() error {
	if b.Name.IsZero() || b.License == "" || len(b.Files) == 0 {
		return fmt.Errorf("bundle %q: incomplete metadata", b.Name)
	}
	seen := make(map[FileRole]struct{}, len(b.Files))
	for _, file := range b.Files {
		if file.Role == "" || file.Filename == "" || file.URL == "" {
			return fmt.Errorf("bundle %q: incomplete file metadata", b.Name)
		}
		if _, ok := seen[file.Role]; ok {
			return fmt.Errorf("bundle %q: duplicate role %q", b.Name, file.Role)
		}
		seen[file.Role] = struct{}{}
	}
	return nil
}

// Manifest records role-to-path mappings for a complete bundle.
type Manifest struct {
	Bundle  BundleName        `json:"bundle"`
	License string            `json:"license"`
	Gated   bool              `json:"gated"`
	Files   map[string]string `json:"files"`
}

// ManifestFilename is the completion marker filename.
const ManifestFilename = "manifest.json"

// Catalog returns the curated model bundles supported by Kronk's Malina SDK.
func Catalog() []Bundle {
	return []Bundle{
		{
			Name:        BundleSD15,
			Description: "Stable Diffusion v1.5 — classic baseline model, single safetensors file (~4.3 GB).",
			License:     "CreativeML Open RAIL-M",
			Files: []BundleFile{
				{
					Role:     RoleModel,
					Filename: "v1-5-pruned-emaonly.safetensors",
					URL:      "https://huggingface.co/stable-diffusion-v1-5/stable-diffusion-v1-5/resolve/main/v1-5-pruned-emaonly.safetensors",
					Size:     "4.3 GB",
				},
			},
		},
		{
			Name:        BundleSDXLBase10,
			Description: "Stable Diffusion XL base 1.0 — mainstream high-quality baseline, single safetensors file (~6.9 GB).",
			License:     "CreativeML Open RAIL++-M",
			Files: []BundleFile{
				{
					Role:     RoleModel,
					Filename: "sd_xl_base_1.0.safetensors",
					URL:      "https://huggingface.co/stabilityai/stable-diffusion-xl-base-1.0/resolve/main/sd_xl_base_1.0.safetensors",
					Size:     "6.9 GB",
				},
			},
		},
		{
			Name:        BundleFlux2Klein4B,
			Description: "FLUX.2 [klein] 4B — compact 4-step distilled model with Qwen3-4B text encoder. Three files (~5.3 GB total).",
			License:     "FLUX Non-Commercial",
			Gated:       true,
			Files: []BundleFile{
				{
					Role:     RoleDiffusion,
					Filename: "flux-2-klein-4b-Q4_0.gguf",
					URL:      "https://huggingface.co/leejet/FLUX.2-klein-4B-GGUF/resolve/main/flux-2-klein-4b-Q4_0.gguf",
					Size:     "2.5 GB",
				},
				{
					Role:     RoleVAE,
					Filename: "ae.safetensors",
					URL:      "https://huggingface.co/black-forest-labs/FLUX.2-dev/resolve/main/ae.safetensors",
					Size:     "335 MB",
				},
				{
					Role:     RoleLLM,
					Filename: "Qwen3-4B-Q4_K_M.gguf",
					URL:      "https://huggingface.co/unsloth/Qwen3-4B-GGUF/resolve/main/Qwen3-4B-Q4_K_M.gguf",
					Size:     "2.5 GB",
				},
			},
		},
		{
			Name:        BundleFlux2Klein9B,
			Description: "FLUX.2 [klein] 9B — flagship 4-step distilled model with Qwen3-8B text encoder. Three files (~16 GB total).",
			License:     "FLUX Non-Commercial",
			Gated:       true,
			Files: []BundleFile{
				{
					Role:     RoleDiffusion,
					Filename: "flux-2-klein-9b-Q4_0.gguf",
					URL:      "https://huggingface.co/leejet/FLUX.2-klein-9B-GGUF/resolve/main/flux-2-klein-9b-Q4_0.gguf",
					Size:     "5.6 GB",
				},
				{
					Role:     RoleVAE,
					Filename: "ae.safetensors",
					URL:      "https://huggingface.co/black-forest-labs/FLUX.2-dev/resolve/main/ae.safetensors",
					Size:     "335 MB",
				},
				{
					Role:     RoleLLM,
					Filename: "Qwen3-8B-Q4_K_M.gguf",
					URL:      "https://huggingface.co/unsloth/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q4_K_M.gguf",
					Size:     "5.0 GB",
				},
			},
		},
	}
}

// BundleByName finds a curated bundle.
func BundleByName(name BundleName) (Bundle, bool) {
	for _, bundle := range Catalog() {
		if bundle.Name.Equal(name) {
			return bundle, true
		}
	}
	return Bundle{}, false
}
