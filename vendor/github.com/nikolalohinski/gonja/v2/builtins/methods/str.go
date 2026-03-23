package methods

import (
	"errors"

	"github.com/nikolalohinski/gonja/v2/builtins/methods/pyerrors"
	"github.com/nikolalohinski/gonja/v2/builtins/methods/pystring"
	"github.com/nikolalohinski/gonja/v2/exec"
	"golang.org/x/exp/utf8string"
)

var strMethods = exec.NewMethodSet[string](map[string]exec.Method[string]{
	"capitalize": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).Capitalize(), nil
	},
	"capwords": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).CapWords(), nil
	},
	"casefold": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).Casefold(), nil
	},
	"center": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			width    int
			fillchar string
		)
		if err := arguments.Take(
			exec.PositionalArgument("width", nil, exec.IntArgument(&width)),
			exec.PositionalArgument("fillchar", exec.AsValue(' '), exec.StringArgument(&fillchar)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).Center(width, utf8string.NewString(fillchar).At(0)), nil
	},
	"count": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			sub   string
			start int
			end   int
		)
		if err := arguments.Take(
			exec.PositionalArgument("sub", nil, exec.StringArgument(&sub)),
			exec.PositionalArgument("start", exec.AsValue(0), exec.IntArgument(&start)),
			exec.PositionalArgument("end", exec.AsValue(len(self)), exec.IntArgument(&end)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).Count(pystring.New(self), &start, &end), nil
	},
	"encode": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			encoding  string
			errorsArg string
		)
		if err := arguments.Take(
			exec.KeywordArgument("encoding", exec.AsValue("utf-8"), exec.StringEnumArgument(&encoding, []string{
				// https://docs.python.org/3/library/codecs.html#standard-encodings
				// Supporting just the most common on that are easy to implement in Go
				"utf-8", "utf_8", "U8", "UTF", "utf8", "cp65001", // UTF-8 and its aliases
				"latin_1", "iso-8859-1", "iso8859-1", "8859", "cp819", "latin", "latin1", "L1", // ISO-8859-1 and its aliases
			})),
			exec.KeywordArgument("errors", exec.AsValue("strict"), exec.StringEnumArgument(&errorsArg, []string{
				// See https://docs.python.org/3/library/codecs.html#error-handlers
				"strict",
				"ignore", // only makes sense for ISO-8859-1
				// not implementing the arguments below as it is too much work
				// and it is very likely no one is using them in Jinja templates
				// "replace",
				// "xmlcharrefreplace",
				// "backslashreplace",
			})),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}

		res, err := pystring.PyString(self).Encode(encoding, errorsArg)
		if err != nil {
			if errors.Is(err, pyerrors.ErrArguments) {
				return nil, exec.ErrInvalidCall(err)
			}
			return nil, err
		}

		return res, nil
	},
	"endswith": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			suffix string
			start  int
			end    int
		)
		if err := arguments.Take(
			exec.PositionalArgument("suffix", nil, exec.StringArgument(&suffix)),
			exec.PositionalArgument("start", exec.AsValue(0), exec.IntArgument(&start)),
			exec.PositionalArgument("end", exec.AsValue(len(self)), exec.IntArgument(&end)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).EndsWith(pystring.New(suffix), &start, &end), nil
	},
	"expandtabs": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			substr  string
			tabsize int
			end     int
		)
		if err := arguments.Take(
			exec.PositionalArgument("substr", nil, exec.StringArgument(&substr)),
			exec.PositionalArgument("tabsize", exec.AsValue(0), exec.IntArgument(&tabsize)),
			exec.PositionalArgument("end", exec.AsValue(len(self)), exec.IntArgument(&end)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).ExpandTabs(pystring.New(substr), &tabsize), nil
	},
	"find": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			sub   string
			start int
			end   int
		)
		if err := arguments.Take(
			exec.PositionalArgument("sub", nil, exec.StringArgument(&sub)),
			exec.PositionalArgument("start", exec.AsValue(0), exec.IntArgument(&start)),
			exec.PositionalArgument("end", exec.AsValue(len(self)), exec.IntArgument(&end)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).Find(pystring.New(sub), &start, &end), nil
	},
	"format": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		args := make([]any, 0, len(arguments.Args))
		for _, arg := range arguments.Args {
			args = append(args, arg.Interface())
		}
		kwargs := make(map[string]any)
		for key, value := range arguments.KwArgs {
			kwargs[key] = value.Interface()
		}
		return pystring.PyString(self).Format(args, kwargs)
	},
	"format_map": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		args := make([]any, 0, len(arguments.Args))
		for _, arg := range arguments.Args {
			args = append(args, arg.Interface())
		}
		kwargs := make(map[string]any)
		for key, value := range arguments.KwArgs {
			kwargs[key] = value.Interface()
		}
		return pystring.PyString(self).FormatMap(args, kwargs)
	},
	"isalnum": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).IsAlnum(), nil
	},
	"isalpha": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).IsAlpha(), nil
	},
	"isascii": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).IsASCII(), nil
	},
	"isdecimal": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).IsDecimal(), nil
	},
	"isdigit": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).IsDigit(), nil
	},
	"islower": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).IsLower(), nil
	},
	"isnumeric": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).IsNumeric(), nil
	},
	"isprintable": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).IsPrintable(), nil
	},
	"isspace": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).IsSpace(), nil
	},
	"istitle": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).IsTitle(), nil
	},
	"isupper": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).IsUpper(), nil
	},
	"join": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			strs []string
		)
		if err := arguments.Take(
			exec.PositionalArgument("iterable", nil, exec.StringListArgument(&strs)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.JoinString(self, strs), nil
	},
	"ljust": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			width    int
			fillchar string
		)
		if err := arguments.Take(
			exec.PositionalArgument("width", nil, exec.IntArgument(&width)),
			exec.PositionalArgument("fillchar", exec.AsValue(' '), exec.StringArgument(&fillchar)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).LJust(width, utf8string.NewString(fillchar).At(0)), nil
	},
	"lower": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).Lower(), nil
	},
	"lstrip": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			cutset string
		)
		if err := arguments.Take(
			exec.PositionalArgument("cutset", exec.AsValue(' '), exec.StringArgument(&cutset)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).LStrip(cutset), nil
	},
	"partition": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			sep string
		)
		if err := arguments.Take(
			exec.PositionalArgument("sep", exec.AsValue(' '), exec.StringArgument(&sep)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		p1, p2, p3 := pystring.PyString(self).Partition(sep)
		return []string{p1.String(), p2.String(), p3.String()}, nil
	},
	"removeprefix": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			prefix string
		)
		if err := arguments.Take(
			exec.PositionalArgument("prefix", nil, exec.StringArgument(&prefix)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).RemovePrefix(prefix), nil
	},
	"removesuffix": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			suffix string
		)
		if err := arguments.Take(
			exec.PositionalArgument("suffix", nil, exec.StringArgument(&suffix)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).RemoveSuffix(suffix), nil
	},
	"replace": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			old   string
			new   string
			count int
		)
		if err := arguments.Take(
			exec.PositionalArgument("old", exec.AsValue(' '), exec.StringArgument(&old)),
			exec.PositionalArgument("new", exec.AsValue(' '), exec.StringArgument(&new)),
			exec.PositionalArgument("count", nil, exec.IntArgument(&count)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).Replace(old, new, count), nil
	},
	"rfind": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			sub   string
			start int
			end   int
		)
		if err := arguments.Take(
			exec.PositionalArgument("sub", nil, exec.StringArgument(&sub)),
			exec.PositionalArgument("start", exec.AsValue(0), exec.IntArgument(&start)),
			exec.PositionalArgument("end", exec.AsValue(len(self)), exec.IntArgument(&end)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).RFind(sub, &start, &end), nil
	},
	"rjust": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			width    int
			fillchar string
		)
		if err := arguments.Take(
			exec.PositionalArgument("width", nil, exec.IntArgument(&width)),
			exec.PositionalArgument("fillchar", exec.AsValue(' '), exec.StringArgument(&fillchar)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).RJust(width, utf8string.NewString(fillchar).At(0)), nil
	},
	"rpartition": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			sep string
		)
		if err := arguments.Take(
			exec.PositionalArgument("sep", exec.AsValue(' '), exec.StringArgument(&sep)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		p1, p2, p3 := pystring.PyString(self).RPartition(sep)
		return []string{p1.String(), p2.String(), p3.String()}, nil
	},
	"rsplit": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			sep      string
			maxsplit int
		)
		if err := arguments.Take(
			exec.PositionalArgument("sep", exec.AsValue(' '), exec.StringArgument(&sep)),
			exec.PositionalArgument("maxsplit", exec.AsValue(-1), exec.IntArgument(&maxsplit)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).RSplit(sep, maxsplit), nil
	},
	"rstrip": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			cutset string
		)
		if err := arguments.Take(
			exec.PositionalArgument("cutset", exec.AsValue(' '), exec.StringArgument(&cutset)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).RStrip(cutset), nil
	},
	"split": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			sep      string
			maxsplit int
		)
		if err := arguments.Take(
			exec.PositionalArgument("sep", exec.AsValue(' '), exec.StringArgument(&sep)),
			exec.PositionalArgument("maxsplit", exec.AsValue(-1), exec.IntArgument(&maxsplit)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).Split(sep, maxsplit), nil
	},
	"splitlines": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			keepends bool
		)
		if err := arguments.Take(
			exec.PositionalArgument("keepends", exec.AsValue(false), exec.BoolArgument(&keepends)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).SplitLines(keepends), nil
	},
	"startswith": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			prefix   string
			prefixes []string
			start    int
			end      int
		)
		if err := arguments.Take(
			exec.PositionalArgument("prefix", nil, exec.OrArgument(
				exec.StringArgument(&prefix),
				exec.StringListArgument(&prefixes),
			)),
			exec.PositionalArgument("start", exec.AsValue(0), exec.IntArgument(&start)),
			exec.PositionalArgument("end", exec.AsValue(len(self)), exec.IntArgument(&end)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}

		// Handle array case.
		if prefixes != nil {
			for _, prefix := range prefixes {
				if pystring.PyString(self).StartsWith(prefix, &start, &end) {
					return true, nil
				}
			}
			return false, nil
		}

		// Handle single prefix case.
		return pystring.PyString(self).StartsWith(prefix, &start, &end), nil
	},
	"strip": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			cutset string
		)
		if err := arguments.Take(
			exec.PositionalArgument("cutset", exec.AsValue(""), exec.StringArgument(&cutset)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).Strip(cutset), nil
	},
	"swapcase": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).SwapCase(), nil
	},
	"title": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).Title(), nil
	},
	"upper": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).Upper(), nil
	},
	"zfill": func(self string, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			width int
		)
		if err := arguments.Take(
			exec.PositionalArgument("width", nil, exec.IntArgument(&width)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pystring.PyString(self).ZFill(width), nil
	},
})
