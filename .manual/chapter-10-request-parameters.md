# Chapter 10: Request Parameters

## Table of Contents

- [10.1 Scope and Defaults](#101-scope-and-defaults)
- [10.2 Core Sampling](#102-core-sampling)
- [10.3 Repetition Control](#103-repetition-control)
- [10.4 Advanced Sampling](#104-advanced-sampling)
- [10.5 Generation and Reasoning](#105-generation-and-reasoning)
- [10.6 Structured Output](#106-structured-output)
- [10.7 Token Log Probabilities](#107-token-log-probabilities)

---

This chapter covers generation parameters used by Chat Completions and the Go
SDK. Other API formats expose compatible subsets or translate their own field
names into these parameters. See [Chapter 9](https://www.kronkai.com/manual#chapter-9-api-endpoints) for
endpoint-specific request formats and streaming behavior.

## 10.1 Scope and Defaults

The defaults below are Kronk's baseline values. When present, a model's GGUF
sampling recommendations take precedence over those baselines. A model
configuration can provide different sampling defaults, and a request can
override them. The precedence order is request, model configuration, GGUF
recommendation, then Kronk baseline. See
[Chapter 3 §3.7](https://www.kronkai.com/manual#37-advanced-features)
for per-model `sampling-parameters`.

GGUF recommendations are used when present for `temperature`, `top_k`,
`top_p`, `min_p`, `repeat_last_n`, `xtc_probability`, and `xtc_threshold`.
Malformed or absent metadata falls back to Kronk's baseline. Model
configuration and explicit request values still win over valid metadata.

JSON requests use `number`, `integer`, `boolean`, and `string` values. The Go
SDK accepts the corresponding Go values in `model.D`.

Avoid changing several samplers at once. Start with the model's defaults,
change one parameter, and evaluate the result against representative prompts.
Parameters that improve creative prose can reduce the reliability of JSON and
tool calls.

## 10.2 Core Sampling

These parameters control how Kronk selects the next token:

| JSON key      | Type      | Baseline | Behavior |
| ------------- | --------- | -------- | -------- |
| `temperature` | number    | `0.8`    | Rescales token probabilities. Higher values generally increase variation. |
| `top_k`       | integer   | `40`     | Keeps only the K most probable candidates. |
| `top_p`       | number    | `0.9`    | Keeps the smallest candidate set whose cumulative probability reaches P. |
| `min_p`       | number    | `0.0`    | Removes candidates below `min_p × probability_of_most_likely_token`; `0` disables it. |
| `seed`        | integer   | random    | Initializes sampling randomness; values from `0` through `4294967295` request repeatable sampling. |

Explicit request values override model-specific sampling defaults, including
`top_p: 1`, which disables nucleus filtering. When `temperature`, `top_k`, or
`top_p` is omitted, Kronk uses the configured value, GGUF recommendation, or
baseline default in that order. Set `temperature: 0` explicitly to request
greedy generation. Set `top_k: 0` to disable top-k filtering.

When `seed` is omitted, Kronk chooses random sampler state for the request. An
explicit seed, including `seed: 0`, makes target sampling, classic draft
sampling, and speculative acceptance and replacement decisions repeatable.
Kronk derives independent request-local streams for the target distribution,
XTC, Adaptive-P, the classic draft distribution, and speculative decisions.
Concurrent requests therefore do not advance one another's seeded streams.
Repeatability requires the same Kronk and native-library builds, model files,
request, sampling settings, backend and devices, speculative mode, and
equivalent batching conditions. It is not guaranteed across software versions,
backends, quantizations, sampler changes, or different batching schedules.

## 10.3 Repetition Control

Kronk supports both token penalties and DRY n-gram penalties:

| JSON key             | Type    | Baseline | Behavior |
| -------------------- | ------- | -------- | -------- |
| `repeat_penalty`     | number  | `1.0`    | Multiplies penalties for tokens seen in the recent window; `1.0` disables it. |
| `repeat_last_n`      | integer | `64`     | Number of recent tokens considered by repetition penalties; `0` disables them and `-1` uses the full context. |
| `frequency_penalty`  | number  | `0.0`    | Penalizes tokens in proportion to how often they appeared. |
| `presence_penalty`   | number  | `0.0`    | Applies a flat penalty to tokens that appeared at least once. |
| `dry_multiplier`     | number  | `0.0`    | Enables DRY and controls its strength; `0` disables it. |
| `dry_base`           | number  | `1.75`   | Exponential penalty growth for longer repeated sequences. |
| `dry_allowed_length` | integer | `2`      | Minimum repeated sequence length before DRY applies. |
| `dry_penalty_last_n` | integer | `-1`     | Recent-token window for DRY; `0` disables it and `-1` uses the full context. |

The repetition and DRY samplers are disabled by default because penalties can
also suppress structural tokens needed by tool-call and JSON formats. Enable
them only after testing the selected model and template.

## 10.4 Advanced Sampling

XTC probabilistically removes likely candidates to increase diversity:

| JSON key         | Type    | Baseline | Behavior |
| ---------------- | ------- | -------- | -------- |
| `xtc_probability` | number  | `0.0`    | Probability that XTC runs for a token; `0` disables it. |
| `xtc_threshold`   | number  | `0.1`    | Probability threshold used when culling candidates. |
| `xtc_min_keep`    | integer | `1`      | Minimum candidates retained by XTC. |

Adaptive-P dynamically adjusts a probability threshold as generation
continues:

| JSON key           | Type   | Baseline | Behavior |
| ------------------ | ------ | -------- | -------- |
| `adaptive_p_target` | number | `0.0`    | Target probability; values above `0` enable Adaptive-P. |
| `adaptive_p_decay`  | number | `0.0`    | Controls how quickly the adaptive state changes. |

These samplers are specialized controls. Leave them disabled unless you can
measure an improvement for a specific workload.

## 10.5 Generation and Reasoning

| JSON key           | Type    | Baseline | Behavior |
| ------------------ | ------- | -------- | -------- |
| `max_tokens`       | integer | model-dependent | Legacy/common maximum output-token field. |
| `max_completion_tokens` | integer | model-dependent | Chat Completions output limit; takes precedence over `max_tokens`. |
| `max_output_tokens` | integer | model-dependent | Responses API output limit; takes precedence over `max_tokens`. |
| `enable_thinking`  | boolean | `true`   | Requests thinking from models and templates that support it. |
| `reasoning_effort` | string  | `medium` | Requests `none`, `minimal`, `low`, `medium`, or `high` effort from supported reasoning templates. |

If neither the request nor model configuration supplies a positive output
limit, Kronk uses the model's configured context window. The actual output can
be shorter because the prompt and generated text share that window, the model
can stop naturally, or another limit can end generation. See Chapter 9 for the
limit and termination fields returned by each API format.

Reasoning controls are model- and template-dependent. Unsupported models may
ignore them. A parser can also normalize `reasoning_effort` to values accepted
by its template; for example, a template that supports only `none` and `high`
cannot honor every intermediate value.

## 10.6 Structured Output

Kronk can convert JSON Schema to a GBNF grammar and constrain emitted tokens.
For OpenAI-compatible clients, prefer `response_format`:

```json
{
  "model": "Qwen/Qwen3-8B-Q8_0",
  "messages": [
    {"role": "user", "content": "Return a language and its year of creation."}
  ],
  "response_format": {
    "type": "json_schema",
    "json_schema": {
      "name": "language",
      "schema": {
        "type": "object",
        "properties": {
          "name": {"type": "string"},
          "year": {"type": "integer"}
        },
        "required": ["name", "year"]
      }
    }
  }
}
```

Supported `response_format.type` values are `text`, `json_object`, and
`json_schema`. Kronk also accepts a schema directly in the top-level
`json_schema` field and accepts a custom GBNF string in `grammar`. Use one
structured-output mechanism per request.

When a constraint is present and `enable_thinking` is omitted, Kronk disables
thinking automatically so free-form reasoning does not precede the structured
answer. Explicitly enabling thinking overrides that default, but is generally
counterproductive for constrained output.

A grammar restricts which tokens can be emitted; it does not guarantee a
complete result. A response cut short by `max_tokens`, context limits, or
cancellation can still contain an incomplete JSON value.

## 10.7 Token Log Probabilities

Set `logprobs: true` to return the log probability of each generated token.
`top_logprobs` requests likely alternatives and is clamped to the range 0–5.
Any positive `top_logprobs` value implicitly enables `logprobs`.

```json
{
  "model": "Qwen/Qwen3-8B-Q8_0",
  "messages": [
    {"role": "user", "content": "What is 2 + 2?"}
  ],
  "logprobs": true,
  "top_logprobs": 3,
  "max_tokens": 10
}
```

Each entry in `choices[].logprobs.content` contains the generated `token`, its
`logprob`, its UTF-8 `bytes`, and up to `top_logprobs` alternatives. Values
closer to zero were more probable under that generation step, but they are not
proof of factual correctness.

Streaming responses attach logprob data to individual delta chunks.
Non-streaming responses collect the entries in the final choice. This data is
useful for token-level diagnostics and comparative scoring; it does not alter
sampling after generation has occurred.
