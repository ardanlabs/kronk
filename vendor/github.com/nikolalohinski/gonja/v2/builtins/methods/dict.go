package methods

import (
	"sort"

	"github.com/nikolalohinski/gonja/v2/exec"
)

var dictMethods = exec.NewMethodSet[map[string]any](map[string]exec.Method[map[string]any]{
	"keys": func(self map[string]any, selfValue *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		keys := make([]string, 0)
		for key := range self {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return keys, nil
	},
	"items": func(self map[string]any, selfValue *exec.Value, arguments *exec.VarArgs) (any, error) {
		if err := arguments.Take(); err != nil {
			return nil, exec.ErrInvalidCall(err)
		}
		items := make([]any, 0)
		for _, item := range self {
			items = append(items, item)
		}
		return items, nil
	},
})
