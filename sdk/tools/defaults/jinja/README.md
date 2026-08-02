# Shipped Jinja Chat Templates

Kronk ships **no** chat templates by default. Models use the
`tokenizer.chat_template` embedded in their GGUF file.

Kronk passes reasoning/thinking content and template-specific request fields
through unchanged. Each stock GGUF template applies its native policy; callers
that want different behavior must use the fields supported by that template.

To override a model's template, drop a `<model-id>.jinja` file (or
`<model-id-without-quant-suffix>.jinja`) into `~/.kronk/jinja/`. The loader
auto-discovers it; see `retrieveTemplate` in `sdk/kronk/model/model.go`.

Any `*.jinja` file added to this directory is embedded into the binary and
seeded to `~/.kronk/jinja/` at startup by `WriteJinjaFiles`.
