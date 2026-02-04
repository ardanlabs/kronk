package model

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
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
// Grammar presets for common output formats.

const (
	// GrammarJSON constrains output to valid JSON objects or arrays.
	// Based on https://github.com/ggml-org/llama.cpp/blob/master/grammars/json.gbnf
	GrammarJSON = `root ::= object | array
value ::= object | array | string | number | "true" | "false" | "null"
object ::= "{" ws ( string ":" ws value ("," ws string ":" ws value)* )? ws "}"
array ::= "[" ws ( value ("," ws value)* )? ws "]"
string ::= "\"" ([^"\\] | "\\" ["\\bfnrt/] | "\\u" [0-9a-fA-F]{4})* "\""
number ::= "-"? ("0" | [1-9][0-9]*) ("." [0-9]+)? ([eE] [+-]? [0-9]+)?
ws ::= [ \t\n\r]*`

	// GrammarJSONObject constrains output to valid JSON objects only.
	// Based on https://github.com/ggml-org/llama.cpp/blob/master/grammars/json.gbnf
	GrammarJSONObject = `root ::= object
value ::= object | array | string | number | "true" | "false" | "null"
object ::= "{" ws ( string ":" ws value ("," ws string ":" ws value)* )? ws "}"
array ::= "[" ws ( value ("," ws value)* )? ws "]"
string ::= "\"" ([^"\\] | "\\" ["\\bfnrt/] | "\\u" [0-9a-fA-F]{4})* "\""
number ::= "-"? ("0" | [1-9][0-9]*) ("." [0-9]+)? ([eE] [+-]? [0-9]+)?
ws ::= [ \t\n\r]*`

	// GrammarJSONArray constrains output to valid JSON arrays only.
	// Based on https://github.com/ggml-org/llama.cpp/blob/master/grammars/json.gbnf
	GrammarJSONArray = `root ::= array
value ::= object | array | string | number | "true" | "false" | "null"
object ::= "{" ws ( string ":" ws value ("," ws string ":" ws value)* )? ws "}"
array ::= "[" ws ( value ("," ws value)* )? ws "]"
string ::= "\"" ([^"\\] | "\\" ["\\bfnrt/] | "\\u" [0-9a-fA-F]{4})* "\""
number ::= "-"? ("0" | [1-9][0-9]*) ("." [0-9]+)? ([eE] [+-]? [0-9]+)?
ws ::= [ \t\n\r]*`

	// GrammarBoolean constrains output to "true" or "false".
	GrammarBoolean = `root ::= "true" | "false"`

	// GrammarYesNo constrains output to "yes" or "no".
	GrammarYesNo = `root ::= "yes" | "no"`

	// GrammarInteger constrains output to integer values.
	GrammarInteger = `root ::= "-"? ( "0" | [1-9][0-9]* )`

	// GrammarNumber constrains output to numeric values (int or float).
	GrammarNumber = `root ::= "-"? ( "0" | [1-9][0-9]* ) ( "." [0-9]+ )? ( [eE] [+-]? [0-9]+ )?`
)

// =============================================================================
// JSON Schema to GBNF conversion.

// fromJSONSchema converts a JSON Schema (as map or D) to a GBNF grammar string.
// It supports object, array, string, number, integer, boolean, and enum types.
func fromJSONSchema(schema any) (string, error) {
	var schemaMap map[string]any

	switch s := schema.(type) {
	case map[string]any:
		schemaMap = s

	case D:
		schemaMap = map[string]any(s)

	default:
		data, err := json.Marshal(schema)
		if err != nil {
			return "", fmt.Errorf("from-json-schema: unable to marshal schema: %w", err)
		}

		if err := json.Unmarshal(data, &schemaMap); err != nil {
			return "", fmt.Errorf("from-json-schema: unable to unmarshal schema: %w", err)
		}
	}

	gb := grammarBuilder{
		rules: make(map[string]string),
	}

	rootRule, err := gb.schemaToRule("root", schemaMap)
	if err != nil {
		return "", fmt.Errorf("from-json-schema: %w", err)
	}

	gb.rules["root"] = rootRule
	gb.addCommonRules()

	return gb.build(), nil
}

// =============================================================================
// Grammar builder for JSON Schema conversion.

type grammarBuilder struct {
	rules   map[string]string
	ruleIdx int
}

func (gb *grammarBuilder) schemaToRule(name string, schema map[string]any) (string, error) {
	schemaType, _ := schema["type"].(string)

	if enum, ok := schema["enum"].([]any); ok {
		return gb.enumToRule(enum)
	}

	switch schemaType {
	case "object":
		return gb.objectToRule(name, schema)

	case "array":
		return gb.arrayToRule(name, schema)

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

func (gb *grammarBuilder) objectToRule(name string, schema map[string]any) (string, error) {
	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		if propsD, ok := schema["properties"].(D); ok {
			props = map[string]any(propsD)
		}
	}

	if len(props) == 0 {
		return "object", nil
	}

	required := make(map[string]bool)
	if reqArr, ok := schema["required"].([]any); ok {
		for _, r := range reqArr {
			if s, ok := r.(string); ok {
				required[s] = true
			}
		}
	}

	if reqArr, ok := schema["required"].([]string); ok {
		for _, r := range reqArr {
			required[r] = true
		}
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
			continue
		}

		propRuleName := fmt.Sprintf("%s_%s", name, key)
		propRule, err := gb.schemaToRule(propRuleName, propSchema)
		if err != nil {
			return "", err
		}

		if !isBuiltinRule(propRule) {
			gb.rules[propRuleName] = propRule
			propRule = propRuleName
		}

		pair := fmt.Sprintf(`"\"" "%s" "\"" ws ":" ws %s`, key, propRule)
		if !required[key] {
			pair = fmt.Sprintf("( %s )?", pair)
		}
		pairs = append(pairs, pair)
	}

	if len(pairs) == 0 {
		return "object", nil
	}

	return fmt.Sprintf(`"{" ws %s ws "}"`, strings.Join(pairs, ` ws "," ws `)), nil
}

func (gb *grammarBuilder) arrayToRule(name string, schema map[string]any) (string, error) {
	items, _ := schema["items"].(map[string]any)
	if items == nil {
		if itemsD, ok := schema["items"].(D); ok {
			items = map[string]any(itemsD)
		}
	}

	if items == nil {
		return "array", nil
	}

	itemRuleName := fmt.Sprintf("%s_item", name)
	itemRule, err := gb.schemaToRule(itemRuleName, items)
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

	if pattern, ok := schema["pattern"].(string); ok {
		return fmt.Sprintf(`"\"" %s "\""`, pattern), nil
	}

	return "string", nil
}

func (gb *grammarBuilder) enumToRule(values []any) (string, error) {
	var options []string
	for _, v := range values {
		switch val := v.(type) {
		case string:
			options = append(options, fmt.Sprintf(`"\"" "%s" "\""`, val))

		case float64:
			if val == float64(int(val)) {
				options = append(options, fmt.Sprintf(`"%d"`, int(val)))
			} else {
				options = append(options, fmt.Sprintf(`"%v"`, val))
			}

		case bool:
			options = append(options, fmt.Sprintf(`"%t"`, val))

		default:
			options = append(options, fmt.Sprintf(`"%v"`, val))
		}
	}

	if len(options) == 0 {
		return "value", nil
	}

	return strings.Join(options, " | "), nil
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
