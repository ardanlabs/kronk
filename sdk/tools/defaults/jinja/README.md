# Shipped Jinja Chat Templates

Kronk ships **no** chat templates by default. Models use the
`tokenizer.chat_template` embedded in their GGUF file.

Reasoning/thinking content is normalized in the engine (it is dropped from
assistant history when IMC is on and `preserve_thinking` is off), so the
stock GGUF templates stay stable across turns without per-model template
fixes. See `sdk/kronk/model/reasoning.go` and the `ReasoningNormalizer`
parser interface in `sdk/kronk/model/parser.go`.

To override a model's template, drop a `<model-id>.jinja` file (or
`<model-id-without-quant-suffix>.jinja`) into `~/.kronk/jinja/`. The loader
auto-discovers it; see `retrieveTemplate` in `sdk/kronk/model/model.go`.

Any `*.jinja` file added to this directory is embedded into the binary and
seeded to `~/.kronk/jinja/` at startup by `WriteJinjaFiles`.
