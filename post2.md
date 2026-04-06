# Taming Jinja: How Kronk Processes Chat Templates in Go

Every Large Language Model ships with a chat template—a small Jinja2 program that wraps your messages in the exact token sequences the model was trained on. Get the template wrong and the model sees gibberish. Get it right and your `system`, `user`, and `assistant` turns land in the precise format the model expects.

For **Kronk**, a high-performance Go inference engine, supporting these templates means solving a problem that nobody in the Python world even thinks about: Jinja2 was designed for Python, tested in Python, and embedded inside GGUF files as Python-flavored scripts. Running them natively in Go requires a custom execution layer and, more often than not, corrected versions of the templates themselves.

## What Chat Templates Do

A chat template converts a structured conversation—an array of role/content message pairs—into the raw token string that gets fed to the model. Different model families use radically different formats.

A Qwen-family model uses ChatML-style markers:

```
<|im_start|>system
You are a helpful assistant.<|im_end|>
<|im_start|>user
Hello!<|im_end|>
<|im_start|>assistant
```

Gemma 4 uses a turn-based format with dedicated tool and thinking tokens:

```
<bos><|turn>system
<|think|>You are a helpful assistant.
<|tool>declaration:web_search{...}<tool|>
<turn|>
<|turn>user
Hello!<turn|>
<|turn>model
```

GLM-4 uses yet another convention with `[gMASK]<sop>` headers and XML-wrapped tool calls. The template encodes all of this—role markers, tool declaration blocks, thinking/reasoning control, and the generation prompt that signals the model to start producing output.

## The GGUF Problem: Python Templates in a Go World

Every GGUF model file carries a `tokenizer.chat_template` metadata field containing the Jinja2 source. The intention is plug-and-play: load the model, extract the template, format your messages. In practice, these templates were authored and tested exclusively against Python's Jinja2 library and the Hugging Face `transformers` tokenizer.

When you move to a Go runtime, several categories of problems appear:

**Dictionary iteration behavior.** Python's `dict.items()` returns key-value tuples that unpack naturally in `for k, v in d.items()`. Go map iteration doesn't produce the same structure without explicit handling.

**Type coercion differences.** Python Jinja2 is lenient about truthy/falsy values. A string `"true"` behaves differently in Go-based Jinja evaluation than it does in Python, breaking `{% if enable_thinking %}` guards.

**Namespace scoping.** Many templates use `{% set ns = namespace(found_first=false) %}` to track state across loop iterations. This Jinja2 extension must be explicitly supported in any non-Python implementation.

**Filter availability.** Templates freely use filters like `tojson`, `fromjson`, and `dictsort` that exist in Python's Jinja2 ecosystem but aren't guaranteed in alternative implementations.

These aren't edge cases. They appear in the templates of mainstream models—Gemma, Qwen, GLM, Mistral—that Kronk users run every day.

## Template Resolution: A Three-Tier Hierarchy

Rather than blindly trusting the embedded metadata, Kronk uses a priority-based lookup when resolving which template to apply:

```
1. User-specified .jinja file  (cfg.JinjaFile)
        ↓ not set
2. Catalog template            (cataloger.RetrieveTemplate)
        ↓ not found
3. GGUF metadata fallback      (tokenizer.chat_template)
```

**Tier 1 — Local override.** If the user points to a `.jinja` file in their model configuration, Kronk reads it directly and uses it unconditionally. This gives developers complete control when experimenting with prompt formats.

**Tier 2 — The Kronk catalog.** Kronk maintains a curated catalog of corrected templates in the [kronk_catalogs](https://github.com/ardanlabs/kronk_catalogs) repository. When a model is loaded, the catalog system checks whether it has a known template for that model ID. If so, the catalog version takes precedence over the GGUF metadata.

**Tier 3 — GGUF metadata.** As a final fallback, Kronk extracts the `tokenizer.chat_template` field from the model file itself.

In code, this resolution lives in `sdk/kronk/model/model.go`:

```go
func retrieveTemplate(cataloger Cataloger, cfg Config, mdl llama.Model, modelInfo ModelInfo) (Template, error) {
    // Tier 1: user-specified file
    if cfg.JinjaFile != "" {
        data, err := readJinjaTemplate(cfg.JinjaFile)
        // ...
        return Template{FileName: cfg.JinjaFile, Script: data}, nil
    }

    // Tier 2: catalog lookup
    if cataloger != nil {
        template, err := cataloger.RetrieveTemplate(modelInfo.ID)
        if err == nil {
            return template, nil
        }
    }

    // Tier 3: GGUF metadata
    data := llama.ModelChatTemplate(mdl, "")
    if data == "" {
        data, _ = llama.ModelMetaValStr(mdl, "tokenizer.chat_template")
    }

    return Template{FileName: "tokenizer.chat_template", Script: data}, nil
}
```

This hierarchy means users get working templates out of the box, while retaining the ability to override at every level.

## Processing Templates with Gonja

Kronk executes Jinja2 templates using [Gonja](https://github.com/nikolalohinski/gonja), a Go-native Jinja2 engine. However, the stock Gonja environment doesn't match the Python Jinja2 behavior that template authors rely on. The customization work lives in `sdk/kronk/model/prompts.go`, where Kronk builds a heavily augmented execution environment.

### Compile Once, Execute Many

Templates are compiled once per model load and reused across all requests:

```go
type compiledTemplate struct {
    tmpl *exec.Template
    err  error
}

m.templateOnce.Do(func() {
    tmpl, err := newTemplateWithFixedItems(m.template.Script)
    m.compiledTmpl = &compiledTemplate{tmpl: tmpl, err: err}
})
```

This avoids repeated parsing overhead on every chat request. The `sync.Once` guard ensures thread safety for concurrent inference.

### Fixing Dictionary Iteration

The single most common compatibility issue is `dict.items()`. Python templates iterate over dictionaries with `for k, v in message.items()` and expect key-value pairs. Kronk registers a custom `items` method on the Dict type that returns `[][]any` pairs:

```go
"items": func(self map[string]any, selfValue *exec.Value, arguments *exec.VarArgs) (any, error) {
    items := make([][]any, 0, len(self))
    for key, value := range self {
        v := exec.AsValue(value).ToGoSimpleType(true)
        items = append(items, []any{key, v})
    }
    return items, nil
},
```

The `ToGoSimpleType(true)` call is critical—it converts Gonja's internal `*exec.Value` wrappers back to plain Go types, preventing reflection errors on unexported fields when the template later serializes or inspects these values.

### Custom Filters

Kronk registers several filters that Python templates take for granted:

- **`tojson`** — Marshals any value to a JSON string. Handles lists specially to avoid reflection issues with Gonja's internal types.
- **`fromjson`** — Parses a JSON string back into a Go value, enabling templates like GLM-4's that parse stringified tool arguments mid-template.
- **`items`** — Also registered as a filter (not just a method) for templates that use the `| items` pipe syntax.

### Global Functions

The execution environment injects several functions into the Jinja namespace:

- **`namespace()`** — Creates a mutable namespace object for cross-loop state tracking. Kronk's implementation unwraps `*exec.Value` to plain Go values so that assignments like `{% set ns.found_first = true %}` work correctly.
- **`strftime_now()`** — Returns the current date, used by models that incorporate temporal context in their system prompts.
- **`raise_exception()`** — Allows templates to signal errors during formatting, which surfaces as a clean error in Kronk's request pipeline.

### Parameter Normalization

Two parameters control critical template behavior and need special handling:

**`add_generation_prompt`** defaults to `true` and tells the template to append the assistant role header at the end. When building cached prefixes for Kronk's Incremental Message Cache (IMC), this is set to `false` so the cached tokens form a valid prefix that can be extended.

**`enable_thinking`** controls whether reasoning/thinking blocks are emitted. Templates like Gemma 4's check `{% if enable_thinking is defined and enable_thinking %}` to decide whether to inject `<|think|>` tokens. This value may arrive as the string `"true"` from CLI input or catalog config, so Kronk normalizes it to a real boolean:

```go
if v, ok := d["enable_thinking"]; ok {
    switch val := v.(type) {
    case string:
        d["enable_thinking"] = val == "true"
    }
}
```

### Filesystem Isolation

Templates should never access the host filesystem. Kronk registers a `noFSLoader` that rejects all read, resolve, and inherit operations:

```go
type noFSLoader struct{}

func (nl *noFSLoader) Read(path string) (io.Reader, error) {
    return nil, errors.New("filesystem access disabled")
}
```

This prevents any `{% include %}` or `{% extends %}` directives in untrusted templates from reaching the disk.

## The Catalog: Corrected Templates at Scale

The reality of GGUF-embedded templates is that they frequently need corrections to work reliably outside Python. Whitespace handling differs, filter behavior varies, and some templates use Python-specific idioms that have no direct Gonja equivalent.

Kronk addresses this through the [kronk_catalogs](https://github.com/ardanlabs/kronk_catalogs) repository, which maintains a growing collection of corrected `.jinja` template files:

```
templates/
├── gemma-3.jinja
├── gemma-4.jinja
├── glm-4.jinja
├── gpt-oss.jinja
├── lfm2.5-vl.jinja
├── ministral.jinja
├── nanbei.jinja
├── qwen3-coder.jinja
├── qwen3-next.jinja
├── qwen3.5.jinja
└── rnj-1.jinja
```

Each template in this repository has been tested against Kronk's Gonja environment and tuned for correct output. When a new model family is released, we write and test a dedicated template rather than waiting for upstream GGUF metadata to catch up.

### Automatic Sync

The catalog system automatically synchronizes templates from GitHub. It tracks SHA hashes for each template file and only downloads changes:

```go
localSHAs := t.readTemplateSHAs()

for _, item := range items {
    if localSHAs[item.Name] != item.SHA {
        files = append(files, item.DownloadURL)
    }
}
```

Templates are stored locally at `~/.kronk/templates/` and are available immediately on the next model load. The sync is resilient—if the network is unavailable or GitHub rate-limits the request, Kronk falls back to the local cache without error.

## Template Integration Points

Templates don't just format chat messages. They participate in several key subsystems:

**Tokenization API.** The `/v1/tokenize` endpoint supports an `apply_template` flag. When set, the input text is wrapped as a user message and run through the model's template before counting tokens. This gives callers an accurate count that includes all template overhead—role markers, separators, and the generation prompt.

**Message caching.** Both the System Prompt Cache (SPC) and Incremental Message Cache (IMC) use template output to build cached token sequences. The template is applied with `add_generation_prompt=false` to produce a valid prefix, then the generation prompt is added only for the final request suffix.

**Media models.** For vision and audio models, `applyRequestJinjaTemplate` extracts binary media content from messages before template application, replacing it with marker strings. The template processes the text structure normally, and the media bytes are handled separately by the multimodal pipeline.

## Why This Matters

Chat templates are the contract between your application and the model. A misformatted prompt doesn't produce an error—it produces subtly wrong output. The model might ignore your system prompt, fail to recognize tool calls, or generate malformed reasoning blocks.

By investing in a robust Jinja processing layer, a curated catalog of corrected templates, and a three-tier resolution hierarchy, Kronk ensures that the conversation reaching the model is exactly what the model expects—regardless of whether you're running Gemma, Qwen, GLM, or any other architecture.
