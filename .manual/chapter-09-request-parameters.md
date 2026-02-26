# Chapter 9: Request Parameters

## Table of Contents

- [9.1 Sampling Parameters](#91-sampling-parameters)
- [9.2 Repetition Control](#92-repetition-control)
- [9.3 Advanced Sampling](#93-advanced-sampling)
- [9.4 Generation Control](#94-generation-control)
- [9.5 Grammar Constrained Output](#95-grammar-constrained-output)
- [9.6 Logprobs (Token Probabilities)](#96-logprobs-token-probabilities)
- [9.7 Parameter Reference](#97-parameter-reference)

---



This chapter documents the request parameters available for controlling model output through both the SDK and REST API.

### 9.1 Sampling Parameters

These parameters control the randomness and diversity of generated text.

| Parameter   | JSON Key      | Type    | Default | Description                                                                                             |
| ----------- | ------------- | ------- | ------- | ------------------------------------------------------------------------------------------------------- |
| Temperature | `temperature` | float32 | 0.8     | Controls randomness of output. Higher values produce more varied text, lower values more deterministic. |
| Top-K       | `top_k`       | int32   | 40      | Limits token pool to K most probable tokens before sampling.                                            |
| Top-P       | `top_p`       | float32 | 0.9     | Nucleus sampling threshold. Only tokens with cumulative probability ≤ top_p are considered.             |
| Min-P       | `min_p`       | float32 | 0.0     | Dynamic sampling threshold. Tokens with probability < min_p × max_probability are excluded.             |

### 9.2 Repetition Control

These parameters help prevent repetitive output.

| Parameter      | JSON Key         | Type    | Default | Description                                                                 |
| -------------- | ---------------- | ------- | ------- | --------------------------------------------------------------------------- |
| Repeat Penalty | `repeat_penalty` | float32 | 1.0     | Penalty multiplier for repeated tokens. Values > 1.0 discourage repetition. |
| Repeat Last N  | `repeat_last_n`  | int32   | 64      | Window size for repetition check. Only the last N tokens are considered.    |

**DRY Parameters (Don't Repeat Yourself):**

DRY penalizes n-gram repetitions to prevent the model from repeating phrases.

| Parameter          | JSON Key             | Type    | Default | Description                                                                 |
| ------------------ | -------------------- | ------- | ------- | --------------------------------------------------------------------------- |
| DRY Multiplier     | `dry_multiplier`     | float32 | 1.05    | N-gram repetition penalty strength. Higher values penalize repetition more. |
| DRY Base           | `dry_base`           | float32 | 1.75    | Exponential penalty base for longer n-grams.                                |
| DRY Allowed Length | `dry_allowed_length` | int32   | 2       | Minimum n-gram length to consider for penalties.                            |
| DRY Penalty Last N | `dry_penalty_last_n` | int32   | 0       | Number of recent tokens to consider for DRY. 0 means all tokens.            |

### 9.3 Advanced Sampling

**XTC (eXtreme Token Culling):**

XTC probabilistically removes high-probability tokens to increase diversity.

| Parameter       | JSON Key          | Type    | Default | Description                                                  |
| --------------- | ----------------- | ------- | ------- | ------------------------------------------------------------ |
| XTC Probability | `xtc_probability` | float32 | 0.0     | Probability of activating XTC on each token. 0 disables XTC. |
| XTC Threshold   | `xtc_threshold`   | float32 | 0.1     | Probability threshold for token culling.                     |
| XTC Min Keep    | `xtc_min_keep`    | uint32  | 1       | Minimum number of tokens to keep after culling.              |

**Adaptive-P:**

Adaptive-P dynamically adjusts the sampling threshold based on output probability.

| Parameter         | JSON Key            | Type    | Default | Description                                                 |
| ----------------- | ------------------- | ------- | ------- | ----------------------------------------------------------- |
| Adaptive-P Target | `adaptive_p_target` | float32 | 0.0     | Target probability threshold. 0 disables adaptive sampling. |
| Adaptive-P Decay  | `adaptive_p_decay`  | float32 | 0.0     | Speed of threshold adjustment toward target.                |

### 9.4 Generation Control

| Parameter        | JSON Key           | Type   | Default  | Description                                                                |
| ---------------- | ------------------ | ------ | -------- | -------------------------------------------------------------------------- |
| Max Tokens       | `max_tokens`       | int    | 4096     | Maximum tokens to generate.                                                |
| Enable Thinking  | `enable_thinking`  | string | "true"   | Enable model thinking/reasoning mode. Set to "false" for direct responses. |
| Reasoning Effort | `reasoning_effort` | string | "medium" | GPT reasoning level: none, minimal, low, medium, high.                     |
| Stream           | `stream`           | bool   | false    | Stream response chunks via SSE.                                            |
| Include Usage    | `include_usage`    | bool   | true     | Include token usage statistics in streaming responses.                     |

### 9.5 Grammar Constrained Output

Grammars force the model to only produce tokens that match a specified pattern, guaranteeing structured output.

**Built-in Presets:**

| Preset              | Description                   |
| ------------------- | ----------------------------- |
| `GrammarJSON`       | Valid JSON objects or arrays  |
| `GrammarJSONObject` | JSON objects only             |
| `GrammarJSONArray`  | JSON arrays only              |
| `GrammarBoolean`    | "true" or "false"             |
| `GrammarYesNo`      | "yes" or "no"                 |
| `GrammarInteger`    | Integer values                |
| `GrammarNumber`     | Numeric values (int or float) |

**Using Grammar Presets (SDK):**

```go
d := model.D{
    "messages": model.DocumentArray(
        model.TextMessage(model.RoleUser, "List 3 languages in JSON"),
    ),
    "grammar": model.GrammarJSONObject,
}
```

**Using Grammar via API:**

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "messages": [{"role": "user", "content": "List 3 languages in JSON"}],
    "grammar": "root ::= object\nvalue ::= object | array | string | number | \"true\" | \"false\" | \"null\"\nobject ::= \"{\" ws ( string \":\" ws value (\",\" ws string \":\" ws value)* )? ws \"}\"\narray ::= \"[\" ws ( value (\",\" ws value)* )? ws \"]\"\nstring ::= \"\\\"\" ([^\"\\\\] | \"\\\\\" [\"\\\\bfnrt/] | \"\\\\u\" [0-9a-fA-F]{4})* \"\\\"\"\nnumber ::= \"-\"? (\"0\" | [1-9][0-9]*) (\".\" [0-9]+)? ([eE] [+-]? [0-9]+)?\nws ::= [ \\t\\n\\r]*"
  }'
```

**JSON Schema Auto-Conversion:**

```go
schema := model.D{
    "type": "object",
    "properties": model.D{
        "name": model.D{"type": "string"},
        "year": model.D{"type": "integer"},
    },
    "required": []string{"name", "year"},
}

d := model.D{
    "messages": model.DocumentArray(...),
    "json_schema": schema,
    "enable_thinking": false,
}
```

Via API with `json_schema` field:

```json
{
  "model": "Qwen3-8B-Q8_0",
  "messages": [...],
  "json_schema": {
    "type": "object",
    "properties": {
      "name": {"type": "string"},
      "year": {"type": "integer"}
    },
    "required": ["name", "year"]
  },
  "enable_thinking": false
}
```

**Custom GBNF Grammars:**

```go
sentimentGrammar := `root ::= sentiment
sentiment ::= "positive" | "negative" | "neutral"`

d := model.D{
    "messages": model.DocumentArray(...),
    "grammar": sentimentGrammar,
    "enable_thinking": false,
}
```

**Important:** When using grammar constraints, set `enable_thinking: false` because the grammar applies from the first output token.

### 9.6 Logprobs (Token Probabilities)

Request log probabilities for generated tokens to understand model confidence
or implement custom sampling strategies.

**Request Parameters:**

| Parameter      | Type | Default | Description                           |
| -------------- | ---- | ------- | ------------------------------------- |
| `logprobs`     | bool | false   | Return log probability for each token |
| `top_logprobs` | int  | 0       | Number of top alternatives (0-5)      |

Setting `top_logprobs > 0` implicitly enables `logprobs`.

**Request:**

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "messages": [
      {"role": "user", "content": "What is 2+2?"}
    ],
    "logprobs": true,
    "top_logprobs": 3,
    "max_tokens": 10
  }'
```

**Response with Logprobs:**

```json
{
  "id": "chatcmpl-xxx",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "4"
      },
      "logprobs": {
        "content": [
          {
            "token": "4",
            "logprob": -0.0012,
            "bytes": [52],
            "top_logprobs": [
              { "token": "4", "logprob": -0.0012, "bytes": [52] },
              { "token": "The", "logprob": -6.82, "bytes": [84, 104, 101] },
              {
                "token": "Four",
                "logprob": -7.15,
                "bytes": [70, 111, 117, 114]
              }
            ]
          }
        ]
      },
      "finish_reason": "stop"
    }
  ]
}
```

**Response Structure:**

- `logprobs.content[]` - Array of per-token probability data
- `token` - The generated token string
- `logprob` - Log probability (always ≤ 0; closer to 0 = higher confidence)
- `bytes` - UTF-8 byte representation of the token
- `top_logprobs[]` - Alternative tokens with their probabilities

**Streaming Behavior:**

- **Streaming**: Logprobs sent in each delta chunk
- **Non-streaming**: All logprobs in final response

**Use Cases:**

- Confidence scoring for model outputs
- Detecting hallucinations (low probability sequences)
- Custom rejection sampling
- Token-level analysis for debugging

### 9.7 Parameter Reference

| Parameter          | JSON Key             | Type    | Default  | Description                          |
| ------------------ | -------------------- | ------- | -------- | ------------------------------------ |
| Temperature        | `temperature`        | float32 | 0.8      | Controls randomness of output        |
| Top-K              | `top_k`              | int32   | 40       | Limits token pool to K most probable |
| Top-P              | `top_p`              | float32 | 0.9      | Nucleus sampling threshold           |
| Min-P              | `min_p`              | float32 | 0.0      | Dynamic sampling threshold           |
| Max Tokens         | `max_tokens`         | int     | 4096     | Maximum tokens to generate           |
| Repeat Penalty     | `repeat_penalty`     | float32 | 1.0      | Penalty for repeated tokens          |
| Repeat Last N      | `repeat_last_n`      | int32   | 64       | Window for repetition check          |
| DRY Multiplier     | `dry_multiplier`     | float32 | 1.05     | N-gram repetition penalty            |
| DRY Base           | `dry_base`           | float32 | 1.75     | Exponential penalty base             |
| DRY Allowed Length | `dry_allowed_length` | int32   | 2        | Min n-gram length for DRY            |
| DRY Penalty Last N | `dry_penalty_last_n` | int32   | 0        | Recent tokens for DRY (0=all)        |
| XTC Probability    | `xtc_probability`    | float32 | 0.0      | XTC activation probability           |
| XTC Threshold      | `xtc_threshold`      | float32 | 0.1      | XTC probability threshold            |
| XTC Min Keep       | `xtc_min_keep`       | uint32  | 1        | Min tokens after XTC                 |
| Adaptive-P Target  | `adaptive_p_target`  | float32 | 0.0      | Adaptive sampling target             |
| Adaptive-P Decay   | `adaptive_p_decay`   | float32 | 0.0      | Adaptive adjustment speed            |
| Enable Thinking    | `enable_thinking`    | string  | "true"   | Enable model thinking                |
| Reasoning Effort   | `reasoning_effort`   | string  | "medium" | GPT reasoning level                  |
| Grammar            | `grammar`            | string  | ""       | GBNF grammar constraint              |
| Logprobs           | `logprobs`           | bool    | false    | Return token probabilities           |
| Top Logprobs       | `top_logprobs`       | int     | 0        | Number of top alternatives           |
| Stream             | `stream`             | bool    | false    | Stream response                      |
| Include Usage      | `include_usage`      | bool    | true     | Include usage in streaming           |
| Return Prompt      | `return_prompt`      | bool    | false    | Include prompt in response           |

---
