package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"slices"
	"sort"
	"strings"

	"github.com/hybridgroup/yzma/pkg/llama"
)

var commonRules = map[string]string{
	"ws":      `[ \t\n\r]*`,
	"string":  `"\"" ( [^"\\] | "\\" ( ["\\bfnrt] | "u" [0-9a-fA-F]{4} ) )* "\""`,
	"number":  `"-"? ( "0" | [1-9][0-9]* ) ( "." [0-9]+ )? ( [eE] [+-]? [0-9]+ )?`,
	"integer": `"-"? ( "0" | [1-9][0-9]* )`,
	"boolean": `"true" | "false"`,
	"value":   `string | number | object | array | boolean | "null"`,
	"object":  `"{" ws ( pair ( ws "," ws pair )* )? ws "}"`,
	"pair":    `string ws ":" ws value`,
	"array":   `"[" ws ( value ( ws "," ws value )* )? ws "]"`,
}

var builtinRules = map[string]bool{
	"value":   true,
	"string":  true,
	"number":  true,
	"integer": true,
	"boolean": true,
	"object":  true,
	"array":   true,
}

func isBuiltinRule(rule string) bool {
	return builtinRules[rule] || strings.HasPrefix(rule, `"`)
}

// =============================================================================
// JSON Schema to GBNF conversion.

// fromJSONSchema converts a JSON Schema (as map or D) to a GBNF grammar string.
// It supports object, array, string, number, integer, boolean, and enum types.
func fromJSONSchema(schema any) (string, error) {
	var schemaMap map[string]any
	// Normalize D, typed slices, and nested maps to the JSON wire shape.
	data, err := json.Marshal(schema)
	if err != nil {
		return "", fmt.Errorf("%w: from-json-schema: marshal schema: %w", ErrInvalidRequest, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&schemaMap); err != nil {
		return "", fmt.Errorf("%w: from-json-schema: decode schema: %w", ErrInvalidRequest, err)
	}
	if schemaMap == nil {
		return "", fmt.Errorf("%w: from-json-schema: schema must be an object", ErrInvalidRequest)
	}

	gb := grammarBuilder{
		rules: make(map[string]string),
	}

	rootRule, err := gb.schemaToRule(schemaMap)
	if err != nil {
		return "", fmt.Errorf("from-json-schema: %w", err)
	}

	gb.rules["root"] = rootRule
	gb.addCommonRules()

	return gb.build(), nil
}

// fromResponseFormat converts the OpenAI-compatible "response_format" object
// into an equivalent GBNF grammar string. It supports "text" (no constraint),
// "json_object", and "json_schema" types. Returns ("", nil) when the format is
// "text" or empty.
func fromResponseFormat(rf any) (string, error) {
	var rfMap map[string]any

	switch v := rf.(type) {
	case map[string]any:
		rfMap = v

	case D:
		rfMap = map[string]any(v)

	default:
		return "", nil
	}

	formatType, _ := rfMap["type"].(string)

	switch formatType {
	case "", "text":
		return "", nil

	case "json_object":
		return fromJSONSchema(map[string]any{"type": "object"})

	case "json_schema":
		var wrapper map[string]any
		value, exists := rfMap["json_schema"]
		if !exists {
			if schema, ok := rfMap["schema"]; ok {
				return fromJSONSchema(schema)
			}
		}

		switch s := value.(type) {
		case map[string]any:
			wrapper = s

		case D:
			wrapper = map[string]any(s)

		default:
			return "", fmt.Errorf("%w: from-response-format: missing json_schema field", ErrInvalidRequest)
		}

		// OpenAI's standard wraps the schema under a "schema" key. Accept the
		// schema being passed directly as a fallback for lenient clients.
		if schema, ok := wrapper["schema"]; ok {
			return fromJSONSchema(schema)
		}

		return fromJSONSchema(wrapper)

	default:
		return "", fmt.Errorf("%w: from-response-format: unsupported type %q", ErrInvalidRequest, formatType)
	}
}

// =============================================================================
// Grammar builder for JSON Schema conversion.

type grammarBuilder struct {
	rules      map[string]string
	nextRuleID int
}

func (gb *grammarBuilder) nextRuleName() string {
	gb.nextRuleID++
	return fmt.Sprintf("rule-%d", gb.nextRuleID)
}

func (gb *grammarBuilder) schemaToRule(schema map[string]any) (string, error) {
	if err := validateGrammarSchema(schema); err != nil {
		return "", err
	}
	schemaType, _ := schema["type"].(string)

	if enum, ok := schema["enum"].([]any); ok {
		var values []any
		for _, value := range enum {
			if schemaType == "" || grammarEnumType(value, schemaType) {
				values = append(values, value)
			}
		}
		if len(values) == 0 {
			return "", fmt.Errorf("%w: enum has no values matching type", ErrInvalidRequest)
		}
		return gb.enumToRule(values)
	}

	switch schemaType {
	case "object":
		return gb.objectToRule(schema)

	case "array":
		return gb.arrayToRule(schema)

	case "string":
		return gb.stringToRule(schema)

	case "number":
		return "number", nil

	case "integer":
		return "integer", nil

	case "boolean":
		return "boolean", nil

	case "null":
		return `"null"`, nil

	default:
		return "value", nil
	}
}

// validateGrammarSchema deliberately allowlists the supported subset. Unknown
// assertions must not silently produce a grammar that ignores constraints.
// Annotation values do not affect the accepted language.
func validateGrammarSchema(schema map[string]any) error {
	schemaType, _ := schema["type"].(string)
	for key, value := range schema {
		switch key {
		case "$schema", "$id", "$comment", "title", "description", "default", "examples", "deprecated", "readOnly", "writeOnly":
		case "contentEncoding", "contentMediaType", "contentSchema":
		case "type":
			switch value {
			case "object", "array", "string", "number", "integer", "boolean", "null":
			default:
				return fmt.Errorf("%w: unsupported schema type %v", ErrInvalidRequest, value)
			}
		case "enum":
			if values, ok := value.([]any); !ok || len(values) == 0 {
				return fmt.Errorf("%w: enum must be a nonempty array", ErrInvalidRequest)
			}
		case "properties", "required", "additionalProperties":
			if schemaType != "object" {
				return fmt.Errorf("%w: %s requires object type", ErrInvalidRequest, key)
			}
			if _, ok := schema["enum"]; ok {
				return fmt.Errorf("%w: enum with object constraints is not supported", ErrInvalidRequest)
			}
			if key == "additionalProperties" {
				if _, ok := value.(bool); !ok {
					return fmt.Errorf("%w: additionalProperties must be a boolean", ErrInvalidRequest)
				}
			}
		case "items":
			if schemaType != "array" {
				return fmt.Errorf("%w: items requires array type", ErrInvalidRequest)
			}
			if _, ok := schema["enum"]; ok {
				return fmt.Errorf("%w: enum with items is not supported", ErrInvalidRequest)
			}
		default:
			return fmt.Errorf("%w: unsupported schema keyword %q", ErrInvalidRequest, key)
		}
	}
	return nil
}

func grammarEnumType(value any, schemaType string) bool {
	switch schemaType {
	case "null":
		return value == nil
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number", "integer":
		n, ok := value.(json.Number)
		if !ok {
			return false
		}
		if schemaType == "number" {
			return true
		}
		r, ok := new(big.Rat).SetString(n.String())
		return ok && r.IsInt()
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	}
	return false
}

func (gb *grammarBuilder) objectToRule(schema map[string]any) (string, error) {
	props, ok := schema["properties"].(map[string]any)
	if _, exists := schema["properties"]; exists && !ok {
		return "", fmt.Errorf("%w: properties must be an object", ErrInvalidRequest)
	}

	required := make(map[string]bool)
	if value, exists := schema["required"]; exists {
		reqArr, ok := value.([]any)
		if !ok {
			return "", fmt.Errorf("%w: required must be an array", ErrInvalidRequest)
		}
		for _, r := range reqArr {
			s, ok := r.(string)
			if !ok || required[s] {
				return "", fmt.Errorf("%w: required must contain unique strings", ErrInvalidRequest)
			}
			if _, exists := props[s]; !exists {
				return "", fmt.Errorf("%w: required property %q has no schema", ErrInvalidRequest, s)
			}
			required[s] = true
		}
	}

	if len(props) == 0 {
		if closed, ok := schema["additionalProperties"].(bool); ok && !closed {
			return `"{" ws "}"`, nil
		}
		return "object", nil
	}

	var keys []string
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var pairs []string
	for _, key := range keys {
		var propSchema map[string]any

		switch v := props[key].(type) {
		case map[string]any:
			propSchema = v
		case D:
			propSchema = map[string]any(v)
		default:
			return "", fmt.Errorf("%w: property %q must be a schema object", ErrInvalidRequest, key)
		}

		propRuleName := gb.nextRuleName()
		propRule, err := gb.schemaToRule(propSchema)
		if err != nil {
			return "", err
		}

		if !isBuiltinRule(propRule) {
			gb.rules[propRuleName] = propRule
			propRule = propRuleName
		}

		keyRule, err := jsonValueToRule(key)
		if err != nil {
			return "", fmt.Errorf("property %q: %w", key, err)
		}

		pair := fmt.Sprintf(`%s ws ":" ws %s`, keyRule, propRule)
		pairs = append(pairs, pair)
	}

	if len(required) == len(keys) {
		return fmt.Sprintf(`"{" ws %s ws "}"`, strings.Join(pairs, ` ws "," ws `)), nil
	}

	// Two suffix states track whether a property has already been emitted.
	// Only the populated state prefixes the next present property with a comma.
	first, rest := `""`, `""`
	for i, key := range slices.Backward(keys) {
		nextFirst, nextRest := gb.nextRuleName(), gb.nextRuleName()
		firstRule := pairs[i] + " " + rest
		restRule := `ws "," ws ` + pairs[i] + " " + rest
		if !required[key] {
			firstRule = "( " + firstRule + " | " + first + " )"
			// Omitting this property must still allow later properties.
			restRule = `( ws "," ws ` + pairs[i] + " )? " + rest
		}
		gb.rules[nextFirst], gb.rules[nextRest] = firstRule, restRule
		first, rest = nextFirst, nextRest
	}
	return fmt.Sprintf(`"{" ws %s ws "}"`, first), nil
}

func (gb *grammarBuilder) arrayToRule(schema map[string]any) (string, error) {
	value, exists := schema["items"]
	if !exists {
		return "array", nil
	}
	items, ok := value.(map[string]any)
	if !ok {
		return "", fmt.Errorf("%w: items must be a schema object", ErrInvalidRequest)
	}

	itemRuleName := gb.nextRuleName()
	itemRule, err := gb.schemaToRule(items)
	if err != nil {
		return "", err
	}

	if !isBuiltinRule(itemRule) {
		gb.rules[itemRuleName] = itemRule
		itemRule = itemRuleName
	}

	return fmt.Sprintf(`"[" ws ( %s ( ws "," ws %s )* )? ws "]"`, itemRule, itemRule), nil
}

func (gb *grammarBuilder) stringToRule(schema map[string]any) (string, error) {
	if enum, ok := schema["enum"].([]any); ok {
		return gb.enumToRule(enum)
	}

	if _, ok := schema["pattern"].(string); ok {
		return "", fmt.Errorf("%w: json schema pattern is not supported", ErrInvalidRequest)
	}

	return "string", nil
}

func (gb *grammarBuilder) enumToRule(values []any) (string, error) {
	var options []string
	for _, v := range values {
		option, err := jsonValueToRule(v)
		if err != nil {
			return "", fmt.Errorf("enum value: %w", err)
		}
		options = append(options, option)
	}

	if len(options) == 0 {
		return "value", nil
	}

	if len(options) == 1 {
		return options[0], nil
	}

	return fmt.Sprintf("( %s )", strings.Join(options, " | ")), nil
}

func jsonValueToRule(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal JSON value: %w", err)
	}
	return gbnfLiteral(string(data)), nil
}

func gbnfLiteral(value string) string {
	var b strings.Builder
	b.Grow(len(value) + 2)
	b.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func (gb *grammarBuilder) addCommonRules() {
	for name, rule := range commonRules {
		if _, exists := gb.rules[name]; !exists {
			gb.rules[name] = rule
		}
	}
}

func (gb *grammarBuilder) build() string {
	var b strings.Builder

	if root, ok := gb.rules["root"]; ok {
		fmt.Fprintf(&b, "root ::= %s\n", root)
	}

	var keys []string
	for k := range gb.rules {
		if k != "root" {
			keys = append(keys, k)
		}
	}

	sort.Strings(keys)

	for _, k := range keys {
		fmt.Fprintf(&b, "%s ::= %s\n", k, gb.rules[k])
	}

	return strings.TrimSpace(b.String())
}

// =============================================================================

// grammarSampler holds a separate grammar sampler that is NOT part of the
// main sampler chain. This matches llama.cpp's approach where grammar is
// managed separately and applied during sampling but with special handling
// for accept.
type grammarSampler struct {
	sampler   llama.Sampler
	nVocab    int
	tokenData []llama.TokenData // Pre-allocated buffer to avoid allocations during sampling.
}

// NewGrammarSampler creates a grammar sampler that will be managed separately
// from the main sampler chain.
func NewGrammarSampler(vocab llama.Vocab, grammar string) *grammarSampler {
	gs, _ := newGrammarSampler(vocab, grammar)
	return gs
}

func newGrammarSampler(vocab llama.Vocab, grammar string) (*grammarSampler, error) {
	if grammar == "" {
		return nil, nil
	}

	sampler := llama.SamplerInitGrammar(vocab, grammar, "root")
	if sampler == 0 {
		return nil, fmt.Errorf("%w: invalid grammar", ErrInvalidRequest)
	}

	nVocab := int(llama.VocabNTokens(vocab))

	// Pre-allocate token data buffer.
	tokenData := make([]llama.TokenData, nVocab)

	return &grammarSampler{
		sampler:   sampler,
		nVocab:    nVocab,
		tokenData: tokenData,
	}, nil
}

// SampleWithGrammar samples a token using the main sampler chain with grammar
// constraints applied first. This is the key integration point that:
// 1. Gets logits from the context
// 2. Builds a token_data_array
// 3. Applies grammar constraints (sets invalid tokens to -inf logits)
// 4. Copies modified logits back to context
// 5. Uses normal SamplerSample which reads from context
//
// SamplerSample accepts the token on the main sampler. The caller must still
// call Accept on the separate grammar sampler after selecting the token.
func (gs *grammarSampler) SampleWithGrammar(ctx llama.Context, chainSampler llama.Sampler, idx int32) llama.Token {
	if gs == nil || gs.sampler == 0 {
		return llama.SamplerSample(chainSampler, ctx, idx)
	}

	// Get logits from context.
	logits, err := llama.GetLogitsIth(ctx, idx, gs.nVocab)
	if err != nil || logits == nil {
		return llama.SamplerSample(chainSampler, ctx, idx)
	}

	// Build token_data_array from logits.
	for i := range gs.nVocab {
		gs.tokenData[i] = llama.TokenData{
			Id:    llama.Token(i),
			Logit: logits[i],
			P:     0, // Will be computed by samplers
		}
	}

	curP := llama.TokenDataArray{
		Data:     &gs.tokenData[0],
		Size:     uint64(gs.nVocab),
		Selected: -1,
		Sorted:   0, // false
	}

	// Apply grammar constraints - this sets invalid tokens to -inf logits.
	llama.SamplerApply(gs.sampler, &curP)

	// Copy modified logits back to context's logits array.
	// This allows the normal SamplerSample to work with grammar-constrained logits.
	for i := range gs.nVocab {
		logits[i] = gs.tokenData[i].Logit
	}

	// Now use normal sampling - it will read the modified logits from context.
	return llama.SamplerSample(chainSampler, ctx, idx)
}

// Accept advances the grammar state machine after a token is selected.
func (gs *grammarSampler) Accept(token llama.Token) {
	if gs == nil || gs.sampler == 0 {
		return
	}

	llama.SamplerAccept(gs.sampler, token)
}

// Free releases the grammar sampler resources.
func (gs *grammarSampler) Free() {
	if gs == nil || gs.sampler == 0 {
		return
	}

	llama.SamplerFree(gs.sampler)
	gs.sampler = 0
}

// Reset resets the grammar sampler state.
func (gs *grammarSampler) Reset() {
	if gs == nil || gs.sampler == 0 {
		return
	}

	llama.SamplerReset(gs.sampler)
}
