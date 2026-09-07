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

	// BundleControlNetCannySD15 identifies the SD 1.5 Canny ControlNet bundle.
	BundleControlNetCannySD15 = newBundleName("controlnet-canny-sd1.5")

	// BundleRealESRGANX4Anime identifies the Real-ESRGAN 4x anime upscaler bundle.
	BundleRealESRGANX4Anime = newBundleName("realesrgan-x4-anime")

	// BundleADetailerFaceYOLOv8N identifies the YOLOv8n ADetailer face detector bundle.
	BundleADetailerFaceYOLOv8N = newBundleName("adetailer-face-yolov8n")

	// BundleAnimateDiffSD15 identifies the SD 1.5 AnimateDiff bundle.
	BundleAnimateDiffSD15 = newBundleName("animatediff-sd1.5")

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
	RoleMotionModule   FileRole = "motion_module"
	RoleUpscaler       FileRole = "upscaler"
	RoleADetailer      FileRole = "adetailer"
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
					URL:      "https://huggingface.co/stable-diffusion-v1-5/stable-diffusion-v1-5/resolve/451f4fe16113bff5a5d2269ed5ad43b0592e9a14/v1-5-pruned-emaonly.safetensors",
					Size:     "4.3 GB",
				},
			},
		},
		{
			Name:        BundleControlNetCannySD15,
			Description: "Quantized SD 1.5 with fp16 Canny ControlNet conditioning. Two files (~2.3 GB total).",
			License:     "CreativeML Open RAIL-M / OpenRAIL",
			Files: []BundleFile{
				{
					Role:     RoleModel,
					Filename: "stable-diffusion-v1-5-pruned-emaonly-Q4_0.gguf",
					URL:      "https://huggingface.co/second-state/stable-diffusion-v1-5-GGUF/resolve/031b5f5df991f511b3f5fa8fed6d99048ababb69/stable-diffusion-v1-5-pruned-emaonly-Q4_0.gguf",
					Size:     "1.6 GB",
				},
				{
					Role:     RoleControlNet,
					Filename: "control_canny-fp16.safetensors",
					URL:      "https://huggingface.co/webui/ControlNet-modules-safetensors/resolve/5194dff6fe5e3d26310a12c527eae8bc02d3a482/control_canny-fp16.safetensors",
					Size:     "723 MB",
				},
			},
		},
		{
			Name:        BundleRealESRGANX4Anime,
			Description: "Real-ESRGAN 4x anime image upscaler (~18 MB).",
			License:     "BSD-3-Clause",
			Files: []BundleFile{
				{
					Role:     RoleUpscaler,
					Filename: "RealESRGAN_x4plus_anime_6B.pth",
					URL:      "https://github.com/xinntao/Real-ESRGAN/releases/download/v0.2.2.4/RealESRGAN_x4plus_anime_6B.pth",
					Size:     "18 MB",
				},
			},
		},
		{
			Name:        BundleADetailerFaceYOLOv8N,
			Description: "Quantized SD 1.5 with a converted YOLOv8n ADetailer face detector. Two files (~1.6 GB total).",
			License:     "CreativeML Open RAIL-M / AGPL-3.0",
			Files: []BundleFile{
				{
					Role:     RoleModel,
					Filename: "stable-diffusion-v1-5-pruned-emaonly-Q4_0.gguf",
					URL:      "https://huggingface.co/second-state/stable-diffusion-v1-5-GGUF/resolve/031b5f5df991f511b3f5fa8fed6d99048ababb69/stable-diffusion-v1-5-pruned-emaonly-Q4_0.gguf",
					Size:     "1.6 GB",
				},
				{
					Role:     RoleADetailer,
					Filename: "face_yolov8n.safetensors",
					URL:      "https://huggingface.co/exeterminal/adetailer-yolov8-safetensors/resolve/07ca0bd47f67955bf49e26c24d5aff8d161d81f2/face_yolov8n.safetensors",
					Size:     "6 MB",
				},
			},
		},
		{
			Name:        BundleAnimateDiffSD15,
			Description: "Quantized SD 1.5 with the fp16 AnimateDiff v3 motion module. Two files (~2.4 GB total).",
			License:     "CreativeML Open RAIL-M / Apache-2.0",
			Files: []BundleFile{
				{
					Role:     RoleModel,
					Filename: "stable-diffusion-v1-5-pruned-emaonly-Q4_0.gguf",
					URL:      "https://huggingface.co/second-state/stable-diffusion-v1-5-GGUF/resolve/031b5f5df991f511b3f5fa8fed6d99048ababb69/stable-diffusion-v1-5-pruned-emaonly-Q4_0.gguf",
					Size:     "1.6 GB",
				},
				{
					Role:     RoleMotionModule,
					Filename: "mm_sd15_v3.safetensors",
					URL:      "https://huggingface.co/conrevo/AnimateDiff-A1111/resolve/aa4a0ef5bd366a0ec898e7a64b6fc0f612e37444/motion_module/mm_sd15_v3.safetensors",
					Size:     "837 MB",
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
