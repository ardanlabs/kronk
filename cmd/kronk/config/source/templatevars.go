package source

// The SDK has no way to list a chat template's variables (the jinja
// library keeps its parse tree unexported), so the tool reads the tags'
// tokens itself.

import (
	"slices"
	"strings"
	"unicode"
)

// templateKeywords are the Jinja words that are not variables: statements,
// operators, literals and the loop object.
var templateKeywords = map[string]bool{
	"if": true, "elif": true, "else": true, "endif": true,
	"for": true, "in": true, "endfor": true, "recursive": true, "loop": true,
	"set": true, "endset": true, "macro": true, "endmacro": true, "call": true, "endcall": true,
	"filter": true, "endfilter": true, "block": true, "endblock": true, "extends": true,
	"include": true, "import": true, "from": true, "as": true, "with": true, "without": true,
	"context": true, "ignore": true, "missing": true, "do": true, "break": true, "continue": true,
	"generation": true, "endgeneration": true, "raw": true, "endraw": true,
	"is": true, "not": true, "and": true, "or": true,
	"true": true, "false": true, "none": true, "True": true, "False": true, "None": true,
}

// templateVariables returns the names a Jinja chat template reads from its
// render context, sorted: the values Kronk, the request or
// chat_template_kwargs give it, such as messages, enable_thinking or
// reasoning_effort. It reads the tokens of the template's tags without
// running it, so a name counts when the template reads it under any
// condition. Attributes, filters, tests, function calls and names the
// template sets before it reads them are left out.
func templateVariables(script string) []string {
	// decided holds the names seen: as a variable, or as a local when their
	// first use is an assignment. A name read first, as in "if x is not
	// defined, set x", is a variable.
	decided := map[string]bool{}
	var out []string

	for _, code := range templateTags(script) {
		toks := templateTokens(code)
		targets := assignTargets(toks)

		depth := 0
		for i, t := range toks {
			switch t {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				depth--
			}

			if !isIdent(t) || templateKeywords[t] || decided[t] || targets[i] {
				continue
			}

			prev, next := tokenAt(toks, i-1), tokenAt(toks, i+1)
			switch {
			case prev == "." || prev == "|":
				// An attribute or a filter.
			case prev == "is" || (prev == "not" && tokenAt(toks, i-2) == "is"):
				// A test, such as defined or string.
			case next == "(":
				// A function: a macro, or a global such as range.
			case next == "=" && depth > 0:
				// A keyword argument.
			default:
				decided[t] = true
				out = append(out, t)
			}
		}

		// The statement's value is read before its targets are set, so
		// "set x = x | default(false)" reads x.
		for i := range targets {
			if targets[i] {
				decided[toks[i]] = true
			}
		}
	}

	slices.Sort(out)
	return out
}

// templateTags returns the code inside the {{ }} and {% %} tags of a
// template, without whitespace control marks. Comments and raw blocks are
// skipped.
func templateTags(script string) []string {
	var out []string
	for {
		start := strings.Index(script, "{")
		if start < 0 || start+1 >= len(script) {
			return out
		}

		var end string
		switch script[start+1] {
		case '{':
			end = "}}"
		case '%':
			end = "%}"
		case '#':
			end = "#}"
		default:
			script = script[start+1:]
			continue
		}

		n := strings.Index(script[start+2:], end)
		if n < 0 {
			return out
		}
		code := script[start+2 : start+2+n]
		script = script[start+2+n+2:]

		if end == "#}" {
			continue
		}

		code = strings.TrimSpace(strings.Trim(code, "-+"))
		if end == "%}" && code == "raw" {
			if i := strings.Index(script, "endraw"); i >= 0 {
				script = script[i:]
			}
			continue
		}

		out = append(out, code)
	}
}

// templateTokens splits tag code into names, numbers, punctuation and
// operators. A string literal is one token, even when it has spaces, so its
// text is never read as a name.
func templateTokens(code string) []string {
	var out []string
	for i := 0; i < len(code); {
		c := rune(code[i])
		switch {
		case unicode.IsSpace(c):
			i++

		case c == '\'' || c == '"':
			j := i + 1
			for j < len(code) && rune(code[j]) != c {
				if code[j] == '\\' {
					j++
				}
				j++
			}
			out = append(out, "''")
			i = j + 1

		case c == '_' || unicode.IsLetter(c) || unicode.IsDigit(c):
			j := i
			for j < len(code) && (code[j] == '_' || unicode.IsLetter(rune(code[j])) || unicode.IsDigit(rune(code[j]))) {
				j++
			}
			out = append(out, code[i:j])
			i = j

		default:
			if i+1 < len(code) && slices.Contains([]string{"==", "!=", "<=", ">=", "//", "**"}, code[i:i+2]) {
				out = append(out, code[i:i+2])
				i += 2
				continue
			}
			out = append(out, string(c))
			i++
		}
	}
	return out
}

// assignTargets marks the tokens a statement assigns: the names before "="
// in set (or all of them in a set block), between for and in, and the name
// and parameters of a macro.
func assignTargets(toks []string) map[int]bool {
	out := map[int]bool{}
	if len(toks) == 0 {
		return out
	}

	switch toks[0] {
	case "set":
		for i := 1; i < len(toks) && toks[i] != "="; i++ {
			out[i] = isIdent(toks[i])
		}

	case "for":
		for i := 1; i < len(toks) && toks[i] != "in"; i++ {
			out[i] = isIdent(toks[i])
		}

	case "macro":
		// macro name(a, b=default): the name and each parameter, not the
		// default values.
		param := true
		for i := 1; i < len(toks) && toks[i] != ")"; i++ {
			switch toks[i] {
			case "(", ",":
				param = true
			case "=":
				param = false
			default:
				if param && isIdent(toks[i]) {
					out[i] = true
				}
			}
		}
	}

	return out
}

func tokenAt(toks []string, i int) string {
	if i < 0 || i >= len(toks) {
		return ""
	}
	return toks[i]
}

func isIdent(t string) bool {
	return t != "" && (t[0] == '_' || unicode.IsLetter(rune(t[0])))
}
