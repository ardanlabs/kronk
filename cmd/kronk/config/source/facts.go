package source

import (
	"strings"

	"github.com/ardanlabs/kronk/cmd/kronk/config/configfile"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// Audio (Bucky, whisper.cpp) and image (Malina, stable-diffusion.cpp) models
// have their own entries under bucky-models: and malina-models:, and their own
// questions. They have no GGUF analysis.

// ModelKind is the kind of model in the picker. It names the section of the
// config file the entry goes to, and the questions asked.
type ModelKind string

const (
	KindChatModel  ModelKind = ""
	KindAudioModel ModelKind = "audio"
	KindImageModel ModelKind = "image"
)

// SectionOf is the config file section a model kind is written to.
func SectionOf(k ModelKind) string {
	switch k {
	case KindAudioModel:
		return configfile.SectionBucky
	case KindImageModel:
		return configfile.SectionMalina
	}
	return configfile.SectionModels
}

// Choice is one model the user can configure.
type Choice struct {
	ID        string
	Kind      ModelKind
	Installed bool
	InFile    bool
	Size      int64

	// Validated is false for an installed model whose files were never
	// checked, which includes downloads that stopped early.
	Validated bool

	// Endpoint is what the model does when it doesn't generate text
	// (embeddings, rerank, decision): from the catalog, or the local GGUF
	// header for a model the catalog doesn't know. Empty for a chat model.
	Endpoint string

	// Tokenizer is an installed model's tokenizer fingerprint,
	// "<tokenizer model>:<pre-tokenizer>", empty when unknown.
	Tokenizer string
}

// Facts is what the questions need to know about the picked model and the
// machine.
type Facts struct {
	ID       string
	Info     models.ModelInfo
	Analysis models.Analysis

	// Inputs are what the model's projector takes besides text, when it has
	// one.
	Inputs Inputs

	// Endpoint is what the model does, from its GGUF metadata: EndpointChat,
	// or embeddings, rerank or decision, which don't generate text.
	Endpoint string

	// HardwareKnown is false when the llama.cpp libraries could not be
	// loaded, so devices and the memory check are unavailable.
	HardwareKnown bool
	HardwareErr   error

	// Devices is the hardware the analysis and the memory check use: this
	// machine's, or the one the user described when Other is set.
	Devices devices.Devices
	Other   *Machine

	// NUMANodes is the number of NUMA nodes, 0 when unknown (another
	// machine).
	NUMANodes int

	// Templates resolves the chat template for the template answer. Nil
	// when there is no model template to read (tests, audio and image).
	Templates *templates

	// Drafts are the installed chat models that can draft for this one in
	// classic speculation: same tokenizer (Tokenizer), the SDK's rule for
	// pairing a draft. The runtime refuses a draft whose vocabulary differs.
	// Adapters are the LoRA adapters in <base>/lora.
	Drafts   []string
	Adapters []string
}

// GPUNames are the GPU device names, e.g. CUDA0, CUDA1.
func (f Facts) GPUNames() []string {
	var out []string
	for _, d := range f.Devices.Devices {
		if strings.HasPrefix(d.Type, "gpu") {
			out = append(out, d.Name)
		}
	}
	return out
}

// Template is the chat template the server uses when the entry's template:
// is file, or has none when file is empty.
func (f Facts) Template(file string) ChatTemplate {
	if f.Templates == nil {
		return ChatTemplate{}
	}
	return f.Templates.forFile(file)
}

// GPUs is the number of GPUs.
func (f Facts) GPUs() int { return len(f.GPUNames()) }

// EndpointChat is the endpoint of a model that generates text.
const EndpointChat = "chat_completion"

// Generates reports whether the model generates text: only then do
// sampling, thinking, the chat template, speculation and the conversation
// cache apply. Embedding and rerank models pool the prompt, and decision
// models score options with their own temperature.
func (f Facts) Generates() bool { return f.Endpoint == EndpointChat }

// Images reports whether the model takes images: its projector has a
// vision encoder, or the projector couldn't be read.
func (f Facts) Images() bool {
	return f.Projection() && (!f.Inputs.Known || f.Inputs.Images)
}

// InputsText names the projector's inputs for the screens: "images",
// "audio", "images and audio", or "images or audio" when the projector
// couldn't be read.
func (f Facts) InputsText() string {
	switch {
	case !f.Inputs.Known:
		return "images or audio"
	case f.Inputs.Images && f.Inputs.Audio:
		return "images and audio"
	case f.Inputs.Audio:
		return "audio"
	}
	return "images"
}

// Tokenizer is the model's tokenizer fingerprint from its metadata, in the
// form of Choice.Tokenizer, empty when the metadata doesn't say.
func (f Facts) Tokenizer() string {
	model := f.Info.Metadata["tokenizer.ggml.model"]
	if model == "" {
		return ""
	}
	return model + ":" + f.Info.Metadata["tokenizer.ggml.pre"]
}

// MoE, Projection, MTP and SWA report the model traits the questions
// depend on.
func (f Facts) MoE() bool { return f.Analysis.Model.MoE != nil }

func (f Facts) Projection() bool { return f.Info.HasProjection }

func (f Facts) MTP() bool { return f.Info.HasMTP }

func (f Facts) SWA() bool { return f.Analysis.Model.Attention.SlidingWindow > 0 }
