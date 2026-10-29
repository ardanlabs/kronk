package questions

import (
	"fmt"
	"strconv"

	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
)

// Question groups, shown as headings in the answers list. The questions are
// asked in the order Chat lists them.
const (
	gEssentials = "Essentials"
	gBehavior   = "Behavior"
	gSampling   = "Sampling"
	gMemory     = "Memory and GPU"
	gGPUs       = "Multiple GPUs"
	gThroughput = "Throughput"
	gFeatures   = "Features"
)

// Chat returns the chat model questions in the order they are asked.
func Chat() []Question {
	// f32 is left out: more memory than f16 for no gain.
	cacheTypes := fixed(option{"f16", "f16"}, option{"bf16", "bf16"}, option{"q8_0", "q8_0"}, option{"q5_1", "q5_1"},
		option{"q5_0", "q5_0"}, option{"q4_1", "q4_1"}, option{"q4_0", "q4_0"})

	return []Question{

		// Essentials

		{
			Key: "context-window", Group: gEssentials, Kind: kindInt, min: 512,
			Title: "How long do conversations need to be?",
			About: "Context window in tokens. Longer uses more memory.",
			Tip:   "Chat needs 8K to 32K; agents and long documents need more. The KV cache grows with it, once per request at the same time.",
			pooled: wording{
				title: "How long can one input be?",
				tip:   "Embedding and rerank inputs longer than this are truncated. The KV cache grows with it, once per request at the same time.",
			},
			// Sizes up to the training context. A longer one goes through
			// "Other value…" and the YaRN question.
			Options: func(f source.Facts) []option {
				train := f.Analysis.Model.TrainingContext
				var opts []option
				for _, o := range []option{{"8K (8192)", "8192"}, {"32K (32768)", "32768"}, {"128K (131072)", "131072"}} {
					if n, _ := strconv.ParseInt(o.Value, 10, 64); train <= 0 || n < train {
						opts = append(opts, o)
					}
				}
				if train > 0 {
					opts = append(opts, option{fmt.Sprintf("Training max (%d)", train), strconv.FormatInt(train, 10)})
				}
				return opts
			},
			Def:  func(f source.Facts) string { return autoTune(f.Analysis.Recommended.ContextWindow) },
			show: always,
		},
		{
			Key: KeyRopeScaling, Group: gEssentials, Kind: kindChoice,
			Title:   "That's longer than the model was trained on. Lower it, or keep it with YaRN?",
			About:   "YaRN extends the context and can lower answer quality. See chapter 7 of the manual.",
			Options: fixed(option{"Keep it with YaRN (only if the model's documentation supports it)", "yarn"}),
			Def:     func(f source.Facts) string { return fmt.Sprintf("lower it to %d", f.Analysis.Model.TrainingContext) },
			show:    longContext,
		},
		{
			Key: "nseq-max", Group: gEssentials, Kind: kindInt, min: 1,
			Title:  "How many requests should run at the same time?",
			About:  "Each slot keeps its own conversation memory.",
			pooled: wording{about: "Each slot keeps its own KV cache."},
			Tip:    "Raise it when several users or agents call this model at once. Each one adds a context window of KV cache.",
			Def:    func(f source.Facts) string { return autoTune(f.Analysis.Recommended.NSeqMax) },
			show:   always,
		},

		// Behavior. The template comes first: the thinking and reasoning
		// questions are asked when it reads their variables.

		{
			Key: "template", Group: gBehavior, Kind: kindText, generation: true,
			Title: "Chat template file to use instead of the model's own?",
			About: "Rarely needed. A Jinja file path on the machine that runs the server.",
			Def:   text("the model's own template"),
			show:  always,
		},
		{
			Key: "sampling-parameters.enable_thinking", Group: gBehavior, Kind: kindBool, generation: true,
			Title:   "Should the model think before it answers?",
			About:   "Used when a request doesn't say. A request can still turn thinking on or off.",
			Tip:     "Thinking helps with hard problems and code, but answers take longer and use more tokens.",
			Options: yesNo,
			Def:     text("yes"),
			Same:    "true",
			show:    func(f source.Facts, a *Answers) bool { return f.Template(a.Value("template")).Reads("enable_thinking") },
		},
		{
			Key: "sampling-parameters.reasoning_effort", Group: gBehavior, Kind: kindChoice, generation: true,
			Title:   "How much should the model reason?",
			About:   "Used when a request doesn't say; a request can still set its own. Some models accept fewer levels.",
			Tip:     "Higher is slower and more thorough. Low or medium suits chat.",
			Options: fixed(option{"none", "none"}, option{"minimal", "minimal"}, option{"low", "low"}, option{"medium", "medium"}, option{"high", "high"}),
			Def:     text("the template's choice"),
			show: func(f source.Facts, a *Answers) bool {
				return f.Template(a.Value("template")).Reads("reasoning_effort")
			},
		},

		{
			Key: KeyTemplateVars, Group: gBehavior, Kind: KindVars, generation: true,
			Title: "Custom chat template variables?",
			About: "One name=value per line, e.g. preserve_thinking=true. Only templates that use a name respond to it. Request values override these.",
			Def:   text("none"),
			show:  always,
		},

		// Sampling

		{
			Key: "sampling-parameters.temperature", Group: gSampling, Kind: kindFloat, min: 0, max: 5, generation: true,
			Title: "Temperature?",
			About: "How random the answers are. 0 always picks the most likely token.",
			Tip:   "The model's own value is a good start. Lower it for code and facts, raise it for creative writing.",
			Def:   fromModel("general.sampling.temp", "1.0"),
			show:  always,
		},
		{
			Key: "sampling-parameters.top_p", Group: gSampling, Kind: kindFloat, min: 0, max: 1, generation: true,
			Title: "Top P?",
			About: "Samples only from the most likely tokens whose chances add up to this share.",
			Tip:   "Keep the model's value unless its documentation says otherwise.",
			Def:   fromModel("general.sampling.top_p", "0.95"),
			show:  always,
		},
		{
			Key: "sampling-parameters.top_k", Group: gSampling, Kind: kindInt, min: 0, generation: true,
			Title: "Top K?",
			About: "Samples only from this many of the most likely tokens. 0 turns it off.",
			Tip:   "Keep the model's value unless its documentation says otherwise.",
			Def:   fromModel("general.sampling.top_k", "20"),
			show:  always,
		},
		{
			Key: "sampling-parameters.min_p", Group: gSampling, Kind: kindFloat, min: 0, max: 1, generation: true,
			Title: "Min P?",
			About: "Drops tokens less likely than this share of the most likely one. 0 turns it off.",
			Tip:   "0.05 to 0.1 cuts odd words at higher temperatures.",
			Def:   fromModel("general.sampling.min_p", "0"),
			show:  always,
		},
		{
			Key: "sampling-parameters.max_tokens", Group: gSampling, Kind: kindInt, min: 1, generation: true,
			Title: "Max tokens per answer?",
			About: "Stops an answer after this many tokens. A request can set its own.",
			Tip:   "Leave it unless answers run on. Thinking models need room for their thinking too.",
			Def:   text("the context window"),
			show:  always,
		},
		{
			Key: "sampling-parameters.repeat_penalty", Group: gSampling, Kind: kindFloat, min: 0, generation: true,
			Title: "Repeat penalty?",
			About: "Above 1 discourages repeating recent tokens; 1 turns it off.",
			Tip:   "1.0 to 1.1. Higher values can hurt code and lists, which repeat by nature.",
			Def:   text("1.0"),
			show:  always,
		},

		// Penalties. Repeat window, DRY, XTC and seed are left to
		// requests: niche, and a per-model seed makes every request repeatable.

		{
			Key: "sampling-parameters.frequency_penalty", Group: gSampling, Kind: kindFloat, min: -2, max: 2, generation: true,
			Title: "Frequency penalty?",
			About: "Discourages tokens by how often they already appeared.",
			Tip:   "Keep 0 unless answers repeat themselves; then try 0.1 to 0.5.",
			Def:   text("0 (off)"),
			show:  always,
		},
		{
			Key: "sampling-parameters.presence_penalty", Group: gSampling, Kind: kindFloat, min: -2, max: 2, generation: true,
			Title: "Presence penalty?",
			About: "Discourages tokens that already appeared at all.",
			Tip:   "Keep 0 unless answers stick to one topic; then try 0.1 to 0.5.",
			Def:   text("0 (off)"),
			show:  always,
		},

		// Memory and GPU

		{
			Key: "cache-type-k", also: []string{"cache-type-v"}, Group: gMemory, Kind: kindChoice,
			Title:   "KV cache precision?",
			About:   "Lower saves memory and may cost some quality. Applies to keys, and to values unless the next question says otherwise.",
			Tip:     "q8_0 halves the cache with little quality loss. Go lower only when memory is very tight.",
			Options: cacheTypes,
			Def:     func(f source.Facts) string { return autoTuneStr(f.Analysis.Recommended.CacheTypeK) },
			show:    always,
		},
		{
			Key: "cache-type-v", Group: gMemory, Kind: kindChoice, sameAs: "cache-type-k",
			Title:   "KV cache precision for values?",
			About:   "Values can use a different precision than keys. A quantized value cache needs flash attention.",
			Tip:     "Values are less sensitive than keys, so a lower value precision costs less quality.",
			Options: cacheTypes,
			Def:     text("same as keys"),
			show:    func(_ source.Facts, a *Answers) bool { return a.Value("cache-type-k") != "" },
		},
		{
			Key: "ngpu-layers", Group: gMemory, Kind: kindInt, min: -1,
			Title:      "How many model layers go on the GPU?",
			About:      "Layers on the GPU run fast. The rest run on the CPU from system RAM.",
			Tip:        "Keep all layers when the model fits. Each layer moved to the CPU slows answers.",
			Options:    fixed(option{"All layers", "0"}, option{"None (CPU only)", "-1"}),
			OtherLabel: "Some layers…",
			Def:        text("all layers"),
			Same:       "0",
			show:       func(f source.Facts, _ *Answers) bool { return f.GPUs() > 0 || !f.HardwareKnown },
		},
		{
			Key: "offload-kqv", Group: gMemory, Kind: kindBool,
			Title:   "Keep the conversation memory (KV cache) on the GPU?",
			pooled:  wording{title: "Keep the KV cache on the GPU?"},
			About:   "No saves GPU memory but is slower.",
			Tip:     "Keep yes unless the GPU is short of memory.",
			Options: yesNo,
			Def:     text("yes"),
			Same:    "true",
			show:    func(f source.Facts, a *Answers) bool { return f.GPUs() > 0 && a.Value("ngpu-layers") != "-1" },
		},
		{
			Key: "moe.mode", Group: gMemory, Kind: kindChoice,
			Title: "Where do the MoE experts go?",
			About: "An MoE model uses only a few experts per token, so experts in system RAM cost less speed than other layers.",
			Tip:   "Auto uses the catalog's choice. All on CPU frees the most GPU memory; keep the top N layers is the middle ground.",
			Options: fixed(option{"auto", "auto"}, option{"All on CPU", "experts_cpu"}, option{"All on GPU", "experts_gpu"},
				option{"Keep the top N layers on GPU", "keep_top_n"}),
			Def:  text("auto"),
			Same: "auto",
			show: func(f source.Facts, _ *Answers) bool { return f.MoE() },
		},
		{
			Key: "moe.keep-experts-top-n", Group: gMemory, Kind: kindInt, min: 0, Required: true,
			Title: "How many top layers keep their experts on GPU?",
			About: "The experts of the other layers go to the CPU.",
			Tip:   "Raise it while GPU memory allows: each layer kept is faster.",
			Def:   text("0"),
			show:  func(f source.Facts, a *Answers) bool { return f.MoE() && a.Value("moe.mode") == "keep_top_n" },
		},
		{
			Key: "numa", Group: gMemory, Kind: kindChoice,
			Title: "NUMA strategy?",
			About: "Matters on machines with more than one CPU socket when experts or layers run on the CPU. With distribute, also set weight loading to none.",
			Tip:   "Leave it disabled on single-socket machines.",
			Options: fixed(option{"distribute", "distribute"}, option{"isolate", "isolate"}, option{"numactl", "numactl"},
				option{"mirror", "mirror"}),
			Def:  text("disabled"),
			show: func(f source.Facts, a *Answers) bool { return f.NUMANodes != 1 && cpuWork(f, a) },
		},
		{
			Key: "flash-attention", Group: gMemory, Kind: kindChoice,
			Title:   "Flash attention?",
			About:   "Faster attention that uses less memory, mostly with long contexts.",
			Tip:     "Keep auto: llama.cpp uses it when the backend supports it.",
			Options: fixed(option{"auto", "auto"}, option{"enabled", "enabled"}, option{"disabled", "disabled"}),
			Def:     func(f source.Facts) string { return autoTuneStr(f.Analysis.Recommended.FlashAttention) },
			show:    always,
		},
		{
			Key: "swa-full", Group: gMemory, Kind: kindBool,
			Title:   "Sliding window cache: compact or full?",
			About:   "This model's sliding window layers can cache only the recent window, or the whole context.",
			Tip:     "Compact saves a lot of memory but limits conversation caching. Full uses more memory.",
			pooled:  wording{tip: "Compact saves a lot of memory. Full uses more memory."},
			Options: fixed(option{"Compact (less memory)", "false"}, option{"Full", "true"}),
			Def:     text("llama.cpp's choice"),
			show:    func(f source.Facts, _ *Answers) bool { return f.SWA() },
		},
		{
			Key: "load-mode", Group: gMemory, Kind: kindChoice,
			Title: "How should the weights load?",
			About: "auto memory-maps the file when the devices support it. none reads it into memory. mlock keeps it from being swapped out.",
			Tip:   "Keep auto. Use none on multi-socket NUMA machines with MoE experts on the CPU.",
			Options: fixed(option{"auto", "auto"}, option{"mmap", "mmap"}, option{"none", "none"}, option{"mlock", "mlock"},
				option{"mmap+mlock", "mmap+mlock"}, option{"direct-io", "direct-io"}),
			Def:  text("auto"),
			Same: "auto",
			show: always,
		},
		{
			Key: "lazy-mode", Group: gMemory, Kind: kindChoice,
			Title:   "Lazy loading: read large tensors only when they are needed?",
			About:   "Auto does it for tensors over 4 GiB when the devices support memory mapping.",
			Options: fixed(option{"Auto", "auto"}, option{"No: read everything at load", "off"}, option{"Yes", "on"}),
			Def:     text("auto"),
			Same:    "auto",
			show:    always,
		},

		// Multiple GPUs

		{
			Key: "devices", Group: gGPUs, Kind: KindMulti,
			Title:   "Which GPUs should this model use?",
			About:   "Space selects. None selected uses all of them.",
			Options: gpuOptions,
			Def:     text("all"),
			show:    multiGPU,
		},
		{
			Key: "split-mode", Group: gGPUs, Kind: kindChoice,
			Title: "How should the model be split across them?",
			About: "layer works on every backend. row and tensor split inside each layer and need backend support.",
			Tip:   "Keep layer unless row or tensor measures faster on these GPUs.",
			Options: fixed(option{"layer: whole layers per GPU", "layer"}, option{"row: rows of each layer", "row"},
				option{"tensor: tensor parallel", "tensor"}, option{"none: one GPU runs the whole model", "none"}),
			Def: func(f source.Facts) string { return autoTuneStr(f.Analysis.Recommended.SplitMode) },
			show: func(f source.Facts, a *Answers) bool {
				return multiGPU(f, a) && len(a.multiValues("devices")) != 1
			},
		},
		{
			Key: "main-gpu", Group: gGPUs, Kind: kindInt, min: 0,
			Title: "Which of the GPUs runs the whole model?",
			About: "Its position among the GPUs this model uses (the ones picked above, or all): 0 is the first.",
			Def:   text("0"),
			show:  func(f source.Facts, a *Answers) bool { return multiGPU(f, a) && a.Value("split-mode") == "none" },
		},
		{
			Key: "tensor-split", Group: gGPUs, Kind: kindFloatList,
			Title: "How much of the model goes on each GPU?",
			About: "Shares in GPU order, comma separated, e.g. 0.6, 0.4.",
			Def:   text("automatic, by free memory"),
			show: func(f source.Facts, a *Answers) bool {
				split := a.Value("split-mode")
				return multiGPU(f, a) && len(a.multiValues("devices")) != 1 && (split == "" || split == "layer" || split == "row")
			},
		},

		// Throughput

		{
			Key: "queue-depth", Group: gThroughput, Kind: kindInt, min: 1,
			Title:   "How many requests may wait for each slot?",
			About:   "Requests beyond that wait to be admitted, up to the time in the next question, then fail.",
			Tip:     "One fits most servers. Each admitted request also keeps a cached conversation.",
			pooled:  wording{tip: "One fits most servers."},
			Options: fixed(option{"None: only running requests are admitted", "1"}, option{"One", "2"}, option{"Two", "3"}),
			Def:     text("one"),
			Same:    "2",
			show:    always,
		},
		{
			Key: "admission-timeout", Group: gThroughput, Kind: kindDuration, min: 1,
			Title: "How long can a request wait to be admitted?",
			About: "After that the request fails. It doesn't limit how long the answer takes.",
			Def:   text("3m"),
			show:  always,
		},
		{
			Key: "prefill-batch-size", Group: gThroughput, Kind: kindInt, min: 1,
			Title: "How many prompt tokens per processing step?",
			About: "Larger reads long prompts in fewer steps, but running requests pause longer and compute buffers grow.",
			Tip:   "Keep 2048 unless you are tuning throughput.",
			Def:   text("2048"),
			show:  always,
		},
		{
			Key: "kv-cache-mode", Group: gThroughput, Kind: kindChoice,
			Title:   "Give each request its own memory slice, or share one pool?",
			About:   "Both use the same total memory. Shared lets requests use one pool, each still within the context window.",
			Tip:     "Keep own slice unless measurements show shared helps. See chapter 4 of the manual.",
			Options: fixed(option{"Own slice per request", "partitioned"}, option{"One shared pool", "shared"}),
			Def:     text("own slice per request"),
			Same:    "partitioned",
			show:    func(f source.Facts, a *Answers) bool { return requests(f, a) > 1 },
		},
		{
			Key: "nthreads", also: []string{"nthreads-batch"}, Group: gThroughput, Kind: kindInt, min: 1,
			Title: "How many CPU threads?",
			About: "Threads for the layers or experts that run on the CPU.",
			Tip:   "Use the number of physical cores. More threads than cores is usually slower.",
			Def:   text("all CPU cores, at least 4"),
			show:  cpuWork,
		},
		{
			Key: "incremental-cache", Group: gThroughput, Kind: kindBool, generation: true,
			Title:   "Cache conversations between turns?",
			About:   "Faster for agents and long chats.",
			Tip:     "Keep yes for chat and agents.",
			Options: yesNo,
			Def:     text("yes"),
			Same:    "true",
			show:    always,
		},
		{
			Key: "cache-min-tokens", Group: gThroughput, Kind: kindInt, min: 0, generation: true,
			Title: "Minimum prompt size before caching (tokens)?",
			About: "Shorter prompts aren't cached: caching them costs more than it saves.",
			Def:   text("100"),
			show:  func(_ source.Facts, a *Answers) bool { return a.Value("incremental-cache") != "false" },
		},
		{
			Key: "imc-session-capacity", Group: gThroughput, Kind: kindInt, min: 1, generation: true,
			Title: "How many conversations to keep cached?",
			About: "At least one per admitted request: requests at the same time × (1 + requests waiting per slot).",
			Def:   text("one per admitted request"),
			show:  func(_ source.Facts, a *Answers) bool { return a.Value("incremental-cache") != "false" },
		},

		// Features

		{
			Key: "speculation", Group: gFeatures, Kind: kindChoice, generation: true,
			Title: "Speculative decoding?",
			About: "Auto uses a draft when one is available: a draft model, then an MTP head.",
			Tip:   "Speeds up answers when a draft fits in memory. Disable it when memory is tight.",
			Options: func(f source.Facts) []option {
				opts := []option{{"auto", "auto"}, {"disabled", "disabled"}}
				if len(f.Drafts) > 0 {
					opts = append(opts, option{"classic (a separate draft model)", "classic"})
				}
				if f.MTP() {
					opts = append(opts, option{"mtp (model's MTP head)", "mtp"})
				}
				return opts
			},
			Def:  text("auto"),
			Same: "auto",
			show: always,
		},
		{
			Key: "draft-model.model-id", Group: gFeatures, Kind: kindChoice, Required: true, generation: true,
			Title: "Which installed model is the draft?",
			About: "Only installed models with this model's tokenizer are listed: the server refuses a draft whose vocabulary differs. Classic needs one request at a time, so nseq-max is set to 1.",
			Tip:   "Pick the smallest one of the same family.",
			Options: func(f source.Facts) []option {
				var opts []option
				for _, id := range f.Drafts {
					opts = append(opts, option{id, id})
				}
				return opts
			},
			Def:  text(""),
			show: func(_ source.Facts, a *Answers) bool { return a.Value("speculation") == "classic" },
		},
		{
			Key: "draft-model.ndraft", Group: gFeatures, Kind: kindInt, min: 1, generation: true,
			Title: "How many draft tokens per step?",
			About: "Tokens the draft guesses before the model checks them.",
			Tip:   "3 is a good start. More helps when the draft is usually right.",
			Def:   text("3"),
			show: func(f source.Facts, a *Answers) bool {
				spec := a.Value("speculation")
				return spec == "classic" || (f.MTP() && spec != "disabled")
			},
		},
		{
			Key: "adapters", Group: gFeatures, Kind: KindAdapters,
			Title: "LoRA adapters to load?",
			About: "One per line: an adapter from the lora folder, or an absolute path. Add =scale for strength, e.g. style=0.8.",
			Def:   text("none"),
			show:  func(f source.Facts, _ *Answers) bool { return len(f.Adapters) > 0 },
		},
		{
			Key: "proj-on-cpu", Group: gFeatures, Kind: kindBool,
			Title:   "Run the projector on the CPU?",
			About:   "The projector turns {inputs} into tokens. On the CPU it frees GPU memory, but requests with {inputs} take longer.",
			Options: fixed(option{"No", "false"}, option{"Yes", "true"}),
			Def:     text("no"),
			Same:    "false",
			show:    func(f source.Facts, _ *Answers) bool { return f.Projection() },
		},
		{
			Key: "proj-device", Group: gFeatures, Kind: kindChoice,
			Title:   "Which GPU should run the projector?",
			About:   "The projector turns {inputs} into tokens.",
			Options: gpuOptions,
			Def:     text("automatic"),
			show: func(f source.Facts, a *Answers) bool {
				return f.Projection() && f.GPUs() > 1 && a.Value("proj-on-cpu") != "true"
			},
		},
		{
			Key: "image-token-budget", Group: gFeatures, Kind: kindInt, min: 1,
			Title: "Image detail budget (tokens per image)?",
			About: "Lower means less detail and less memory.",
			Def:   text("the projector's own value"),
			show:  func(f source.Facts, _ *Answers) bool { return f.Images() },
		},
	}
}

// multiGPU reports whether the machine has more than one GPU and the model
// uses them.
func multiGPU(f source.Facts, a *Answers) bool {
	return f.GPUs() > 1 && a.Value("ngpu-layers") != "-1"
}

// cpuWork reports whether some of the model runs on the CPU: no GPU, layers
// kept off the GPU, or MoE experts on the CPU.
func cpuWork(f source.Facts, a *Answers) bool {
	if f.HardwareKnown && f.GPUs() == 0 {
		return true
	}
	if layers := a.Value("ngpu-layers"); layers != "" && layers != "0" {
		return true
	}
	mode := a.Value("moe.mode")
	return f.MoE() && (mode == "experts_cpu" || mode == "keep_top_n")
}

// requests is the answered number of requests at the same time, or
// AutoTune's.
func requests(f source.Facts, a *Answers) int64 {
	if n, err := strconv.ParseInt(a.Value("nseq-max"), 10, 64); err == nil {
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

// fromModel is a default the model can set in its GGUF metadata under key,
// otherwise Kronk's own.
func fromModel(key string, kronkDefault string) func(source.Facts) string {
	return func(f source.Facts) string {
		if v := f.Info.Metadata[key]; v != "" {
			return v + " (from the model)"
		}
		return kronkDefault
	}
}
