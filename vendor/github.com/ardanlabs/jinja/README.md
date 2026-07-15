# jinja

A pure-Go Jinja template engine purpose-built for rendering LLM chat templates. It compiles and executes the Jinja templates embedded in GGUF model files, turning conversations and tool definitions into the exact token sequences each model expects. Zero dependencies, zero CGO.

Copyright 2026 Ardan Labs

hello@ardanlabs.com

## Project Status

[![Go Reference](https://pkg.go.dev/badge/github.com/ardanlabs/jinja.svg)](https://pkg.go.dev/github.com/ardanlabs/jinja)
[![Go Report Card](https://goreportcard.com/badge/github.com/ardanlabs/jinja)](https://goreportcard.com/report/github.com/ardanlabs/jinja)
[![go.mod Go version](https://img.shields.io/github/go-mod/go-version/ardanlabs/jinja)](https://github.com/ardanlabs/jinja)
[![Linux](https://github.com/ardanlabs/jinja/actions/workflows/linux.yml/badge.svg)](https://github.com/ardanlabs/jinja/actions/workflows/linux.yml)

## Install

```
go get github.com/ardanlabs/jinja
```

## Quick Start

The API has two steps: compile a template once, then render it with data as many times as needed. Compiled templates are safe for concurrent use.

```go
package main

import (
	"fmt"
	"log"

	"github.com/ardanlabs/jinja"
)

func main() {
	tmpl, err := jinja.Compile("Hello {{ name }}!")
	if err != nil {
		log.Fatal(err)
	}

	result, err := tmpl.Render(map[string]any{
		"name": "World",
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result)
	// Output: Hello World!
}
```

## Chat Template Example

The primary use case is rendering LLM chat templates. Each model ships a Jinja template that formats conversations into the token layout the model was trained on.

```go
const chatTemplate = `{%- if messages[0].role == 'system' -%}
<|im_start|>system
{{ messages[0].content }}<|im_end|>
{%- endif %}
{%- for message in messages -%}
{%- if message.role != 'system' %}
<|im_start|>{{ message.role }}
{{ message.content }}<|im_end|>
{%- endif -%}
{%- endfor %}
{%- if add_generation_prompt %}
<|im_start|>assistant
{%- endif -%}`

tmpl, err := jinja.Compile(chatTemplate)
if err != nil {
	log.Fatal(err)
}

result, err := tmpl.Render(map[string]any{
	"messages": []any{
		map[string]any{"role": "system", "content": "You are a helpful assistant."},
		map[string]any{"role": "user", "content": "What is the capital of France?"},
		map[string]any{"role": "assistant", "content": "The capital of France is Paris."},
		map[string]any{"role": "user", "content": "What about Germany?"},
	},
	"add_generation_prompt": true,
})
```

See the [examples](examples/) directory for more, including tool calling.

## Supported Features

### Template Syntax

- Variable output: `{{ expr }}`
- Statements: `{% if %}`, `{% for %}`, `{% set %}`, `{% macro %}`, `{% block %}`
- Whitespace control: `{%- ... -%}`, `{{- ... -}}`
- Comments: `{# ... #}`
- String concatenation with `~`
- Inline if expressions: `{{ x if condition else y }}`
- Slice notation: `{{ items[::-1] }}`

### Filters

`abs` · `batch` · `capitalize` · `count` · `default` / `d` · `dictsort` · `escape` / `e` · `first` · `float` · `fromjson` · `indent` · `int` · `items` · `join` · `last` · `length` · `list` · `lower` · `map` · `max` · `min` · `reject` · `rejectattr` · `replace` · `reverse` · `round` · `safe` · `select` · `selectattr` · `sort` · `string` · `sum` · `title` · `tojson` · `trim` · `unique` · `upper` · `wordcount`

### Tests

`defined` · `undefined` · `none` · `boolean` · `integer` · `float` · `number` · `string` · `mapping` · `iterable` · `sequence` · `callable` · `true` · `false` · `odd` · `even` · `upper` · `lower` · `sameas` · `eq` · `ne` · `gt` · `ge` · `lt` · `le` · `in`

### Global Functions

`namespace` · `range` · `dict` · `joiner` · `cycler` · `raise_exception` · `strftime_now`

### String Methods

`.strip()` · `.split()` · `.startswith()` · `.endswith()` · `.upper()` · `.lower()` · `.replace()` · `.find()` · `.count()` · `.lstrip()` · `.rstrip()`

### Dict Methods

`.get()` · `.items()` · `.keys()` · `.values()` · `.update()`

## Tested Models

The test suite compiles and renders the chat templates from these models:

| Model                         | Family      |
| ----------------------------- | ----------- |
| Qwen3-8B                      | Qwen3       |
| gpt-oss-20b                   | GPT-OSS     |
| Qwen3-VL-30B-A3B-Instruct     | Qwen3-VL    |
| Qwen3.5-35B-A3B               | Qwen3.5     |
| Qwen2-Audio-7B                | Qwen2-Audio |
| gemma-4-26B-A4B-it            | Gemma 4     |
| Ministral-3-14B-Instruct-2512 | Mistral 3   |
| rnj-1-instruct                | RNJ-1       |
| LFM2.5-VL-1.6B                | LFM2.5      |

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for the full text.
