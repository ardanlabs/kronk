package jinja

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// cachedBuiltins is a read-only scope populated once with all built-in
// globals and filters. It is shared across every render — no code path
// writes to the builtins scope (only chained reads through parent
// links and lookupFilter), so a single shared instance is safe and
// avoids ~20 allocations per render.
var cachedBuiltins = func() *scope {
	s := newScope(nil)
	registerGlobals(s)
	registerFilters(s)
	return s
}()

// =============================================================================
// Global functions
// =============================================================================

func registerGlobals(s *scope) {
	s.set("namespace", NewCallable("namespace", builtinNamespace))
	s.set("raise_exception", NewCallable("raise_exception", builtinRaiseException))
	s.set("range", NewCallable("range", builtinRange))
	s.set("joiner", NewCallable("joiner", builtinJoiner))
	s.set("dict", NewCallable("dict", builtinDict))
	s.set("cycler", NewCallable("cycler", builtinCycler))
	s.set("strftime_now", NewCallable("strftime_now", builtinStrftimeNow))
}

func builtinNamespace(args []Value, kwargs map[string]Value) (Value, error) {
	d := NewDict()
	dict := d.AsDict()
	for k, v := range kwargs {
		dict.Set(k, v)
	}
	return d, nil
}

func builtinRaiseException(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 {
		return Undefined(), fmt.Errorf("raise_exception: missing message")
	}
	return Undefined(), fmt.Errorf("%s", printValue(args[0]))
}

func builtinRange(args []Value, kwargs map[string]Value) (Value, error) {
	var start, stop, step int64
	switch len(args) {
	case 1:
		start = 0
		stop = toInt64(args[0])
		step = 1
	case 2:
		start = toInt64(args[0])
		stop = toInt64(args[1])
		step = 1
	case 3:
		start = toInt64(args[0])
		stop = toInt64(args[1])
		step = toInt64(args[2])
	default:
		return Undefined(), fmt.Errorf("range: expected 1-3 arguments, got %d", len(args))
	}

	if step == 0 {
		return Undefined(), fmt.Errorf("range: step must not be zero")
	}

	var items []Value
	if step > 0 {
		for i := start; i < stop; i += step {
			items = append(items, NewInt(i))
		}
	} else {
		for i := start; i > stop; i += step {
			items = append(items, NewInt(i))
		}
	}

	return NewList(items), nil
}

func builtinJoiner(args []Value, kwargs map[string]Value) (Value, error) {
	sep := ""
	if len(args) > 0 {
		sep = printValue(args[0])
	}

	first := true
	return NewCallable("joiner", func(args []Value, kwargs map[string]Value) (Value, error) {
		if first {
			first = false
			return NewString(""), nil
		}
		return NewString(sep), nil
	}), nil
}

func builtinDict(args []Value, kwargs map[string]Value) (Value, error) {
	d := NewDict()
	dict := d.AsDict()
	for k, v := range kwargs {
		dict.Set(k, v)
	}
	return d, nil
}

func builtinStrftimeNow(args []Value, kwargs map[string]Value) (Value, error) {
	format := "%Y-%m-%d"
	if len(args) > 0 {
		format = printValue(args[0])
	}

	// Convert Python strftime format to Go time format.
	now := time.Now()
	goFmt := strftimeToGo(format)
	return NewString(now.Format(goFmt)), nil
}

// strftimeToGo converts a Python strftime format string to a Go time format.
func strftimeToGo(format string) string {
	r := strings.NewReplacer(
		"%Y", "2006",
		"%m", "01",
		"%d", "02",
		"%H", "15",
		"%M", "04",
		"%S", "05",
		"%B", "January",
		"%b", "Jan",
		"%A", "Monday",
		"%a", "Mon",
		"%p", "PM",
		"%I", "03",
		"%Z", "MST",
		"%%", "%",
	)
	return r.Replace(format)
}

func builtinCycler(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 {
		return Undefined(), fmt.Errorf("cycler: expected at least one argument")
	}

	idx := 0
	items := make([]Value, len(args))
	copy(items, args)

	d := NewDict()
	dict := d.AsDict()
	dict.Set("current", items[0])
	dict.Set("next", NewCallable("cycler.next", func(a []Value, kw map[string]Value) (Value, error) {
		v := items[idx]
		idx = (idx + 1) % len(items)
		dict.Set("current", items[idx])
		return v, nil
	}))
	dict.Set("reset", NewCallable("cycler.reset", func(a []Value, kw map[string]Value) (Value, error) {
		idx = 0
		dict.Set("current", items[0])
		return None(), nil
	}))

	return d, nil
}

// =============================================================================
// Filter functions
// =============================================================================

func registerFilters(s *scope) {
	filters := map[string]func([]Value, map[string]Value) (Value, error){
		"tojson":     filterTojson,
		"from_json":  filterFromjson,
		"fromjson":   filterFromjson,
		"items":      filterItems,
		"join":       filterJoin,
		"default":    filterDefault,
		"d":          filterDefault,
		"trim":       filterTrim,
		"lower":      filterLower,
		"upper":      filterUpper,
		"title":      filterTitle,
		"capitalize": filterCapitalize,
		"first":      filterFirst,
		"last":       filterLast,
		"length":     filterLength,
		"count":      filterLength,
		"reverse":    filterReverse,
		"sort":       filterSort,
		"unique":     filterUnique,
		"list":       filterList,
		"int":        filterInt,
		"float":      filterFloatConv,
		"string":     filterString,
		"safe":       filterSafe,
		"replace":    filterReplace,
		"round":      filterRound,
		"abs":        filterAbs,
		"map":        filterMap,
		"select":     filterSelect,
		"reject":     filterReject,
		"selectattr": filterSelectattr,
		"rejectattr": filterRejectattr,
		"indent":     filterIndent,
		"wordcount":  filterWordcount,
		"escape":     filterEscape,
		"e":          filterEscape,
		"dictsort":   filterDictsort,
		"max":        filterMax,
		"min":        filterMin,
		"sum":        filterSum,
		"batch":      filterBatch,
	}

	for name, fn := range filters {
		s.set("__filter_"+name, NewCallable(name, fn))
	}
}

// lookupFilter retrieves a filter callable from the scope chain.
func lookupFilter(s *scope, name string) (Value, bool) {
	return s.get("__filter_" + name)
}

// =============================================================================
// Test evaluation
// =============================================================================

func evalTest(name string, val Value, args []Value) (bool, error) {
	switch name {
	case "defined":
		return !val.IsUndefined(), nil
	case "undefined":
		return val.IsUndefined(), nil
	case "none":
		return val.IsNone(), nil
	case "boolean":
		return val.IsBool(), nil
	case "integer":
		return val.IsInt(), nil
	case "float":
		return val.IsFloat(), nil
	case "number":
		return val.IsNumber(), nil
	case "string":
		return val.IsString(), nil
	case "mapping":
		return val.IsDict(), nil
	case "iterable":
		return val.IsList() || val.IsDict() || val.IsString(), nil
	case "sequence":
		return val.IsList(), nil
	case "callable":
		return val.IsCallable(), nil
	case "true":
		return val.IsBool() && val.AsBool(), nil
	case "false":
		return val.IsBool() && !val.AsBool(), nil
	case "eq", "equalto", "==":
		if len(args) < 1 {
			return false, fmt.Errorf("test %q requires an argument", name)
		}
		return val.Equals(args[0]), nil
	case "ne", "!=":
		if len(args) < 1 {
			return false, fmt.Errorf("test %q requires an argument", name)
		}
		return !val.Equals(args[0]), nil
	case "gt", "greaterthan", ">":
		if len(args) < 1 {
			return false, nil
		}
		return compareValues(val, args[0]) > 0, nil
	case "ge", ">=":
		if len(args) < 1 {
			return false, nil
		}
		return compareValues(val, args[0]) >= 0, nil
	case "lt", "lessthan", "<":
		if len(args) < 1 {
			return false, nil
		}
		return compareValues(val, args[0]) < 0, nil
	case "le", "<=":
		if len(args) < 1 {
			return false, nil
		}
		return compareValues(val, args[0]) <= 0, nil
	case "in":
		if len(args) < 1 {
			return false, nil
		}
		return containsValue(args[0], val), nil
	case "odd":
		if val.IsInt() {
			return val.AsInt()%2 != 0, nil
		}
		return false, nil
	case "even":
		if val.IsInt() {
			return val.AsInt()%2 == 0, nil
		}
		return false, nil
	case "upper":
		if val.IsString() {
			s := val.AsString()
			return s == strings.ToUpper(s), nil
		}
		return false, nil
	case "lower":
		if val.IsString() {
			s := val.AsString()
			return s == strings.ToLower(s), nil
		}
		return false, nil
	case "sameas":
		if len(args) < 1 {
			return false, nil
		}
		return val.Equals(args[0]), nil
	}

	return false, fmt.Errorf("unknown test %q", name)
}

// =============================================================================
// Filter implementations
// =============================================================================

func filterTojson(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 {
		return NewString(""), nil
	}

	// The indented form is used rarely in chat templates; fall back to
	// the reflection-based encoder for correctness instead of duplicating
	// indent state in the direct encoder.
	if indent, ok := kwargs["indent"]; ok && !indent.IsNone() && !indent.IsUndefined() {
		n := int(toInt64(indent))
		data, err := json.MarshalIndent(valueToGo(args[0]), "", strings.Repeat(" ", n))
		if err != nil {
			return NewString(""), nil
		}
		return NewString(string(data)), nil
	}

	// Direct Value-tree encoder. Bypasses valueToGo's intermediate
	// map[string]any/[]any allocations and json.Marshal's reflection
	// path. Per-render allocations drop from ~80 (Dict.Set + valueToGo
	// + reflect) to a handful (one strings.Builder grow + one Marshal
	// per leaf string for escaping).
	var sb strings.Builder
	if err := encodeValueJSON(&sb, args[0]); err != nil {
		return NewString(""), nil
	}
	return NewString(sb.String()), nil
}

// encodeValueJSON writes the JSON encoding of v directly into sb without
// going through an intermediate Go-typed tree.
//
// The output matches Python json.dumps's defaults so that prompts rendered by
// this engine are byte-for-byte identical to those produced by HuggingFace's
// reference Jinja2 implementation:
//
//   - Item separator is ", " (with a trailing space), key/value separator is
//     ": " — Python's default when no indent is given.
//   - HTML metacharacters '&', '<', '>' are NOT escaped to \u0026 / \u003c /
//     \u003e (Go's encoding/json escapes these by default; Python does not).
//
// Both differences would otherwise change the tokenization of every tool
// definition that ends up in a chat prompt.
func encodeValueJSON(sb *strings.Builder, v Value) error {
	switch v.kind {
	case KindUndefined, KindNone, KindCallable:
		sb.WriteString("null")
		return nil

	case KindBool:
		if v.AsBool() {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}
		return nil

	case KindInt:
		var buf [20]byte
		sb.Write(strconv.AppendInt(buf[:0], v.AsInt(), 10))
		return nil

	case KindFloat:
		f := v.AsFloat()
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return fmt.Errorf("tojson: cannot encode %v", f)
		}
		var buf [32]byte
		sb.Write(strconv.AppendFloat(buf[:0], f, 'g', -1, 64))
		return nil

	case KindString:
		return encodeJSONString(sb, v.AsString())

	case KindList:
		sb.WriteByte('[')
		for i, item := range v.AsList().Items {
			if i > 0 {
				sb.WriteString(", ")
			}
			if err := encodeValueJSON(sb, item); err != nil {
				return err
			}
		}
		sb.WriteByte(']')
		return nil

	case KindDict:
		sb.WriteByte('{')
		d := v.AsDict()
		for i, key := range d.Keys {
			if i > 0 {
				sb.WriteString(", ")
			}
			if err := encodeJSONString(sb, key); err != nil {
				return err
			}
			sb.WriteString(": ")
			if err := encodeValueJSON(sb, d.Data[key]); err != nil {
				return err
			}
		}
		sb.WriteByte('}')
		return nil
	}

	sb.WriteString("null")
	return nil
}

// encodeJSONString writes the JSON encoding of s without HTML-escaping (no
// \u0026 / \u003c / \u003e), matching Python json.dumps's default behaviour.
func encodeJSONString(sb *strings.Builder, s string) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return err
	}
	// json.Encoder.Encode appends a trailing newline; strip it.
	out := buf.Bytes()
	if n := len(out); n > 0 && out[n-1] == '\n' {
		out = out[:n-1]
	}
	sb.Write(out)
	return nil
}

func filterFromjson(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 || !args[0].IsString() {
		return Undefined(), nil
	}

	var result any
	if err := json.Unmarshal([]byte(args[0].AsString()), &result); err != nil {
		return Undefined(), fmt.Errorf("fromjson: %w", err)
	}

	return FromGoValue(result), nil
}

func filterItems(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 || args[0].IsUndefined() {
		return NewList(nil), nil
	}

	// Python jinja2's |items filter raises TemplateError when applied to
	// anything that isn't a mapping, and several chat templates rely on
	// that error to bail out of an unsupported branch (e.g. when a
	// tool_call's `arguments` is a JSON string instead of an object).
	// Silently returning an empty list here let those templates render
	// invalid prompts. The wording deliberately matches Python jinja2 so
	// templates that surface this string get an identical message.
	if !args[0].IsDict() {
		//lint:ignore ST1005 message text mirrors Python jinja2 verbatim
		return Undefined(), fmt.Errorf("Can only get item pairs from a mapping.")
	}

	d := args[0].AsDict()
	items := make([]Value, 0, d.Len())
	for _, key := range d.Keys {
		pair := NewList([]Value{NewString(key), d.Data[key]})
		items = append(items, pair)
	}

	return NewList(items), nil
}

func filterJoin(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 {
		return NewString(""), nil
	}

	if !args[0].IsList() {
		return NewString(printValue(args[0])), nil
	}

	sep := ""
	if len(args) > 1 {
		sep = printValue(args[1])
	}
	if s, ok := kwargs["d"]; ok {
		sep = printValue(s)
	}

	attr := ""
	if a, ok := kwargs["attribute"]; ok {
		attr = printValue(a)
	}

	list := args[0].AsList()
	parts := make([]string, list.Len())
	for i, item := range list.Items {
		if attr != "" && item.IsDict() {
			v, ok := item.AsDict().Get(attr)
			if ok {
				item = v
			}
		}
		parts[i] = printValue(item)
	}

	return NewString(strings.Join(parts, sep)), nil
}

func filterDefault(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 {
		return Undefined(), nil
	}

	val := args[0]
	defVal := NewString("")
	if len(args) > 1 {
		defVal = args[1]
	}

	boolean := false
	if b, ok := kwargs["boolean"]; ok && b.IsBool() {
		boolean = b.AsBool()
	}

	if val.IsUndefined() {
		return defVal, nil
	}

	if boolean && !val.IsTruthy() {
		return defVal, nil
	}

	return val, nil
}

func filterTrim(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 {
		return NewString(""), nil
	}
	s := printValue(args[0])
	if len(args) > 1 {
		chars := printValue(args[1])
		s = strings.Trim(s, chars)
	} else {
		s = strings.TrimSpace(s)
	}
	return NewString(s), nil
}

func filterLower(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 {
		return NewString(""), nil
	}
	return NewString(strings.ToLower(printValue(args[0]))), nil
}

func filterUpper(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 {
		return NewString(""), nil
	}
	return NewString(strings.ToUpper(printValue(args[0]))), nil
}

func filterTitle(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 {
		return NewString(""), nil
	}

	s := printValue(args[0])
	runes := []rune(s)
	inWord := false
	for i, r := range runes {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if !inWord {
				runes[i] = unicode.ToUpper(r)
				inWord = true
			} else {
				runes[i] = unicode.ToLower(r)
			}
		} else {
			inWord = false
		}
	}
	return NewString(string(runes)), nil
}

func filterCapitalize(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 {
		return NewString(""), nil
	}
	s := printValue(args[0])
	if len(s) == 0 {
		return NewString(""), nil
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	for i := 1; i < len(runes); i++ {
		runes[i] = unicode.ToLower(runes[i])
	}
	return NewString(string(runes)), nil
}

func filterFirst(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 || !args[0].IsList() {
		return Undefined(), nil
	}
	l := args[0].AsList()
	if l.Len() == 0 {
		return Undefined(), nil
	}
	return l.Get(0), nil
}

func filterLast(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 || !args[0].IsList() {
		return Undefined(), nil
	}
	l := args[0].AsList()
	if l.Len() == 0 {
		return Undefined(), nil
	}
	return l.Get(l.Len() - 1), nil
}

func filterLength(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 {
		return NewInt(0), nil
	}

	v := args[0]
	switch {
	case v.IsList():
		return NewInt(int64(v.AsList().Len())), nil
	case v.IsDict():
		return NewInt(int64(v.AsDict().Len())), nil
	case v.IsString():
		return NewInt(int64(len([]rune(v.AsString())))), nil
	}

	return NewInt(0), nil
}

func filterReverse(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 {
		return NewList(nil), nil
	}

	v := args[0]
	if v.IsString() {
		runes := []rune(v.AsString())
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return NewString(string(runes)), nil
	}

	if v.IsList() {
		src := v.AsList().Items
		items := make([]Value, len(src))
		for i, item := range src {
			items[len(src)-1-i] = item
		}
		return NewList(items), nil
	}

	return v, nil
}

func filterSort(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 || !args[0].IsList() {
		return NewList(nil), nil
	}

	src := args[0].AsList().Items
	items := make([]Value, len(src))
	copy(items, src)

	reverse := false
	if r, ok := kwargs["reverse"]; ok && r.IsBool() {
		reverse = r.AsBool()
	}

	sort.SliceStable(items, func(i, j int) bool {
		cmp := compareValues(items[i], items[j])
		if reverse {
			return cmp > 0
		}
		return cmp < 0
	})

	return NewList(items), nil
}

func filterUnique(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 || !args[0].IsList() {
		return NewList(nil), nil
	}

	src := args[0].AsList().Items
	seen := make(map[string]bool)
	var items []Value
	for _, item := range src {
		key := printValue(item)
		if !seen[key] {
			seen[key] = true
			items = append(items, item)
		}
	}

	return NewList(items), nil
}

func filterList(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 {
		return NewList(nil), nil
	}

	v := args[0]
	if v.IsList() {
		return v, nil
	}
	if v.IsString() {
		runes := []rune(v.AsString())
		items := make([]Value, len(runes))
		for i, r := range runes {
			items[i] = NewString(string(r))
		}
		return NewList(items), nil
	}

	return NewList([]Value{v}), nil
}

func filterInt(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 {
		return NewInt(0), nil
	}
	return NewInt(toInt64(args[0])), nil
}

func filterFloatConv(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 {
		return NewFloat(0), nil
	}
	return NewFloat(toFloat64(args[0])), nil
}

func filterString(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 {
		return NewString(""), nil
	}
	return NewString(printValue(args[0])), nil
}

func filterSafe(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 {
		return NewString(""), nil
	}
	return args[0], nil
}

func filterReplace(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) < 3 {
		return NewString(""), fmt.Errorf("replace: expected at least 3 arguments")
	}
	s := printValue(args[0])
	old := printValue(args[1])
	newStr := printValue(args[2])

	count := -1
	if len(args) > 3 {
		count = int(toInt64(args[3]))
	}
	if c, ok := kwargs["count"]; ok {
		count = int(toInt64(c))
	}

	return NewString(strings.Replace(s, old, newStr, count)), nil
}

func filterRound(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 {
		return NewFloat(0), nil
	}

	precision := int64(0)
	if len(args) > 1 {
		precision = toInt64(args[1])
	}
	if p, ok := kwargs["precision"]; ok {
		precision = toInt64(p)
	}

	f := toFloat64(args[0])
	pow := math.Pow(10, float64(precision))
	rounded := math.Round(f*pow) / pow

	if precision == 0 {
		return NewFloat(rounded), nil
	}
	return NewFloat(rounded), nil
}

func filterAbs(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 {
		return NewInt(0), nil
	}

	v := args[0]
	if v.IsInt() {
		n := v.AsInt()
		if n < 0 {
			n = -n
		}
		return NewInt(n), nil
	}
	if v.IsFloat() {
		return NewFloat(math.Abs(v.AsFloat())), nil
	}

	return NewInt(0), nil
}

func filterMap(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 || !args[0].IsList() {
		return NewList(nil), nil
	}

	list := args[0].AsList()

	// map(attribute="name") form
	if attr, ok := kwargs["attribute"]; ok {
		attrName := printValue(attr)
		defVal := Undefined()
		if d, ok := kwargs["default"]; ok {
			defVal = d
		}

		items := make([]Value, list.Len())
		for i, item := range list.Items {
			if item.IsDict() {
				v, found := item.AsDict().Get(attrName)
				if found {
					items[i] = v
				} else {
					items[i] = defVal
				}
			} else {
				items[i] = defVal
			}
		}
		return NewList(items), nil
	}

	return NewList(list.Items), nil
}

func filterSelect(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 || !args[0].IsList() {
		return NewList(nil), nil
	}

	list := args[0].AsList()
	testName := ""
	if len(args) > 1 && args[1].IsString() {
		testName = args[1].AsString()
	}

	testArgs := args[2:]

	var items []Value
	for _, item := range list.Items {
		if testName == "" {
			if item.IsTruthy() {
				items = append(items, item)
			}
		} else {
			result, err := evalTest(testName, item, testArgs)
			if err != nil {
				return Undefined(), err
			}
			if result {
				items = append(items, item)
			}
		}
	}

	return NewList(items), nil
}

func filterReject(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 || !args[0].IsList() {
		return NewList(nil), nil
	}

	list := args[0].AsList()
	testName := ""
	if len(args) > 1 && args[1].IsString() {
		testName = args[1].AsString()
	}

	testArgs := args[2:]

	var items []Value
	for _, item := range list.Items {
		if testName == "" {
			if !item.IsTruthy() {
				items = append(items, item)
			}
		} else {
			result, err := evalTest(testName, item, testArgs)
			if err != nil {
				return Undefined(), err
			}
			if !result {
				items = append(items, item)
			}
		}
	}

	return NewList(items), nil
}

func filterSelectattr(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) < 2 || !args[0].IsList() {
		return NewList(nil), nil
	}

	list := args[0].AsList()
	attrName := printValue(args[1])
	testName := "truthy"
	if len(args) > 2 {
		testName = printValue(args[2])
	}

	testArgs := args[3:]

	var items []Value
	for _, item := range list.Items {
		attrVal := Undefined()
		if item.IsDict() {
			if v, ok := item.AsDict().Get(attrName); ok {
				attrVal = v
			}
		}

		if testName == "truthy" {
			if attrVal.IsTruthy() {
				items = append(items, item)
			}
		} else {
			result, err := evalTest(testName, attrVal, testArgs)
			if err != nil {
				return Undefined(), err
			}
			if result {
				items = append(items, item)
			}
		}
	}

	return NewList(items), nil
}

func filterRejectattr(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) < 2 || !args[0].IsList() {
		return NewList(nil), nil
	}

	list := args[0].AsList()
	attrName := printValue(args[1])
	testName := "truthy"
	if len(args) > 2 {
		testName = printValue(args[2])
	}

	testArgs := args[3:]

	var items []Value
	for _, item := range list.Items {
		attrVal := Undefined()
		if item.IsDict() {
			if v, ok := item.AsDict().Get(attrName); ok {
				attrVal = v
			}
		}

		if testName == "truthy" {
			if !attrVal.IsTruthy() {
				items = append(items, item)
			}
		} else {
			result, err := evalTest(testName, attrVal, testArgs)
			if err != nil {
				return Undefined(), err
			}
			if !result {
				items = append(items, item)
			}
		}
	}

	return NewList(items), nil
}

func filterIndent(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 {
		return NewString(""), nil
	}

	s := printValue(args[0])
	width := 4
	if len(args) > 1 {
		width = int(toInt64(args[1]))
	}
	if w, ok := kwargs["width"]; ok {
		width = int(toInt64(w))
	}

	indentFirst := false
	if len(args) > 2 {
		indentFirst = args[2].IsTruthy()
	}
	if f, ok := kwargs["first"]; ok {
		indentFirst = f.IsTruthy()
	}

	pad := strings.Repeat(" ", width)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if i == 0 && !indentFirst {
			continue
		}
		if line != "" {
			lines[i] = pad + line
		}
	}

	return NewString(strings.Join(lines, "\n")), nil
}

func filterWordcount(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 {
		return NewInt(0), nil
	}
	words := strings.Fields(printValue(args[0]))
	return NewInt(int64(len(words))), nil
}

func filterEscape(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 {
		return NewString(""), nil
	}
	s := printValue(args[0])
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return NewString(s), nil
}

func filterDictsort(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 || !args[0].IsDict() {
		return NewList(nil), nil
	}

	d := args[0].AsDict()
	reverse := false
	if r, ok := kwargs["reverse"]; ok && r.IsBool() {
		reverse = r.AsBool()
	}

	keys := make([]string, len(d.Keys))
	copy(keys, d.Keys)

	sort.SliceStable(keys, func(i, j int) bool {
		if reverse {
			return keys[i] > keys[j]
		}
		return keys[i] < keys[j]
	})

	items := make([]Value, len(keys))
	for i, key := range keys {
		items[i] = NewList([]Value{NewString(key), d.Data[key]})
	}

	return NewList(items), nil
}

func filterMax(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 || !args[0].IsList() {
		return Undefined(), nil
	}

	list := args[0].AsList()
	if list.Len() == 0 {
		return Undefined(), nil
	}

	maxVal := list.Items[0]
	for _, item := range list.Items[1:] {
		if compareValues(item, maxVal) > 0 {
			maxVal = item
		}
	}

	return maxVal, nil
}

func filterMin(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 || !args[0].IsList() {
		return Undefined(), nil
	}

	list := args[0].AsList()
	if list.Len() == 0 {
		return Undefined(), nil
	}

	minVal := list.Items[0]
	for _, item := range list.Items[1:] {
		if compareValues(item, minVal) < 0 {
			minVal = item
		}
	}

	return minVal, nil
}

func filterSum(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) == 0 || !args[0].IsList() {
		return NewInt(0), nil
	}

	list := args[0].AsList()
	var sumF float64
	isFloat := false

	for _, item := range list.Items {
		if item.IsFloat() {
			isFloat = true
			sumF += item.AsFloat()
		} else if item.IsInt() {
			sumF += float64(item.AsInt())
		}
	}

	if isFloat {
		return NewFloat(sumF), nil
	}
	return NewInt(int64(sumF)), nil
}

func filterBatch(args []Value, kwargs map[string]Value) (Value, error) {
	if len(args) < 2 || !args[0].IsList() {
		return NewList(nil), nil
	}

	list := args[0].AsList()
	linecount := int(toInt64(args[1]))
	if linecount <= 0 {
		return NewList(nil), nil
	}

	var batches []Value
	for i := 0; i < list.Len(); i += linecount {
		end := min(i+linecount, list.Len())
		batch := make([]Value, end-i)
		copy(batch, list.Items[i:end])
		batches = append(batches, NewList(batch))
	}

	return NewList(batches), nil
}

// =============================================================================
// Helpers
// =============================================================================

func toInt64(v Value) int64 {
	switch {
	case v.IsInt():
		return v.AsInt()
	case v.IsFloat():
		return int64(v.AsFloat())
	case v.IsBool():
		if v.AsBool() {
			return 1
		}
		return 0
	case v.IsString():
		s := v.AsString()
		var n int64
		fmt.Sscanf(s, "%d", &n)
		return n
	}
	return 0
}

func toFloat64(v Value) float64 {
	switch {
	case v.IsFloat():
		return v.AsFloat()
	case v.IsInt():
		return float64(v.AsInt())
	case v.IsBool():
		if v.AsBool() {
			return 1
		}
		return 0
	case v.IsString():
		s := v.AsString()
		var f float64
		fmt.Sscanf(s, "%f", &f)
		return f
	}
	return 0
}

func compareValues(a, b Value) int {
	af := toFloat64(a)
	bf := toFloat64(b)
	if af < bf {
		return -1
	}
	if af > bf {
		return 1
	}
	if a.IsString() && b.IsString() {
		return strings.Compare(a.AsString(), b.AsString())
	}
	return 0
}

func containsValue(container, item Value) bool {
	if container.IsList() {
		for _, v := range container.AsList().Items {
			if v.Equals(item) {
				return true
			}
		}
		return false
	}
	if container.IsDict() && item.IsString() {
		return container.AsDict().Has(item.AsString())
	}
	if container.IsString() && item.IsString() {
		return strings.Contains(container.AsString(), item.AsString())
	}
	return false
}

// valueToGo converts a Value back to a plain Go type suitable for
// json.Marshal.
func valueToGo(v Value) any {
	switch v.kind {
	case KindUndefined, KindNone:
		return nil
	case KindBool:
		return v.AsBool()
	case KindInt:
		return v.AsInt()
	case KindFloat:
		return v.AsFloat()
	case KindString:
		return v.AsString()
	case KindList:
		list := v.AsList()
		out := make([]any, list.Len())
		for i, item := range list.Items {
			out[i] = valueToGo(item)
		}
		return out
	case KindDict:
		d := v.AsDict()
		out := make(map[string]any, d.Len())
		for _, key := range d.Keys {
			out[key] = valueToGo(d.Data[key])
		}
		return out
	case KindCallable:
		return nil
	}
	return nil
}

// printValue converts a Value to its template output string. Unlike
// String() which returns a Python-style repr, printValue returns the
// raw text for template rendering (no quotes around strings, empty
// string for None/Undefined).
func printValue(v Value) string {
	switch v.kind {
	case KindUndefined:
		return ""
	case KindNone:
		return ""
	case KindBool:
		if v.AsBool() {
			return "True"
		}
		return "False"
	case KindInt:
		return fmt.Sprintf("%d", v.AsInt())
	case KindFloat:
		f := v.AsFloat()
		s := fmt.Sprintf("%g", f)
		return s
	case KindString:
		return v.AsString()
	case KindList:
		return v.String()
	case KindDict:
		return v.String()
	case KindCallable:
		return ""
	}
	return ""
}
