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
	kindList     // comma separated, written as a YAML list
	kindPassword // typed in clear, written as its SHA-256
	kindVars     // name=value lines, written as a map; see vars.go
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
	gThroughput = "Throughput"
	gFeatures   = "Features"
)

var groups = []group{
	{name: gEssentials},
	{name: gBehavior},
	{name: gSampling},
	{name: gMemory},
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

// questions returns the model questions in the order they are asked. The
// list comes from the init question map (#833).
func questions() []question {
	return []question{

		// Essentials

		{
			key: "context-window", group: gEssentials, kind: kindInt, min: 512,
			title: "How long do conversations need to be?",
			about: "Context window in tokens. Longer uses more memory.",
			options: func(f facts) []option {
				opts := []option{{"8K (8192)", "8192"}, {"32K (32768)", "32768"}, {"128K (131072)", "131072"}}
				if n := f.Analysis.Model.TrainingContext; n > 0 && !slices.ContainsFunc(opts, func(o option) bool { return o.value == strconv.FormatInt(n, 10) }) {
					opts = append(opts, option{fmt.Sprintf("Training max (%d)", n), strconv.FormatInt(n, 10)})
				}
				return opts
			},
			def:  func(f facts) string { return autoTune(f.Analysis.Recommended.ContextWindow) },
			show: always,
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
			title:   "Should the model think before answering by default?",
			options: fixed(option{"On", "true"}, option{"Off", "false"}),
			def:     func(facts) string { return "on" },
			same:    "true",
			show:    func(f facts, _ *answers) bool { return f.Thinking },
		},
		{
			key: "sampling-parameters.reasoning_effort", group: gBehavior, kind: kindChoice,
			title:   "Default reasoning effort?",
			about:   "Some templates accept fewer levels.",
			options: fixed(option{"none", "none"}, option{"minimal", "minimal"}, option{"low", "low"}, option{"medium", "medium"}, option{"high", "high"}),
			def:     func(facts) string { return "the template's choice" },
			show:    func(f facts, _ *answers) bool { return f.Reasoning },
		},
		{
			key: keyTemplateVars, group: gBehavior, kind: kindVars,
			title: "Custom chat template variables?",
			about: "One name=value per line, e.g. preserve_thinking=true. Only templates that use a name respond to it. Request values override these.",
			def:   func(facts) string { return "none" },
			show:  always,
		},

		// Sampling

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

		// Memory and GPU

		{
			key: "cache-type-k", also: []string{"cache-type-v"}, group: gMemory, kind: kindChoice,
			title: "KV cache precision?",
			about: "Lower saves memory and may cost some quality. Applies to keys and values.",
			options: fixed(option{"f16", "f16"}, option{"q8_0", "q8_0"}, option{"q4_0", "q4_0"}, option{"bf16", "bf16"},
				option{"f32", "f32"}, option{"q4_1", "q4_1"}, option{"q5_0", "q5_0"}, option{"q5_1", "q5_1"}),
			def:  func(f facts) string { return autoTuneStr(f.Analysis.Recommended.CacheTypeK) },
			show: always,
		},
		{
			key: "ngpu-layers", group: gMemory, kind: kindInt, min: -1,
			title:   "How many model layers go on the GPU?",
			options: fixed(option{"All layers", "0"}, option{"None (CPU only)", "-1"}),
			def:     func(facts) string { return "all layers" },
			same:    "0",
			show:    func(f facts, _ *answers) bool { return f.GPUs > 0 || !f.HardwareKnown },
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
			key: "moe.keep-experts-top-n", group: gMemory, kind: kindInt, min: 0,
			title: "How many top layers keep their experts on GPU?",
			def:   func(facts) string { return "0" },
			show:  func(f facts, a *answers) bool { return f.moe() && a.value("moe.mode") == "keep_top_n" },
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
			title:   "Sliding window cache?",
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

		// Throughput

		{
			key: "queue-depth", group: gThroughput, kind: kindInt, min: 1,
			title: "How many requests can wait in line per slot?",
			def:   func(facts) string { return "2" },
			show:  always,
		},
		{
			key: "admission-timeout", group: gThroughput, kind: kindDuration, min: 1,
			title: "How long can a request wait for a slot?",
			def:   func(facts) string { return "3m" },
			show:  always,
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

		// Features

		{
			key: "speculation", group: gFeatures, kind: kindChoice,
			title: "Speculative decoding?",
			options: func(f facts) []option {
				opts := []option{{"auto", "auto"}, {"disabled", "disabled"}}
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
			key: "draft-model.ndraft", group: gFeatures, kind: kindInt, min: 1,
			title: "How many draft tokens per step?",
			def:   func(facts) string { return "3" },
			show:  func(f facts, a *answers) bool { return f.mtp() && a.value("speculation") != "disabled" },
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
			key: "image-token-budget", group: gFeatures, kind: kindInt, min: 1,
			title: "Image detail budget (tokens per image)?",
			about: "Lower means less detail and less memory.",
			def:   func(facts) string { return "the projector's own value" },
			show:  func(f facts, _ *answers) bool { return f.projection() },
		},
	}
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
}

func newAnswers(qs []question) *answers {
	a := answers{
		byKey: make(map[string]question, len(qs)),
		sel:   make(map[string]*string, len(qs)),
		other: make(map[string]*string, len(qs)),
	}

	for _, q := range qs {
		a.byKey[q.key] = q
		a.sel[q.key] = new(string)
		a.other[q.key] = new(string)
	}

	return &a
}

// value is the final answer for key. Empty means keep Kronk's default.
func (a *answers) value(key string) string {
	sel, ok := a.sel[key]
	if !ok {
		return ""
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
