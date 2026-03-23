package methods

import (
	"github.com/nikolalohinski/gonja/v2/builtins/methods/pyfloat"
	"github.com/nikolalohinski/gonja/v2/exec"
)

var floatMethods = exec.NewMethodSet[float64](map[string]exec.Method[float64]{
	"is_integer": func(self float64, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pyfloat.New(self).IsInteger(), nil
	},
	"as_integer_ratio": func(self float64, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		a, b := pyfloat.New(self).AsIntegerRatio()
		return []int{a, b}, nil // lack of tuple type we reuse the list type
	},
	"hex": func(self float64, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pyfloat.New(self).Hex(), nil // lack of tuple type we reuse the list type
	},
})
