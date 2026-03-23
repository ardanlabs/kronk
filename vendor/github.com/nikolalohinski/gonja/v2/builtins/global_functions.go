package builtins

import (
	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/utils"
	"github.com/pkg/errors"
)

var GlobalFunctions = exec.NewContext(map[string]any{
	"cycler":    cyclerFunction,
	"dict":      dictFunction,
	"joiner":    joinerFunction,
	"lipsum":    lipSumFunction,
	"namespace": namespaceFunction,
	"range":     rangeFunction,
})

func rangeFunction(_ *exec.Evaluator, params *exec.VarArgs) (<-chan int, error) {
	var (
		start = 0
		stop  = -1
		step  = 1
	)
	switch n := len(params.Args); n > 0 {
	case n == 1 && params.Args[0].IsInteger():
		stop = params.Args[0].Integer()
	case n == 2 && params.Args[0].IsInteger() && params.Args[1].IsInteger():
		start = params.Args[0].Integer()
		stop = params.Args[1].Integer()
	case n == 3 && params.Args[0].IsInteger() && params.Args[1].IsInteger() && params.Args[2].IsInteger():
		start = params.Args[0].Integer()
		stop = params.Args[1].Integer()
		step = params.Args[2].Integer()
	default:
		return nil, exec.ErrInvalidCall(errors.New("expected signature is [start, ]stop[, step] where all arguments are integers"))
	}

	if step == 0 {
		return nil, exec.ErrInvalidCall(errors.New("step cannot be 0"))
	}

	channel := make(chan int)
	go func() {
		if step > 0 {
			for i := start; i < stop; i += step {
				channel <- i
			}
		} else {
			for i := start; i > stop; i += step {
				channel <- i
			}
		}
		close(channel)
	}()
	return channel, nil
}

func dictFunction(_ *exec.Evaluator, params *exec.VarArgs) *exec.Value {
	dict := exec.NewDict()
	for key, value := range params.KwArgs {
		dict.Pairs = append(dict.Pairs, &exec.Pair{
			Key:   exec.AsValue(key),
			Value: value,
		})
	}
	return exec.AsValue(dict)
}

type cycler struct {
	values  []string
	idx     int
	getters map[string]any
}

func (c *cycler) Reset() {
	c.idx = 0
	c.getters["current"] = c.values[c.idx]
}

func (c *cycler) Next() string {
	c.idx++
	value := c.getters["current"].(string)
	if c.idx >= len(c.values) {
		c.idx = 0
	}
	c.getters["current"] = c.values[c.idx]
	return value
}

func cyclerFunction(_ *exec.Evaluator, params *exec.VarArgs) *exec.Value {
	c := &cycler{}
	for _, arg := range params.Args {
		c.values = append(c.values, arg.String())
	}
	c.getters = map[string]any{
		"next":  c.Next,
		"reset": c.Reset,
	}
	c.Reset()
	return exec.AsValue(c.getters)
}

type joiner struct {
	sep   string
	first bool
}

func (j *joiner) String() string {
	if !j.first {
		j.first = true
		return ""
	}
	return j.sep
}

func joinerFunction(_ *exec.Evaluator, params *exec.VarArgs) *exec.Value {
	var (
		sep string
	)
	if err := params.Take(
		exec.KeywordArgument("sep", exec.AsValue(","), exec.StringArgument(&sep)),
	); err != nil {
		return exec.AsValue(exec.ErrInvalidCall(err))
	}
	j := &joiner{sep: sep}
	return exec.AsValue(j.String)
}

func namespaceFunction(_ *exec.Evaluator, params *exec.VarArgs) map[string]any {
	ns := map[string]any{}
	for key, value := range params.KwArgs {
		ns[key] = value
	}
	return ns
}

func lipSumFunction(_ *exec.Evaluator, params *exec.VarArgs) *exec.Value {
	var (
		n    int
		html bool
		min  int
		max  int
	)
	if err := params.Take(
		exec.KeywordArgument("n", exec.AsValue(5), exec.IntArgument(&n)),
		exec.KeywordArgument("html", exec.AsValue(true), exec.BoolArgument(&html)),
		exec.KeywordArgument("min", exec.AsValue(20), exec.IntArgument(&min)),
		exec.KeywordArgument("max", exec.AsValue(100), exec.IntArgument(&max)),
	); err != nil {
		return exec.AsValue(exec.ErrInvalidCall(err))
	}
	return exec.AsSafeValue(utils.Lipsum(n, html, min, max))
}
