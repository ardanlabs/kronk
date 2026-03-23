// Package builtins provides built-in filters, tests, control structures, and global functions.
package builtins

import (
	"fmt"
	"math"
	"math/rand"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	json "github.com/json-iterator/go"
	"github.com/pkg/errors"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/utils"
)

// Filters export all builtin filters
var Filters = exec.NewFilterSet(map[string]exec.FilterFunction{
	"abs":            filterAbs,
	"attr":           filterAttr,
	"batch":          filterBatch,
	"capitalize":     filterCapitalize,
	"center":         filterCenter,
	"default":        filterDefault,
	"d":              filterDefault,
	"dictsort":       filterDictSort,
	"e":              filterEscape,
	"escape":         filterEscape,
	"filesizeformat": filterFileSize,
	"first":          filterFirst,
	"float":          filterFloat,
	"forceescape":    filterForceEscape,
	"format":         filterFormat,
	"groupby":        filterGroupBy,
	"indent":         filterIndent,
	"int":            filterInteger,
	"join":           filterJoin,
	"last":           filterLast,
	"length":         filterLength,
	"list":           filterList,
	"lower":          filterLower,
	"map":            filterMap,
	"max":            filterMax,
	"min":            filterMin,
	"pprint":         filterPPrint,
	"random":         filterRandom,
	"rejectattr":     filterRejectAttr,
	"reject":         filterReject,
	"replace":        filterReplace,
	"reverse":        filterReverse,
	"round":          filterRound,
	"safe":           filterSafe,
	"selectattr":     filterSelectAttr,
	"select":         filterSelect,
	"slice":          filterSlice,
	"sort":           filterSort,
	"string":         filterString,
	"striptags":      filterStriptags,
	"sum":            filterSum,
	"title":          filterTitle,
	"tojson":         filterToJSON,
	"trim":           filterTrim,
	"truncate":       filterTruncate,
	"unique":         filterUnique,
	"upper":          filterUpper,
	"urlencode":      filterUrlencode,
	"urlize":         filterUrlize,
	"wordcount":      filterWordcount,
	"wordwrap":       filterWordwrap,
	"xmlattr":        filterXMLAttr,
})

func filterAbs(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	if p := params.ExpectNothing(); p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'abs'"))
	}
	if in.IsInteger() {
		asInt := in.Integer()
		if asInt < 0 {
			return exec.AsValue(-asInt)
		}
		return in
	} else if in.IsFloat() {
		return exec.AsValue(math.Abs(in.Float()))
	}
	return exec.AsValue(math.Abs(in.Float())) // nothing to do here, just to keep track of the safe application
}

func filterAttr(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	p := params.ExpectArgs(1)
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'attr'"))
	}
	attr := p.First().String()
	value, _ := in.GetAttribute(attr)
	return value
}

func filterBatch(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	p := params.Expect(1, []*exec.KwArg{{Name: "fill_with", Default: nil}})
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'batch'"))
	}
	size := p.First().Integer()
	out := make([]any, 0)
	var row []any
	in.Iterate(func(idx, count int, key, value *exec.Value) bool {
		if math.Mod(float64(idx), float64(size)) == 0 {
			if row != nil {
				out = append(out, exec.AsValue(row).Interface())
			}
			row = make([]any, 0)
		}
		row = append(row, key.Interface())
		return true
	}, func() {})
	if len(row) > 0 {
		fillWith := p.KwArgs["fill_with"]
		if !fillWith.IsNil() {
			for len(row) < size {
				row = append(row, fillWith.Interface())
			}
		}
		out = append(out, exec.AsValue(row).Interface())
	}
	return exec.AsValue(out)
}

func filterCapitalize(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	if p := params.ExpectNothing(); p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'capitalize'"))
	}

	if !in.IsString() {
		return in
	}

	if in.Len() <= 0 {
		return exec.AsValue("")
	}
	t := in.String()
	r, size := utf8.DecodeRuneInString(t)
	return exec.AsValue(strings.ToUpper(string(r)) + strings.ToLower(t[size:]))
}

func filterCenter(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	p := params.ExpectArgs(1)
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'center'"))
	}
	width := p.First().Integer()
	slen := in.Len()
	if width <= slen {
		return in
	}

	spaces := width - slen
	left := spaces/2 + spaces%2
	right := spaces / 2

	return exec.AsValue(fmt.Sprintf("%s%s%s", strings.Repeat(" ", left),
		in.String(), strings.Repeat(" ", right)))
}

func sortByKey(in *exec.Value, caseSensitive bool, reverse bool) [][2]any {
	out := make([][2]any, 0)
	in.IterateOrder(func(idx, count int, key, value *exec.Value) bool {
		out = append(out, [2]any{key.Interface(), value.Interface()})
		return true
	}, func() {}, reverse, true, caseSensitive)
	return out
}

func sortByValue(in *exec.Value, caseSensitive, reverse bool) [][2]any {
	out := make([][2]any, 0)
	items := in.Items()
	var sorter func(i, j int) bool
	switch {
	case caseSensitive && reverse:
		sorter = func(i, j int) bool {
			return items[i].Value.String() > items[j].Value.String()
		}
	case caseSensitive && !reverse:
		sorter = func(i, j int) bool {
			return items[i].Value.String() < items[j].Value.String()
		}
	case !caseSensitive && reverse:
		sorter = func(i, j int) bool {
			return strings.ToLower(items[i].Value.String()) > strings.ToLower(items[j].Value.String())
		}
	case !caseSensitive && !reverse:
		sorter = func(i, j int) bool {
			return strings.ToLower(items[i].Value.String()) < strings.ToLower(items[j].Value.String())
		}
	}
	sort.Slice(items, sorter)
	for _, item := range items {
		out = append(out, [2]any{item.Key.Interface(), item.Value.Interface()})
	}
	return out
}

func filterDictSort(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	p := params.Expect(0, []*exec.KwArg{
		{Name: "case_sensitive", Default: false},
		{Name: "by", Default: "key"},
		{Name: "reverse", Default: false},
	})
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'dictsort'"))
	}

	caseSensitive := p.KwArgs["case_sensitive"].Bool()
	by := p.KwArgs["by"].String()
	reverse := p.KwArgs["reverse"].Bool()

	switch by {
	case "key":
		return exec.AsValue(sortByKey(in, caseSensitive, reverse))
	case "value":
		return exec.AsValue(sortByValue(in, caseSensitive, reverse))
	default:
		return exec.AsValue(errors.New(`by should be either 'key' or 'value`))
	}
}

func filterEscape(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	if p := params.ExpectNothing(); p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'escape'"))
	}
	if in.Safe {
		return in
	}
	return exec.AsSafeValue(in.Escaped())
}

var (
	bytesPrefixes  = []string{"kB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"}
	binaryPrefixes = []string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB", "YiB"}
)

func filterFileSize(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	p := params.Expect(0, []*exec.KwArg{{Name: "binary", Default: false}})
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'filesizeformat'"))
	}
	bytes := in.Float()
	binary := p.KwArgs["binary"].Bool()
	var base float64
	var prefixes []string
	if binary {
		base = 1024.0
		prefixes = binaryPrefixes
	} else {
		base = 1000.0
		prefixes = bytesPrefixes
	}
	if bytes == 1.0 {
		return exec.AsValue("1 Byte")
	} else if bytes < base {
		return exec.AsValue(fmt.Sprintf("%.0f Bytes", bytes))
	} else {
		var i int
		var unit float64
		var prefix string
		for i, prefix = range prefixes {
			unit = math.Pow(base, float64(i+2))
			if bytes < unit {
				return exec.AsValue(fmt.Sprintf("%.1f %s", (base * bytes / unit), prefix))
			}
		}
		return exec.AsValue(fmt.Sprintf("%.1f %s", (base * bytes / unit), prefix))
	}
}

func filterFirst(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	if p := params.ExpectNothing(); p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'first'"))
	}
	if in.CanSlice() && in.Len() > 0 {
		return in.Index(0)
	}
	return exec.AsValue("")
}

func filterFloat(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	if p := params.ExpectNothing(); p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'float'"))
	}
	if in.IsNil() {
		return exec.AsValue(0.0)
	}
	return exec.AsValue(in.Float())
}

func filterForceEscape(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	if p := params.ExpectNothing(); p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'forceescape'"))
	}
	return exec.AsSafeValue(in.Escaped())
}

func filterFormat(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	args := []any{}
	for _, arg := range params.Args {
		args = append(args, arg.Interface())
	}
	return exec.AsValue(fmt.Sprintf(in.String(), args...))
}

func filterGroupBy(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	p := params.ExpectArgs(1)
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'groupby"))
	}
	field := p.First().String()
	groups := make(map[any][]any)
	groupers := []any{}

	in.Iterate(func(idx, count int, key, value *exec.Value) bool {
		attr, found := key.Get(field)
		if !found {
			return true
		}
		lst, exists := groups[attr.Interface()]
		if !exists {
			lst = make([]any, 0)
			groupers = append(groupers, attr.Interface())
		}
		lst = append(lst, key.Interface())
		groups[attr.Interface()] = lst
		return true
	}, func() {})

	out := make([]map[string]any, 0)
	for _, grouper := range groupers {
		out = append(out, map[string]any{
			"grouper": exec.AsValue(grouper).Interface(),
			"list":    exec.AsValue(groups[grouper]).Interface(),
		})
	}
	return exec.AsValue(out)
}

func filterIndent(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	var (
		width int
		first bool
		blank bool
	)
	if err := params.Take(
		exec.KeywordArgument("width", exec.AsValue(4), exec.IntArgument(&width)),
		exec.KeywordArgument("first", exec.AsValue(false), exec.BoolArgument(&first)),
		exec.KeywordArgument("blank", exec.AsValue(false), exec.BoolArgument(&blank)),
	); err != nil {
		return exec.AsValue(exec.ErrInvalidCall(err))
	}
	if !in.IsString() {
		return exec.AsValue(exec.ErrInvalidCall(fmt.Errorf("%s is not a string", in.String())))
	}
	indent := strings.Repeat(" ", width)
	lines := strings.Split(in.String(), "\n")
	var out strings.Builder
	for idx, line := range lines {
		if line == "" && !blank {
			out.WriteByte('\n')
			continue
		}
		if idx > 0 || first {
			out.WriteString(indent)
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return exec.AsValue(out.String())
}

func filterInteger(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	if p := params.ExpectNothing(); p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'int'"))
	}
	if in.IsNil() {
		return exec.AsValue(0)
	}
	return exec.AsValue(in.Integer())
}

func filterJoin(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	p := params.Expect(0, []*exec.KwArg{
		{Name: "d", Default: ""},
		{Name: "attribute", Default: nil},
	})
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'join'"))
	}
	if !in.CanSlice() {
		return in
	}
	sep := p.KwArgs["d"].String()
	sl := make([]string, 0, in.Len())
	for i := 0; i < in.Len(); i++ {
		sl = append(sl, in.Index(i).String())
	}
	return exec.AsValue(strings.Join(sl, sep))
}

func filterLast(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	if p := params.ExpectNothing(); p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'last'"))
	}
	if in.CanSlice() && in.Len() > 0 {
		return in.Index(in.Len() - 1)
	}
	return exec.AsValue("")
}

func filterLength(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	if p := params.ExpectNothing(); p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'length'"))
	}
	return exec.AsValue(in.Len())
}

func filterList(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	if p := params.ExpectNothing(); p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'list'"))
	}
	if in.IsString() {
		out := []string{}
		for _, r := range in.String() {
			out = append(out, string(r))
		}
		return exec.AsValue(out)
	}
	out := make([]any, 0)
	in.Iterate(func(idx, count int, key, value *exec.Value) bool {
		out = append(out, key.Interface())
		return true
	}, func() {})
	return exec.AsValue(out)
}

func filterLower(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	if p := params.ExpectNothing(); p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'lower'"))
	}
	return exec.AsValue(strings.ToLower(in.String()))
}

func filterMap(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	p := params.Expect(0, []*exec.KwArg{
		{Name: "filter", Default: ""},
		{Name: "attribute", Default: nil},
		{Name: "default", Default: nil},
	})
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'map'"))
	}
	filter := p.KwArgs["filter"].String()
	attribute := p.KwArgs["attribute"].String()
	defaultVal := p.KwArgs["default"]
	out := make([]any, 0)
	in.Iterate(func(idx, count int, key, value *exec.Value) bool {
		val := key
		if len(attribute) > 0 {
			attr, found := val.Get(attribute)
			if found {
				val = attr
			} else if defaultVal != nil {
				val = defaultVal
			} else {
				return true
			}
		}
		if len(filter) > 0 {
			val = e.ExecuteFilterByName(filter, val, exec.NewVarArgs())
		}
		out = append(out, val.Interface())
		return true
	}, func() {})
	return exec.AsValue(out)
}

func filterMax(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	p := params.Expect(0, []*exec.KwArg{
		{Name: "case_sensitive", Default: false},
		{Name: "attribute", Default: nil},
	})
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'max'"))
	}
	caseSensitive := p.KwArgs["case_sensitive"].Bool()
	attribute := p.KwArgs["attribute"].String()

	var max *exec.Value
	in.Iterate(func(idx, count int, key, value *exec.Value) bool {
		val := key
		if len(attribute) > 0 {
			attr, found := val.Get(attribute)
			if found {
				val = attr
			} else {
				val = nil
			}
		}
		if max == nil {
			max = val
			return true
		}
		if val == nil || max == nil {
			return true
		}
		switch {
		case max.IsFloat() || max.IsInteger() && val.IsFloat() || val.IsInteger():
			if val.Float() > max.Float() {
				max = val
			}
		case max.IsString() && val.IsString():
			if !caseSensitive && strings.ToLower(val.String()) > strings.ToLower(max.String()) {
				max = val
			} else if caseSensitive && val.String() > max.String() {
				max = val
			}
		default:
			max = exec.AsValue(errors.Errorf(`%s and %s are not comparable`, max.Val.Type(), val.Val.Type()))
		}
		return true
	}, func() {})

	if max == nil {
		return exec.AsValue("")
	}
	return max
}

func filterMin(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	p := params.Expect(0, []*exec.KwArg{
		{Name: "case_sensitive", Default: false},
		{Name: "attribute", Default: nil},
	})
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'min'"))
	}
	caseSensitive := p.KwArgs["case_sensitive"].Bool()
	attribute := p.KwArgs["attribute"].String()

	var min *exec.Value
	in.Iterate(func(idx, count int, key, value *exec.Value) bool {
		val := key
		if len(attribute) > 0 {
			attr, found := val.Get(attribute)
			if found {
				val = attr
			} else {
				val = nil
			}
		}
		if min == nil {
			min = val
			return true
		}
		if val == nil || min == nil {
			return true
		}
		switch {
		case min.IsFloat() || min.IsInteger() && val.IsFloat() || val.IsInteger():
			if val.Float() < min.Float() {
				min = val
			}
		case min.IsString() && val.IsString():
			if !caseSensitive && strings.ToLower(val.String()) < strings.ToLower(min.String()) {
				min = val
			} else if caseSensitive && val.String() < min.String() {
				min = val
			}
		default:
			min = exec.AsValue(errors.Errorf(`%s and %s are not comparable`, min.Val.Type(), val.Val.Type()))
		}
		return true
	}, func() {})

	if min == nil {
		return exec.AsValue("")
	}
	return min
}

func filterPPrint(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	p := params.Expect(0, []*exec.KwArg{{Name: "verbose", Default: false}})
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'pprint'"))
	}
	b, err := json.MarshalIndent(in.Interface(), "", "  ")
	if err != nil {
		return exec.AsValue(errors.Wrapf(err, `Unable to pretty print '%s'`, in.String()))
	}
	return exec.AsSafeValue(string(b))
}

func filterRandom(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	if p := params.ExpectNothing(); p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'random'"))
	}
	if !in.CanSlice() || in.Len() <= 0 {
		return in
	}
	i := rand.Intn(in.Len())
	return in.Index(i)
}

func filterReject(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	var test func(*exec.Value) bool
	if len(params.Args) == 0 {
		// Reject truthy value
		test = func(in *exec.Value) bool {
			return in.IsTrue()
		}
	} else {
		name := params.First().String()
		testParams := &exec.VarArgs{
			Args:   params.Args[1:],
			KwArgs: params.KwArgs,
		}
		test = func(in *exec.Value) bool {
			out := e.ExecuteTestByName(name, in, testParams)
			return out.IsTrue()
		}
	}

	out := make([]any, 0)

	in.Iterate(func(idx, count int, key, value *exec.Value) bool {
		if !test(key) {
			out = append(out, key.Interface())
		}
		return true
	}, func() {})

	return exec.AsValue(out)
}

func filterRejectAttr(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	var test func(*exec.Value) *exec.Value
	if len(params.Args) < 1 {
		return exec.AsValue(errors.New("Wrong signature for 'rejectattr', expect at least an attribute name as argument"))
	}
	attribute := params.First().String()
	if len(params.Args) == 1 {
		// Reject truthy value
		test = func(in *exec.Value) *exec.Value {
			attr, found := in.Get(attribute)
			if !found {
				return exec.AsValue(errors.Errorf(`%s has no attribute '%s'`, in.String(), attribute))
			}
			return attr
		}
	} else {
		name := params.Args[1].String()
		testParams := &exec.VarArgs{
			Args:   params.Args[2:],
			KwArgs: params.KwArgs,
		}
		test = func(in *exec.Value) *exec.Value {
			attr, found := in.Get(attribute)
			if !found {
				return exec.AsValue(errors.Errorf(`%s has no attribute '%s'`, in.String(), attribute))
			}
			out := e.ExecuteTestByName(name, attr, testParams)
			return out
		}
	}

	out := make([]any, 0)
	var err *exec.Value

	in.Iterate(func(idx, count int, key, value *exec.Value) bool {
		result := test(key)
		if result.IsError() {
			err = result
			return false
		}
		if !result.IsTrue() {
			out = append(out, key.Interface())
		}
		return true
	}, func() {})

	if err != nil {
		return err
	}
	return exec.AsValue(out)
}

func filterReplace(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	p := params.Expect(2, []*exec.KwArg{{Name: "count", Default: nil}})
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'replace'"))
	}
	old := p.Args[0].String()
	new := p.Args[1].String()
	count := p.KwArgs["count"]
	if count.IsNil() {
		return exec.AsValue(strings.ReplaceAll(in.String(), old, new))
	}
	return exec.AsValue(strings.Replace(in.String(), old, new, count.Integer()))
}

func filterReverse(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	if p := params.ExpectNothing(); p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'safe'"))
	}
	if in.IsString() {
		var out strings.Builder
		in.IterateOrder(func(idx, count int, key, value *exec.Value) bool {
			out.WriteString(key.String())
			return true
		}, func() {}, true, false, false)
		return exec.AsValue(out.String())
	}
	out := make([]any, 0)
	in.IterateOrder(func(idx, count int, key, value *exec.Value) bool {
		out = append(out, key.Interface())
		return true
	}, func() {}, true, true, false)
	return exec.AsValue(out)
}

func filterRound(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	p := params.Expect(0, []*exec.KwArg{{Name: "precision", Default: 0}, {Name: "method", Default: "common"}})
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'round'"))
	}
	method := p.KwArgs["method"].String()
	var op func(float64) float64
	switch method {
	case "common":
		op = math.Round
	case "floor":
		op = math.Floor
	case "ceil":
		op = math.Ceil
	default:
		return exec.AsValue(errors.Errorf(`Unknown method '%s', mush be one of 'common, 'floor', 'ceil`, method))
	}
	value := in.Float()
	factor := float64(10 * p.KwArgs["precision"].Integer())
	if factor > 0 {
		value = value * factor
	}
	value = op(value)
	if factor > 0 {
		value = value / factor
	}
	return exec.AsValue(value)
}

func filterSafe(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	if p := params.ExpectNothing(); p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'safe'"))
	}
	in.Safe = true
	return in // nothing to do here, just to keep track of the safe application
}

func filterSelect(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	var test func(*exec.Value) bool
	if len(params.Args) == 0 {
		// Reject truthy value
		test = func(in *exec.Value) bool {
			return in.IsTrue()
		}
	} else {
		name := params.First().String()
		testParams := &exec.VarArgs{
			Args:   params.Args[1:],
			KwArgs: params.KwArgs,
		}
		test = func(in *exec.Value) bool {
			out := e.ExecuteTestByName(name, in, testParams)
			return out.IsTrue()
		}
	}

	out := make([]any, 0)

	in.Iterate(func(idx, count int, key, value *exec.Value) bool {
		if test(key) {
			out = append(out, key.Interface())
		}
		return true
	}, func() {})

	return exec.AsValue(out)
}

func filterSlice(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	var (
		slices   int
		fillWith any
	)
	if err := params.Take(
		exec.PositionalArgument("slices", nil, exec.IntArgument(&slices)),
		exec.KeywordArgument("fill_with", exec.AsValue(nil), exec.AnyArgument(&fillWith)),
	); err != nil {
		return exec.AsValue(exec.ErrInvalidCall(err))
	}
	if slices < 1 {
		return exec.AsValue(exec.ErrInvalidCall(fmt.Errorf("slices argument %d must be > 0", slices)))
	}
	if !in.IsList() {
		return exec.AsValue(exec.ErrInvalidCall(fmt.Errorf("%s is not a list", in.String())))
	}
	quotient := int(math.Ceil(float64(in.Len()) / float64(slices)))
	remainder := in.Len() % slices
	output := make([]any, 0)
	in.Iterate(func(index, _ int, value, _ *exec.Value) bool {
		if index%quotient == 0 {
			output = append(output, []any{value.Interface()})
		} else {
			output[len(output)-1] = append(output[len(output)-1].([]any), value.Interface())
		}
		return true
	}, func() {})
	if remainder > 0 && fillWith != nil {
		for len(output[len(output)-1].([]any)) < quotient {
			output[len(output)-1] = append(output[len(output)-1].([]any), fillWith)
		}
	}
	return exec.AsValue(output)
}

func filterSort(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	p := params.Expect(0, []*exec.KwArg{{Name: "reverse", Default: false}, {Name: "case_sensitive", Default: false}})
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'sort'"))
	}
	reverse := p.KwArgs["reverse"].Bool()
	caseSensitive := p.KwArgs["case_sensitive"].Bool()
	out := make([]any, 0)
	in.IterateOrder(func(idx, count int, key, value *exec.Value) bool {
		out = append(out, key.Interface())
		return true
	}, func() {}, reverse, true, caseSensitive)
	return exec.AsValue(out)
}

func filterString(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	if p := params.ExpectNothing(); p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'string'"))
	}
	return exec.AsValue(in.String())
}

var reStriptags = regexp.MustCompile("<[^>]*?>")

func filterStriptags(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	if p := params.ExpectNothing(); p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'striptags'"))
	}
	s := in.String()

	// Strip all tags
	s = reStriptags.ReplaceAllString(s, "")

	return exec.AsValue(strings.TrimSpace(s))
}

func filterSum(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	p := params.Expect(0, []*exec.KwArg{{Name: "attribute", Default: nil}, {Name: "start", Default: 0}})
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'sum'"))
	}

	attribute := p.KwArgs["attribute"]
	sum := p.KwArgs["start"].Float()
	var err error

	in.Iterate(func(idx, count int, key, value *exec.Value) bool {
		if attribute.IsString() {
			val := key
			found := true
			for attr := range strings.SplitSeq(attribute.String(), ".") {
				val, found = val.Get(attr)
				if !found {
					err = errors.Errorf("'%s' has no attribute '%s'", key.String(), attribute.String())
					return false
				}
			}
			if found && val.IsNumber() {
				sum += val.Float()
			}
		} else if attribute.IsInteger() {
			value, found := key.GetItem(attribute.Integer())
			if found {
				sum += value.Float()
			}
		} else {
			sum += key.Float()
		}
		return true
	}, func() {})

	if err != nil {
		return exec.AsValue(err)
	} else if sum == math.Trunc(sum) {
		return exec.AsValue(int64(sum))
	}
	return exec.AsValue(sum)
}

func filterTitle(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	if p := params.ExpectNothing(); p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'title'"))
	}
	if !in.IsString() {
		return exec.AsValue("")
	}
	return exec.AsValue(cases.Title(language.English).String(strings.ToLower(in.String())))
}

func filterTrim(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	charsParam := exec.KwArg{
		Name:    "chars",
		Default: " ",
	}
	p := params.ExpectKwArgs([]*exec.KwArg{&charsParam})
	if p.IsError() || !in.IsString() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'trim'"))
	}
	chars := p.GetKeywordArgument(charsParam.Name, charsParam.Default).String()
	return exec.AsValue(strings.Trim(in.String(), chars))
}

func filterToJSON(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	// Done not mess around with trying to marshall error pipelines
	if in.IsError() {
		return in
	}

	// Monkey patching because arrays handling is broken
	if in.IsList() {
		inCast := make([]any, in.Len())
		for index := range inCast {
			item := exec.ToValue(in.Index(index).Val)
			inCast[index] = item.Val.Interface()
		}
		in = exec.AsValue(inCast)
	}

	p := params.Expect(0, []*exec.KwArg{
		{Name: "indent", Default: nil},
		{Name: "ensure_ascii", Default: exec.AsValue(true)}, // Accepted for compatibility, ignored (Go handles unicode)
	})
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'tojson'"))
	}

	casted := in.ToGoSimpleType(true)
	if err, ok := casted.(error); ok {
		return exec.AsValue(err)
	}

	indent := p.KwArgs["indent"]
	var out string
	if indent.IsNil() {
		b, err := json.ConfigCompatibleWithStandardLibrary.Marshal(casted)
		if err != nil {
			return exec.AsValue(errors.Wrap(err, "Unable to marhsall to json"))
		}
		out = string(b)
	} else if indent.IsInteger() {
		b, err := json.ConfigCompatibleWithStandardLibrary.MarshalIndent(casted, "", strings.Repeat(" ", indent.Integer()))
		if err != nil {
			return exec.AsValue(errors.Wrap(err, "Unable to marhsall to json"))
		}
		out = string(b)
	} else {
		return exec.AsValue(errors.Errorf("Expected an integer for 'indent', got %s", indent.String()))
	}
	return exec.AsSafeValue(out)
}

func filterTruncate(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	p := params.Expect(0, []*exec.KwArg{
		{Name: "length", Default: 255},
		{Name: "killwords", Default: false},
		{Name: "end", Default: "..."},
		{Name: "leeway", Default: 0},
	})
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'truncate'"))
	}

	source := in.String()
	length := p.KwArgs["length"].Integer()
	leeway := p.KwArgs["leeway"].Integer()
	killwords := p.KwArgs["killwords"].Bool()
	end := p.KwArgs["end"].String()
	rEnd := []rune(end)
	fullLength := length + leeway
	runes := []rune(source)

	if length < len(rEnd) {
		return exec.AsValue(errors.Errorf(`expected length >= %d, got %d`, len(rEnd), length))
	}

	if len(runes) <= fullLength {
		return exec.AsValue(source)
	}

	atLength := string(runes[:length-len(rEnd)])
	if !killwords {
		atLength = strings.TrimRightFunc(atLength, func(r rune) bool {
			return !unicode.IsSpace(r)
		})
		atLength = strings.TrimRight(atLength, " \n\t")
	}
	return exec.AsValue(fmt.Sprintf("%s%s", atLength, end))
}

func filterUnique(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	p := params.Expect(0, []*exec.KwArg{{Name: "case_sensitive", Default: false}, {Name: "attribute", Default: nil}})
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'unique'"))
	}

	caseSensitive := p.KwArgs["case_sensitive"].Bool()
	attribute := p.KwArgs["attribute"]

	out := make([]any, 0)
	tracker := map[any]bool{}
	var err error

	in.Iterate(func(idx, count int, key, value *exec.Value) bool {
		val := key
		if attribute.IsString() {
			attr := attribute.String()
			nested, found := key.Get(attr)
			if !found {
				err = errors.Errorf(`%s has no attribute %s`, key.String(), attr)
				return false
			}
			val = nested
		}
		tracked := val.Interface()
		if !caseSensitive && val.IsString() {
			tracked = strings.ToLower(val.String())
		}
		if _, contains := tracker[tracked]; !contains {
			tracker[tracked] = true
			out = append(out, key.Interface())
		}
		return true
	}, func() {})

	if err != nil {
		return exec.AsValue(err)
	}
	return exec.AsValue(out)
}

func filterUpper(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	if p := params.ExpectNothing(); p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'upper'"))
	}
	return exec.AsValue(strings.ToUpper(in.String()))
}

func filterUrlencode(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	if p := params.ExpectNothing(); p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'urlencode'"))
	}
	return exec.AsValue(url.QueryEscape(in.String()))
}

// TODO: This regexp could do some work
var filterUrlizeURLRegexp = regexp.MustCompile(`((((http|https)://)|www\.|((^|[ ])[0-9A-Za-z_\-]+(\.com|\.net|\.org|\.info|\.biz|\.de))))(?U:.*)([ ]+|$)`)
var filterUrlizeEmailRegexp = regexp.MustCompile(`(\w+@\w+\.\w{2,4})`)

func filterUrlizeHelper(input string, trunc int, rel string, target string) (string, error) {
	var soutErr error
	sout := filterUrlizeURLRegexp.ReplaceAllStringFunc(input, func(raw_url string) string {
		var prefix string
		var suffix string
		if strings.HasPrefix(raw_url, " ") {
			prefix = " "
		}
		if strings.HasSuffix(raw_url, " ") {
			suffix = " "
		}

		raw_url = strings.TrimSpace(raw_url)

		url := utils.IRIEncode(raw_url)

		if !strings.HasPrefix(url, "http") {
			url = fmt.Sprintf("http://%s", url)
		}

		title := raw_url

		if trunc > 3 && len(title) > trunc {
			title = fmt.Sprintf("%s...", title[:trunc-3])
		}

		title = utils.Escape(title)

		attrs := ""
		if len(target) > 0 {
			attrs = fmt.Sprintf(` target="%s"`, target)
		}

		rels := []string{}
		cleanedRel := strings.Trim(strings.Replace(rel, "noopener", "", -1), " ")
		if len(cleanedRel) > 0 {
			rels = append(rels, cleanedRel)
		}
		rels = append(rels, "noopener")
		rel = strings.Join(rels, " ")

		return fmt.Sprintf(`%s<a href="%s" rel="%s"%s>%s</a>%s`, prefix, url, rel, attrs, title, suffix)
	})
	if soutErr != nil {
		return "", soutErr
	}

	sout = filterUrlizeEmailRegexp.ReplaceAllStringFunc(sout, func(mail string) string {
		title := mail

		if trunc > 3 && len(title) > trunc {
			title = fmt.Sprintf("%s...", title[:trunc-3])
		}

		return fmt.Sprintf(`<a href="mailto:%s">%s</a>`, mail, title)
	})
	return sout, nil
}

func filterUrlize(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	p := params.Expect(0, []*exec.KwArg{
		{Name: "trim_url_limit", Default: nil},
		{Name: "nofollow", Default: false},
		{Name: "target", Default: nil},
		{Name: "rel", Default: nil},
	})
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'urlize'"))
	}
	truncate := -1
	if param := p.KwArgs["trim_url_limit"]; param.IsInteger() {
		truncate = param.Integer()
	}
	rel := p.KwArgs["rel"]
	target := p.KwArgs["target"]

	s, err := filterUrlizeHelper(in.String(), truncate, rel.String(), target.String())
	if err != nil {
		return exec.AsValue(err)
	}

	return exec.AsValue(s)
}

func filterWordcount(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	if p := params.ExpectNothing(); p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'wordcount'"))
	}
	return exec.AsValue(len(strings.Fields(in.String())))
}

func filterWordwrap(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	words := strings.Fields(in.String())
	wordsLen := len(words)
	wrapAt := params.Args[0].Integer()
	if wrapAt <= 0 {
		return in
	}

	linecount := wordsLen/wrapAt + wordsLen%wrapAt
	lines := make([]string, 0, linecount)
	for i := range linecount {
		min := wrapAt * (i + 1)
		if wordsLen < min {
			min = wordsLen
		}
		lines = append(lines, strings.Join(words[wrapAt*i:min], " "))
	}
	return exec.AsValue(strings.Join(lines, "\n"))
}

func filterXMLAttr(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	p := params.ExpectKwArgs([]*exec.KwArg{{Name: "autospace", Default: true}})
	if p.IsError() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'xmlattr'"))
	}
	autospace := p.KwArgs["autospace"].Bool()
	kvs := []string{}
	in.Iterate(func(idx, count int, key, value *exec.Value) bool {
		if !value.IsTrue() {
			return true
		}
		kv := fmt.Sprintf(`%s="%s"`, key.Escaped(), value.Escaped())
		kvs = append(kvs, kv)
		return true
	}, func() {})
	out := strings.Join(kvs, " ")
	if autospace {
		out = " " + out
	}
	return exec.AsValue(out)
}

func filterDefault(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	p := params.Expect(1, []*exec.KwArg{{
		Name:    "boolean",
		Default: false,
	}})
	if p.IsError() || !p.GetKeywordArgument("boolean", false).IsBool() {
		return exec.AsValue(errors.Wrap(p, "Wrong signature for 'default'"))
	}
	if in.IsError() || in.IsNil() {
		return p.First()
	}
	if p.GetKeywordArgument("boolean", false).Bool() && !in.IsTrue() {
		return p.First()
	}
	return in
}

func filterSelectAttr(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) *exec.Value {
	if in.IsError() {
		return in
	}
	var test func(*exec.Value) *exec.Value
	if len(params.Args) < 1 {
		return exec.AsValue(errors.New("Wrong signature for 'selectattr', expect at least an attribute name as argument"))
	}
	attribute := params.First().String()
	if len(params.Args) == 1 {
		// Reject truthy value
		test = func(in *exec.Value) *exec.Value {
			attr, found := in.Get(attribute)
			if !found {
				return exec.AsValue(errors.Errorf(`%s has no attribute '%s'`, in.String(), attribute))
			}
			return attr
		}
	} else {
		name := params.Args[1].String()
		testParams := &exec.VarArgs{
			Args:   params.Args[2:],
			KwArgs: params.KwArgs,
		}
		test = func(in *exec.Value) *exec.Value {
			attr, found := in.Get(attribute)
			if !found {
				return exec.AsValue(errors.Errorf(`%s has no attribute '%s'`, in.String(), attribute))
			}
			out := e.ExecuteTestByName(name, attr, testParams)
			return out
		}
	}

	out := make([]any, 0)
	var err *exec.Value

	in.Iterate(func(idx, count int, key, value *exec.Value) bool {
		result := test(key)
		if result.IsError() {
			err = result
			return false
		}
		if result.IsTrue() {
			out = append(out, key.Interface())
		}
		return true
	}, func() {})

	if err != nil {
		return err
	}
	return exec.AsValue(out)
}
