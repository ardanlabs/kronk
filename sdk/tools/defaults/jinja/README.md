# Shipped Jinja Chat Templates

Kronk ships **no** chat templates by default. Models use the
`tokenizer.chat_template` embedded in their GGUF file.

Kronk passes reasoning/thinking content and template-specific request fields
through unchanged. Each stock GGUF template applies its native policy; callers
that want different behavior must use the fields supported by that template.

To override a model's template, drop a `<model-id>.jinja` file (or
`<model-id-without-quant-suffix>.jinja`) into `<basePath>/jinja/`. SDK callers
can set this root with `model.WithBasePath(basePath)`. The model manager passes
its base path into resolved model configurations automatically. When no base
path is supplied, discovery uses `KRONK_BASE_PATH`, then `~/.kronk`.

An explicit base path overrides the environment. If no matching file exists
under that root, the loader uses the GGUF template without searching other
roots. Explicit `JinjaScript` and `JinjaFile` overrides still take precedence.
See `retrieveTemplate` in `sdk/kronk/model/model.go`.

Any `*.jinja` file added to this directory is embedded into the binary and
seeded to `<basePath>/jinja/` at server startup by `WriteJinjaFiles`.
