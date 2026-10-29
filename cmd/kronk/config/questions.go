package config

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// kind is the type of value a question writes.
type kind int

const (
	kindChoice kind = iota
	kindBool
	kindInt
	kindFloat
	kindDuration
	kindText
	kindList      // comma separated, written as a YAML list
	kindPassword  // typed in clear, written as its SHA-256
	kindVars      // name=value lines, written as a map; see vars.go
	kindMulti     // several options at once, written as a list
	kindFloatList // comma separated numbers, written as a list
	kindAdapters  // adapter lines, written as a list of maps; see adapters.go
)

// otherValue is the select value that opens a free-form input.
const otherValue = "\x00other"

type option struct {
	label string
	value string
}

func fixed(opts ...option) func(facts) []option {
	return func(facts) []option { return opts }
}

// group is a set of questions, asked in the order of groups.
type group struct {
	name string
}

const (
	gEssentials = "Essentials"
	gBehavior   = "Behavior"
	gSampling   = "Sampling"
	gMemory     = "Memory and GPU"
	gGPUs       = "Multiple GPUs"
	gThroughput = "Throughput"
	gFeatures   = "Features"
)

var groups = []group{
	{name: gEssentials},
	{name: gBehavior},
	{name: gSampling},
	{name: gMemory},
	{name: gGPUs},
	{name: gThroughput},
	{name: gFeatures},
}

// question is one setting in the model entry. key is the YAML path inside
// the entry (dot separated). TestQuestionKeys checks every key and value
// against models.ModelConfig, so a renamed setting fails the build's tests.
type question struct {
	key     string
	also    []string
	group   string
	title   string
	about   string
	kind    kind
	options func(facts) []option
	min     float64
	max     float64
	def     func(facts) string
	show    func(facts, *answers) bool

	// same is the option value that behaves like leaving the key out. It is
	// not listed, since the "(default)" option already covers it, and a file
	// that already has it is left as it is.
	same string

	// secret is never shown or prefilled. An empty answer keeps the value
	// in the file instead of removing it.
	secret bool

	// sameAs names the question whose answer this one follows when left
	// empty, e.g. value precision follows key precision. An empty answer
	// leaves the key to that question.
	sameAs string

	// required lists no "(default)" option: the setting needs a value when
	// the question is asked.
	required bool

	// otherLabel replaces "Other value…" for the free-form option.
	otherLabel string
}

func always(facts, *answers) bool { return true }

func (q question) keys() []string {
	return append([]string{q.key}, q.also...)
}

// parse converts an answer to the typed value written to YAML.
func (q question) parse(s string) (any, error) {
	switch q.kind {
	case kindBool:
		return strconv.ParseBool(s)

	case kindInt:
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, errors.New("enter a whole number")
		}
		if err := q.inRange(float64(n)); err != nil {
			return nil, err
		}
		return n, nil

	case kindFloat:
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, errors.New("enter a number")
		}
		if err := q.inRange(n); err != nil {
			return nil, err
		}
		return n, nil

	case kindDuration:
		d, err := time.ParseDuration(s)
		if err != nil {
			return nil, errors.New("enter a duration like 30s, 3m or 1h")
		}
		if d < 0 || (d == 0 && q.min > 0) {
			return nil, errors.New("must be greater than zero")
		}
		return s, nil

	case kindList:
		var out []string
		for part := range strings.SplitSeq(s, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
		return out, nil

	case kindPassword:
		return hashPassword(s), nil

	case kindVars:
		return parseVars(s)

	case kindFloatList:
		var out []float64
		for part := range strings.SplitSeq(s, ",") {
			part = strings.TrimSpace(part)
			n, err := strconv.ParseFloat(part, 64)
			if err != nil || n < 0 {
				return nil, fmt.Errorf("%q: enter numbers of 0 or more, comma separated", part)
			}
			out = append(out, n)
		}
		return out, nil

	case kindMulti:
		var out []string
		for part := range strings.SplitSeq(s, ",") {
			if part != "" {
				out = append(out, part)
			}
		}
		return out, nil

	case kindAdapters:
		return parseAdapters(s)
	}

	return s, nil
}

func (q question) inRange(n float64) error {
	if n < q.min {
		return fmt.Errorf("must be %s or more", trimFloat(q.min))
	}
	if q.max > 0 && n > q.max {
		return fmt.Errorf("must be %s or less", trimFloat(q.max))
	}
	return nil
}

// validate is used by the free-form inputs. Empty keeps the default.
func (q question) validate(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	_, err := q.parse(s)
	return err
}

func trimFloat(n float64) string {
	return strconv.FormatFloat(n, 'f', -1, 64)
}

// =============================================================================

// questions returns the chat model questions in the order they are asked.
func questions() []question {
	// f32 is left out: more memory than f16 for no gain.
	cacheTypes := fixed(option{"f16", "f16"}, option{"bf16", "bf16"}, option{"q8_0", "q8_0"}, option{"q5_1", "q5_1"},
		option{"q5_0", "q5_0"}, option{"q4_1", "q4_1"}, option{"q4_0", "q4_0"})

	return []question{

		// Essentials

		{
			key: "context-window", group: gEssentials, kind: kindInt, min: 512,
			title: "How long do conversations need to be?",
			about: "Context window in tokens. Longer uses more memory.",
			// Sizes up to the training context. A longer one goes through
			// "Other value…" and the YaRN question.
			options: func(f facts) []option {
				train := f.Analysis.Model.TrainingContext
				var opts []option
				for _, o := range []option{{"8K (8192)", "8192"}, {"32K (32768)", "32768"}, {"128K (131072)", "131072"}} {
					if n, _ := strconv.ParseInt(o.value, 10, 64); train <= 0 || n < train {
						opts = append(opts, o)
					}
				}
				if train > 0 {
					opts = append(opts, option{fmt.Sprintf("Training max (%d)", train), strconv.FormatInt(train, 10)})
				}
				return opts
			},
			def:  func(f facts) string { return autoTune(f.Analysis.Recommended.ContextWindow) },
			show: always,
		},
		{
			key: keyRopeScaling, group: gEssentials, kind: kindChoice,
			title:   "That's longer than the model was trained on. Lower it, or keep it with YaRN?",
			about:   "YaRN extends the context and can lower answer quality. See chapter 7 of the manual.",
			options: fixed(option{"Keep it with YaRN (only if the model's documentation supports it)", "yarn"}),
			def:     func(f facts) string { return fmt.Sprintf("lower it to %d", f.Analysis.Model.TrainingContext) },
			show:    longContext,
		},
		{
			key: "nseq-max", group: gEssentials, kind: kindInt, min: 1,
			title: "How many requests should run at the same time?",
			about: "Each slot keeps its own conversation memory.",
			def:   func(f facts) string { return autoTune(f.Analysis.Recommended.NSeqMax) },
			show:  always,
		},

		// Behavior

		{
			key: "sampling-parameters.enable_thinking", group: gBehavior, kind: kindBool,
			title:   "Should the model think before it answers?",
			about:   "Used when a request doesn't say. A request can still turn thinking on or off.",
			options: fixed(option{"Yes", "true"}, option{"No", "false"}),
			def:     func(facts) string { return "yes" },
			same:    "true",
			show:    func(f facts, _ *answers) bool { return f.Thinking },
		},
		{
			key: "sampling-parameters.reasoning_effort", group: gBehavior, kind: kindChoice,
			title:   "How much should the model reason?",
			about:   "Used when a request doesn't say; a request can still set its own. Some models accept fewer levels.",
			options: fixed(option{"none", "none"}, option{"minimal", "minimal"}, option{"low", "low"}, option{"medium", "medium"}, option{"high", "high"}),
			def:     func(facts) string { return "the template's choice" },
			show:    func(f facts, _ *answers) bool { return f.Reasoning },
		},

		// Sampling (B3)

		{
			key: "sampling-parameters.temperature", group: gSampling, kind: kindFloat, min: 0, max: 5,
			title: "Temperature?",
			def:   func(f facts) string { return metaOr(f, "general.sampling.temp", "1.0") },
			show:  always,
		},
		{
			key: "sampling-parameters.top_p", group: gSampling, kind: kindFloat, min: 0, max: 1,
			title: "Top P?",
			def:   func(f facts) string { return metaOr(f, "general.sampling.top_p", "0.95") },
			show:  always,
		},
		{
			key: "sampling-parameters.top_k", group: gSampling, kind: kindInt, min: 0,
			title: "Top K?",
			def:   func(f facts) string { return metaOr(f, "general.sampling.top_k", "20") },
			show:  always,
		},
		{
			key: "sampling-parameters.min_p", group: gSampling, kind: kindFloat, min: 0, max: 1,
			title: "Min P?",
			def:   func(f facts) string { return metaOr(f, "general.sampling.min_p", "0") },
			show:  always,
		},
		{
			key: "sampling-parameters.max_tokens", group: gSampling, kind: kindInt, min: 1,
			title: "Max tokens per answer?",
			def:   func(facts) string { return "the context window" },
			show:  always,
		},
		{
			key: "sampling-parameters.repeat_penalty", group: gSampling, kind: kindFloat, min: 0,
			title: "Repeat penalty?",
			def:   func(facts) string { return "1.0" },
			show:  always,
		},

		// Penalties. Repeat window, DRY, XTC and seed are left to
		// requests: niche, and a per-model seed makes every request repeatable.

		{
			key: "sampling-parameters.frequency_penalty", group: gSampling, kind: kindFloat, min: -2, max: 2,
			title: "Frequency penalty?",
			about: "Discourages tokens by how often they already appeared.",
			def:   func(facts) string { return "0 (off)" },
			show:  always,
		},
		{
			key: "sampling-parameters.presence_penalty", group: gSampling, kind: kindFloat, min: -2, max: 2,
			title: "Presence penalty?",
			about: "Discourages tokens that already appeared at all.",
			def:   func(facts) string { return "0 (off)" },
			show:  always,
		},

		// Template

		{
			key: "template", group: gBehavior, kind: kindText,
			title: "Chat template file to use instead of the model's own?",
			about: "Rarely needed. A Jinja file path on the machine that runs the server.",
			def:   func(facts) string { return "the model's own template" },
			show:  always,
		},
		{
			key: keyTemplateVars, group: gBehavior, kind: kindVars,
			title: "Custom chat template variables?",
			about: "One name=value per line, e.g. preserve_thinking=true. Only templates that use a name respond to it. Request values override these.",
			def:   func(facts) string { return "none" },
			show:  always,
		},

		// Memory and GPU

		{
			key: "cache-type-k", also: []string{"cache-type-v"}, group: gMemory, kind: kindChoice,
			title:   "KV cache precision?",
			about:   "Lower saves memory and may cost some quality. Applies to keys, and to values unless the next question says otherwise.",
			options: cacheTypes,
			def:     func(f facts) string { return autoTuneStr(f.Analysis.Recommended.CacheTypeK) },
			show:    always,
		},
		{
			key: "cache-type-v", group: gMemory, kind: kindChoice, sameAs: "cache-type-k",
			title:   "KV cache precision for values?",
			options: cacheTypes,
			def:     func(facts) string { return "same as keys" },
			show:    func(_ facts, a *answers) bool { return a.value("cache-type-k") != "" },
		},
		{
			key: "ngpu-layers", group: gMemory, kind: kindInt, min: -1,
			title:      "How many model layers go on the GPU?",
			options:    fixed(option{"All layers", "0"}, option{"None (CPU only)", "-1"}),
			otherLabel: "Some layers…",
			def:        func(facts) string { return "all layers" },
			same:       "0",
			show:       func(f facts, _ *answers) bool { return f.GPUs > 0 || !f.HardwareKnown },
		},
		{
			key: "offload-kqv", group: gMemory, kind: kindBool,
			title:   "Keep the conversation memory (KV cache) on the GPU?",
			about:   "No saves GPU memory but is slower.",
			options: fixed(option{"Yes", "true"}, option{"No", "false"}),
			def:     func(facts) string { return "yes" },
			same:    "true",
			show:    func(f facts, a *answers) bool { return f.GPUs > 0 && a.value("ngpu-layers") != "-1" },
		},
		{
			key: "moe.mode", group: gMemory, kind: kindChoice,
			title: "Where do the MoE experts go?",
			options: fixed(option{"auto", "auto"}, option{"All on CPU", "experts_cpu"}, option{"All on GPU", "experts_gpu"},
				option{"Keep the top N layers on GPU", "keep_top_n"}),
			def:  func(facts) string { return "auto" },
			same: "auto",
			show: func(f facts, _ *answers) bool { return f.moe() },
		},
		{
			key: "moe.keep-experts-top-n", group: gMemory, kind: kindInt, min: 0, required: true,
			title: "How many top layers keep their experts on GPU?",
			def:   func(facts) string { return "0" },
			show:  func(f facts, a *answers) bool { return f.moe() && a.value("moe.mode") == "keep_top_n" },
		},
		{
			key: "numa", group: gMemory, kind: kindChoice,
			title: "NUMA strategy?",
			about: "Matters on machines with more than one CPU socket when experts or layers run on the CPU. With distribute, also set weight loading to none.",
			options: fixed(option{"distribute", "distribute"}, option{"isolate", "isolate"}, option{"numactl", "numactl"},
				option{"mirror", "mirror"}),
			def:  func(facts) string { return "disabled" },
			show: cpuWork,
		},
		{
			key: "flash-attention", group: gMemory, kind: kindChoice,
			title:   "Flash attention?",
			options: fixed(option{"auto", "auto"}, option{"enabled", "enabled"}, option{"disabled", "disabled"}),
			def:     func(f facts) string { return autoTuneStr(f.Analysis.Recommended.FlashAttention) },
			show:    always,
		},
		{
			key: "swa-full", group: gMemory, kind: kindBool,
			title:   "Sliding window cache: compact or full?",
			options: fixed(option{"Compact (less memory)", "false"}, option{"Full", "true"}),
			def:     func(facts) string { return "llama.cpp's choice" },
			show:    func(f facts, _ *answers) bool { return f.swa() },
		},
		{
			key: "load-mode", group: gMemory, kind: kindChoice,
			title: "How should the weights load?",
			options: fixed(option{"auto", "auto"}, option{"mmap", "mmap"}, option{"none", "none"}, option{"mlock", "mlock"},
				option{"mmap+mlock", "mmap+mlock"}, option{"direct-io", "direct-io"}),
			def:  func(facts) string { return "auto" },
			same: "auto",
			show: always,
		},
		{
			key: "lazy-mode", group: gMemory, kind: kindChoice,
			title:   "Lazy loading: read large tensors only when they are needed?",
			options: fixed(option{"Auto", "auto"}, option{"No: read everything at load", "off"}, option{"Yes", "on"}),
			def:     func(facts) string { return "auto" },
			same:    "auto",
			show:    always,
		},

		// Multiple GPUs

		{
			key: "devices", group: gGPUs, kind: kindMulti,
			title: "Which GPUs should this model use?",
			about: "Space selects. None selected uses all of them.",
			options: func(f facts) []option {
				var opts []option
				for _, name := range f.gpuNames() {
					opts = append(opts, option{name, name})
				}
				return opts
			},
			def:  func(facts) string { return "all" },
			show: multiGPU,
		},
		{
			key: "split-mode", group: gGPUs, kind: kindChoice,
			title: "How should the model be split across them?",
			options: fixed(option{"layer: whole layers per GPU", "layer"}, option{"row: rows of each layer", "row"},
				option{"tensor: tensor parallel", "tensor"}, option{"none: one GPU runs the whole model", "none"}),
			def: func(f facts) string { return autoTuneStr(f.Analysis.Recommended.SplitMode) },
			show: func(f facts, a *answers) bool {
				return multiGPU(f, a) && len(*a.multi["devices"]) != 1
			},
		},
		{
			key: "main-gpu", group: gGPUs, kind: kindInt, min: 0,
			title: "Which of the GPUs runs the whole model?",
			about: "Its position among the GPUs this model uses (the ones picked above, or all): 0 is the first.",
			def:   func(facts) string { return "0" },
			show:  func(f facts, a *answers) bool { return multiGPU(f, a) && a.value("split-mode") == "none" },
		},
		{
			key: "tensor-split", group: gGPUs, kind: kindFloatList,
			title: "How much of the model goes on each GPU?",
			about: "Shares in GPU order, comma separated, e.g. 0.6, 0.4.",
			def:   func(facts) string { return "automatic, by free memory" },
			show: func(f facts, a *answers) bool {
				split := a.value("split-mode")
				return multiGPU(f, a) && len(*a.multi["devices"]) != 1 && (split == "" || split == "layer" || split == "row")
			},
		},

		// Throughput

		{
			key: "queue-depth", group: gThroughput, kind: kindInt, min: 1,
			title:   "How many requests may wait for each slot?",
			about:   "Requests beyond that wait to be admitted, up to the time in the next question, then fail.",
			options: fixed(option{"None: only running requests are admitted", "1"}, option{"One", "2"}, option{"Two", "3"}),
			def:     func(facts) string { return "one" },
			same:    "2",
			show:    always,
		},
		{
			key: "admission-timeout", group: gThroughput, kind: kindDuration, min: 1,
			title: "How long can a request wait to be admitted?",
			def:   func(facts) string { return "3m" },
			show:  always,
		},
		{
			key: "prefill-batch-size", group: gThroughput, kind: kindInt, min: 1,
			title: "How many prompt tokens per processing step?",
			def:   func(facts) string { return "2048" },
			show:  always,
		},
		{
			key: "kv-cache-mode", group: gThroughput, kind: kindChoice,
			title:   "Give each request its own memory slice, or share one pool?",
			options: fixed(option{"Own slice per request", "partitioned"}, option{"One shared pool", "shared"}),
			def:     func(facts) string { return "own slice per request" },
			same:    "partitioned",
			show:    func(f facts, a *answers) bool { return requests(f, a) > 1 },
		},
		{
			key: "nthreads", also: []string{"nthreads-batch"}, group: gThroughput, kind: kindInt, min: 1,
			title: "How many CPU threads?",
			def:   func(facts) string { return "all CPU cores, at least 4" },
			show:  cpuWork,
		},
		{
			key: "incremental-cache", group: gThroughput, kind: kindBool,
			title:   "Cache conversations between turns?",
			about:   "Faster for agents and long chats.",
			options: fixed(option{"Yes", "true"}, option{"No", "false"}),
			def:     func(facts) string { return "yes" },
			same:    "true",
			show:    always,
		},
		{
			key: "cache-min-tokens", group: gThroughput, kind: kindInt, min: 0,
			title: "Minimum prompt size before caching (tokens)?",
			def:   func(facts) string { return "100" },
			show:  func(_ facts, a *answers) bool { return a.value("incremental-cache") != "false" },
		},
		{
			key: "imc-session-capacity", group: gThroughput, kind: kindInt, min: 1,
			title: "How many conversations to keep cached?",
			about: "At least one per admitted request: requests at the same time × (1 + requests waiting per slot).",
			def:   func(f facts) string { return "one per admitted request" },
			show:  func(_ facts, a *answers) bool { return a.value("incremental-cache") != "false" },
		},

		// Features

		{
			key: "speculation", group: gFeatures, kind: kindChoice,
			title: "Speculative decoding?",
			about: "Auto uses a draft when one is available: a draft model, then an MTP head.",
			options: func(f facts) []option {
				opts := []option{{"auto", "auto"}, {"disabled", "disabled"}}
				if len(f.Installed) > 1 {
					opts = append(opts, option{"classic (a separate draft model)", "classic"})
				}
				if f.mtp() {
					opts = append(opts, option{"mtp (model's MTP head)", "mtp"})
				}
				return opts
			},
			def:  func(facts) string { return "auto" },
			same: "auto",
			show: always,
		},
		{
			key: "draft-model.model-id", group: gFeatures, kind: kindChoice, required: true,
			title: "Which installed model is the draft?",
			about: "A small model of the same family. Classic needs one request at a time, so nseq-max is set to 1.",
			options: func(f facts) []option {
				var opts []option
				for _, id := range f.Installed {
					if id != f.ID {
						opts = append(opts, option{id, id})
					}
				}
				return opts
			},
			def:  func(facts) string { return "" },
			show: func(_ facts, a *answers) bool { return a.value("speculation") == "classic" },
		},
		{
			key: "draft-model.ndraft", group: gFeatures, kind: kindInt, min: 1,
			title: "How many draft tokens per step?",
			def:   func(facts) string { return "3" },
			show: func(f facts, a *answers) bool {
				spec := a.value("speculation")
				return spec == "classic" || (f.mtp() && spec != "disabled")
			},
		},
		{
			key: "adapters", group: gFeatures, kind: kindAdapters,
			title: "LoRA adapters to load?",
			about: "One per line: an adapter from the lora folder, or an absolute path. Add =scale for strength, e.g. style=0.8.",
			def:   func(facts) string { return "none" },
			show:  func(f facts, _ *answers) bool { return len(f.Adapters) > 0 },
		},
		{
			key: "proj-on-cpu", group: gFeatures, kind: kindBool,
			title:   "Run the image projector on the CPU?",
			options: fixed(option{"No", "false"}, option{"Yes", "true"}),
			def:     func(facts) string { return "no" },
			same:    "false",
			show:    func(f facts, _ *answers) bool { return f.projection() },
		},
		{
			key: "proj-device", group: gFeatures, kind: kindChoice,
			title: "Which GPU should run the image projector?",
			options: func(f facts) []option {
				var opts []option
				for _, name := range f.gpuNames() {
					opts = append(opts, option{name, name})
				}
				return opts
			},
			def: func(facts) string { return "automatic" },
			show: func(f facts, a *answers) bool {
				return f.projection() && f.GPUs > 1 && a.value("proj-on-cpu") != "true"
			},
		},
		{
			key: "image-token-budget", group: gFeatures, kind: kindInt, min: 1,
			title: "Image detail budget (tokens per image)?",
			about: "Lower means less detail and less memory.",
			def:   func(facts) string { return "the projector's own value" },
			show:  func(f facts, _ *answers) bool { return f.projection() },
		},
	}
}

// multiGPU reports whether the machine has more than one GPU and the model
// uses them.
func multiGPU(f facts, a *answers) bool {
	return f.GPUs > 1 && a.value("ngpu-layers") != "-1"
}

// cpuWork reports whether some of the model runs on the CPU: no GPU, layers
// kept off the GPU, or MoE experts on the CPU.
func cpuWork(f facts, a *answers) bool {
	if f.HardwareKnown && f.GPUs == 0 {
		return true
	}
	if layers := a.value("ngpu-layers"); layers != "" && layers != "0" {
		return true
	}
	mode := a.value("moe.mode")
	return f.moe() && (mode == "experts_cpu" || mode == "keep_top_n")
}

// requests is the answered number of requests at the same time, or
// AutoTune's.
func requests(f facts, a *answers) int64 {
	if n, err := strconv.ParseInt(a.value("nseq-max"), 10, 64); err == nil {
		return n
	}
	return f.Analysis.Recommended.NSeqMax
}

func autoTune(n int64) string {
	if n <= 0 {
		return "AutoTune's pick"
	}
	return fmt.Sprintf("AutoTune's pick: %d", n)
}

func autoTuneStr(s string) string {
	if s == "" {
		return "AutoTune's pick"
	}
	return fmt.Sprintf("AutoTune's pick: %s", s)
}

func metaOr(f facts, key string, kronkDefault string) string {
	if v := f.Info.Metadata[key]; v != "" {
		return v + " (from the model)"
	}
	return kronkDefault
}

// =============================================================================

// answers holds the form values. A select stores its value in sel; when the
// user picks "Other value…" the typed value is in other.
type answers struct {
	byKey map[string]question
	sel   map[string]*string
	other map[string]*string
	multi map[string]*[]string // kindMulti answers
}

func newAnswers(qs []question) *answers {
	a := answers{
		byKey: make(map[string]question, len(qs)),
		sel:   make(map[string]*string, len(qs)),
		other: make(map[string]*string, len(qs)),
		multi: make(map[string]*[]string),
	}

	for _, q := range qs {
		a.byKey[q.key] = q
		a.sel[q.key] = new(string)
		a.other[q.key] = new(string)
		a.multi[q.key] = new([]string)
	}

	return &a
}

// value is the final answer for key. Empty means keep Kronk's default.
func (a *answers) value(key string) string {
	sel, ok := a.sel[key]
	if !ok {
		return ""
	}
	if a.byKey[key].kind == kindMulti {
		return strings.Join(*a.multi[key], ",")
	}
	if *sel == otherValue {
		return strings.TrimSpace(*a.other[key])
	}
	return strings.TrimSpace(*sel)
}

// prefill sets the answers from an existing entry, so editing starts from
// what is in the file.
func (a *answers) prefill(f facts, existing map[string]any) {
	for key, q := range a.byKey {
		if q.kind == kindVars {
			*a.sel[key] = formatVars(existingVars(existing))
			continue
		}

		v, ok := existing[key]
		if !ok || q.secret {
			continue
		}
		if q.sameAs != "" && fmt.Sprint(existing[q.sameAs]) == fmt.Sprint(v) {
			continue
		}

		switch q.kind {
		case kindAdapters:
			*a.sel[key] = formatAdapters(v)
			continue
		case kindMulti:
			list, _ := v.([]any)
			for _, item := range list {
				*a.multi[key] = append(*a.multi[key], fmt.Sprint(item))
			}
			continue
		case kindFloatList:
			list, _ := v.([]any)
			parts := make([]string, len(list))
			for i, item := range list {
				parts[i] = fmt.Sprint(item)
			}
			*a.sel[key] = strings.Join(parts, ", ")
			continue
		}

		s := fmt.Sprint(v)
		if list, ok := v.([]any); ok {
			parts := make([]string, len(list))
			for i, e := range list {
				parts[i] = fmt.Sprint(e)
			}
			s = strings.Join(parts, ",")
		}
		if q.same != "" && s == q.same {
			continue
		}
		if q.options == nil {
			*a.sel[key] = s
			continue
		}

		if slices.ContainsFunc(q.options(f), func(o option) bool { return o.value == s }) {
			*a.sel[key] = s
			continue
		}

		*a.sel[key] = otherValue
		*a.other[key] = s
	}
}

// applies reports whether q is asked for this model and these answers.
func (a *answers) applies(f facts, q question) bool {
	return q.show(f, a)
}

func groupOf(name string) group {
	for _, g := range groups {
		if g.name == name {
			return g
		}
	}
	return group{name: name}
}
