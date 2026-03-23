package exec

import (
	"fmt"
	"strings"

	"github.com/nikolalohinski/gonja/v2/nodes"
	"github.com/pkg/errors"
)

// Macro is the type macro functions must fulfill
type Macro func(params *VarArgs) *Value

type MacroSet map[string]Macro

// Exists returns true if the given filter is already registered
func (ms MacroSet) Exists(name string) bool {
	_, existing := ms[name]
	return existing
}

// Register registers a new filter. If there's already a filter with the same
// name, Register will panic. You usually want to call this
// function in the filter's init() function:
// http://golang.org/doc/effective_go.html#init
func (ms *MacroSet) Register(name string, fn Macro) error {
	if ms.Exists(name) {
		return errors.Errorf("filter with name '%s' is already registered", name)
	}
	(*ms)[name] = fn
	return nil
}

// Replace replaces an already registered filter with a new implementation. Use this
// function with caution since it allows you to change existing filter behaviour.
func (ms *MacroSet) Replace(name string, fn Macro) error {
	if !ms.Exists(name) {
		return errors.Errorf("filter with name '%s' does not exist (therefore cannot be overridden)", name)
	}
	(*ms)[name] = fn
	return nil
}

func MacroNodeToFunc(node *nodes.Macro, r *Renderer) (Macro, error) {
	return func(params *VarArgs) *Value {
		var out strings.Builder
		sub := r.Inherit()
		sub.Output = &out

		macroArguments := make([]*Pair, len(node.Kwargs))
		for i, positionalArgument := range params.Args {
			if i >= len(node.Kwargs) {
				return AsValue(fmt.Errorf("macro '%s' received %d arguments but expected only %d", node.Name, len(params.Args), len(node.Wrapper.Nodes)))
			}
			key := r.Eval(node.Kwargs[i].Key)
			if key.IsError() {
				return AsValue(fmt.Errorf("macro '%s' failed to evaluate positional argument named '%s': %s", node.Name, node.Kwargs[i].Key.String(), key))
			}
			macroArguments[i] = &Pair{
				Value: positionalArgument,
				Key:   key,
			}
		}
	kwargs:
		for keyword, argument := range params.KwArgs {
			for i, validArgument := range node.Kwargs {
				validKeyword := r.Eval(validArgument.Key)
				if validKeyword.IsError() {
					return AsValue(fmt.Errorf("macro '%s' failed to evaluate positional argument named '%s': %s", node.Name, node.Kwargs[i].Key.String(), validKeyword))
				}
				if validKeyword.String() == keyword {
					if macroArguments[i] != nil {
						return AsValue(fmt.Errorf("macro '%s' received '%s' argument twice", node.Name, keyword))
					}
					macroArguments[i] = &Pair{
						Value: argument,
						Key:   validKeyword,
					}
					continue kwargs
				}
			}
			return AsValue(fmt.Errorf("macro '%s' takes no keyword argument '%s'", node.Name, keyword))
		}
		for i, defaultArgument := range node.Kwargs {
			if macroArguments[i] == nil {
				key := r.Eval(defaultArgument.Key)
				if key.IsError() {
					return AsValue(fmt.Errorf("macro '%s' failed to evaluate default argument key named '%s': %s", node.Name, defaultArgument.Key.String(), key))
				}
				value := r.Eval(defaultArgument.Value)
				if value.IsError() {
					return AsValue(fmt.Errorf("macro '%s' failed to evaluate '%s': %s", node.Name, defaultArgument.Value.String(), value))
				}
				macroArguments[i] = &Pair{
					Key:   key,
					Value: value,
				}
			}
		}
		for _, arg := range macroArguments {
			sub.Environment.Context.Set(arg.Key.String(), arg.Value)
		}
		err := sub.ExecuteWrapper(node.Wrapper)
		if err != nil {
			return AsValue(errors.Wrapf(err, `Unable to execute macro '%s'`, node.Name))
		}
		return AsSafeValue(out.String())
	}, nil
}
