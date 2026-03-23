package methods

import (
	"reflect"

	"github.com/nikolalohinski/gonja/v2/exec"
)

var listMethods = exec.NewMethodSet[[]any](map[string]exec.Method[[]any]{
	"append": func(_ []any, selfValue *exec.Value, arguments *exec.VarArgs) (any, error) {
		var (
			x any
		)
		if err := arguments.Take(
			exec.PositionalArgument("x", nil, exec.AnyArgument(&x)),
		); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}

		*selfValue = *exec.ToValue(reflect.Append(selfValue.Val, reflect.ValueOf(exec.ToValue(x))))

		return nil, nil
	},
	"reverse": func(_ []any, selfValue *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		reversed := reflect.MakeSlice(selfValue.Val.Type(), 0, 0)
		for i := selfValue.Val.Len() - 1; i >= 0; i-- {
			reversed = reflect.Append(reversed, selfValue.Val.Index(i))
		}
		for i := 0; i < selfValue.Val.Len(); i++ {
			selfValue.Val.Index(i).Set(reversed.Index(i))
		}

		return nil, nil
	},
	"copy": func(self []any, selfValue *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		return self, nil
	},
})
