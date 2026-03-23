package methods

import (
	"github.com/nikolalohinski/gonja/v2/builtins/methods/pyint"
	"github.com/nikolalohinski/gonja/v2/exec"
)

var intMethods = exec.NewMethodSet[int](map[string]exec.Method[int]{
	"is_integer": func(self int, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return true, nil
	},
	"bit_length": func(self int, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pyint.New(self).BitLength(), nil
	},
	"bit_count": func(self int, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return pyint.New(self).BitCount(), nil
	},
	"as_integer_ratio": func(self int, _ *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		a, b := pyint.New(self).AsIntegerRatio()
		return []int{a, b}, nil // lack of tuple type we reuse the list type
	},
})
