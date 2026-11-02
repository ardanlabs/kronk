# Branching Logic Flow

Part of `kronk-service-architecture`.

Style guidance for new or rewritten conditionals, not a rule to enforce on existing code: Kronk has many `if`/`else if` chains, including recent ones, so an existing chain is never a finding by itself. For shallow branching (1–3 cases), prefer **defaulting** or **naked switches** over if/else chains — less nesting, no duplication, lower cognitive load.

In-repo reference: `errs.FromSDK` (`cmd/server/app/sdk/errs/errs.go`) sets `code := Internal`, then overrides it in one naked `switch` of `errors.Is` cases.

**Apply when:** writing 1–3 branch conditionals, refactoring nested if/else, type-asserting on `any`/`map[string]any`, or when every branch assigns the same variable.

## Rule 1: Default first, override later

Initialize with the default, then override only in the special case.

```go
// ❌ every branch re-assigns result
var result map[string]any
if outer, ok := input["outer"].(map[string]any); ok {
    if inner, ok := outer["inner"].(map[string]any); ok {
        result = make(map[string]any, len(inner))
        maps.Copy(result, inner)
    } else {
        result = make(map[string]any)
    }
} else {
    result = make(map[string]any)
}

// ✅ default up front, override only the special case
result := make(map[string]any)
if outer, ok := input["outer"].(map[string]any); ok {
    if inner, ok := outer["inner"].(map[string]any); ok {
        result = make(map[string]any, len(inner))
        maps.Copy(result, inner)
    }
}
```

## Rule 2: Naked switch over if/else-if ladders

```go
// ❌
if a > b {
    result = a
} else if a == b {
    result = a + b
} else {
    result = b
}

// ✅
switch {
case a > b:
    result = a
case a == b:
    result = a + b
default:
    result = b
}
```

For simple two-way numeric comparison, prefer `min`/`max`: `result := max(a, b)`.

## When NOT to apply

- Default init is heavyweight (native call, model or file load, large alloc, network call).
- The "default" case is exceptional and should fail fast with an error.
- Nesting >3 levels — extract a function instead.
- Defaulting would mask a bug that should surface explicitly.

