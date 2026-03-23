package exec

import (
	"fmt"
	"reflect"
	"runtime"

	"github.com/nikolalohinski/gonja/v2/nodes"
	"github.com/pkg/errors"
)

func (e *Evaluator) evalCall(node *nodes.Call) *Value {
	fn := e.Eval(node.Func)
	if !fn.IsCallable() {
		getAttributeNode, ok := node.Func.(*nodes.GetAttribute)
		if node.Parent == nil || !ok {
			return AsValue(errors.Errorf(`%s is not callable`, node.Func))
		}
		return e.evalMethod(node.Parent, getAttributeNode.Attribute, node.Args, node.Kwargs)
	}
	if fn.IsError() {
		return AsValue(errors.Wrapf(fn, `unable to evaluate function '%s'`, node.Func))
	}

	var current reflect.Value
	var isSafe bool

	var params []reflect.Value
	var err error
	t := fn.Val.Type()

	if t.NumIn() == 1 && t.In(0) == reflect.TypeFor[*VarArgs]() {
		params, err = e.evalVarArgs(node)
	} else if t.NumIn() == 2 && t.In(0) == reflect.TypeFor[*Evaluator]() && t.In(1) == reflect.TypeFor[*VarArgs]() {
		params, err = e.evalVarArgs(node)
		params = append([]reflect.Value{reflect.ValueOf(e)}, params...)
	} else {
		params, err = e.evalParams(node, fn)
	}
	if err != nil {
		return AsValue(errors.Wrapf(err, `unable to evaluate parameters`))
	}
	functionName := runtime.FuncForPC(fn.Val.Pointer()).Name()
	if nameNode, ok := node.Func.(*nodes.Name); ok {
		functionName = nameNode.Name.Val
	}

	// Call it and get first return parameter back
	values := fn.Val.Call(params)
	rv := values[0]
	if t.NumOut() == 2 {
		e := values[1].Interface()
		if e != nil {
			err, ok := e.(error)
			if !ok {
				return AsValue(fmt.Errorf("second return value of function '%s' is not an error", functionName))
			}
			if err, ok := err.(ErrInvalidCall); ok && err != nil {
				return AsValue(fmt.Errorf("invalid call to function '%s': %s", functionName, err.Error()))
			} else if err != nil {
				return AsValue(err)
			}
		}
	}

	if rv.Type() != typeOfValuePtr {
		current = reflect.ValueOf(rv.Interface())
	} else {
		// Return the function call value
		current = rv.Interface().(*Value).Val
		isSafe = rv.Interface().(*Value).Safe
	}

	if !current.IsValid() {
		// Value is not valid (e. g. NIL value)
		return AsValue(nil)
	}
	value := &Value{Val: current, Safe: isSafe}
	if value.IsError() {
		if err, ok := value.Interface().(ErrInvalidCall); ok {
			return AsValue(fmt.Errorf("invalid call to function '%s': %s", functionName, err.Error()))
		}
	}
	return value
}

func (e *Evaluator) evalMethod(parentNode nodes.Node, method string, args []nodes.Expression, kwargs map[string]nodes.Expression) *Value {
	parent := e.Eval(parentNode)
	if parent.IsError() {
		return AsValue(errors.Wrapf(parent, "unable to evaluate '%s'", parentNode))
	}
	parameters := NewVarArgs()
	for _, param := range args {
		value := e.Eval(param)
		if value.IsError() {
			return AsValue(errors.Wrapf(value, "unable to evaluate parameter %s", param))
		}
		parameters.Args = append(parameters.Args, value)
	}

	for key, param := range kwargs {
		value := e.Eval(param)
		if value.IsError() {
			return AsValue(errors.Wrapf(value, "unable to evaluate parameter %s=%s", key, param))
		}
		parameters.KwArgs[key] = value
	}
	var result any
	err := fmt.Errorf("unknown method '%s' for '%s'", method, parent.String())
	switch {
	case parent.IsString():
		if method, ok := e.Environment.Methods.Str.Get(method); ok {
			result, err = method(parent.String(), parent, parameters)
		}
	case parent.IsBool():
		if method, ok := e.Environment.Methods.Bool.Get(method); ok {
			result, err = method(parent.Bool(), parent, parameters)
		}
	case parent.IsFloat():
		if method, ok := e.Environment.Methods.Float.Get(method); ok {
			result, err = method(parent.Float(), parent, parameters)
		}
	case parent.IsInteger():
		if method, ok := e.Environment.Methods.Int.Get(method); ok {
			result, err = method(parent.Integer(), parent, parameters)
		}
	case parent.IsDict():
		if method, ok := e.Environment.Methods.Dict.Get(method); ok {
			dict := parent.ToGoSimpleType(false)
			if err, ok := dict.(error); err != nil && ok {
				return AsValue(fmt.Errorf("failed to cast '%s' to a Go type: %s", parent.String(), err))
			}
			goMap, ok := dict.(map[string]any)
			if !ok {
				return AsValue(fmt.Errorf("failed to cast '%s' to map[string]interface{}: %s", parent.String(), err))
			}
			result, err = method(goMap, parent, parameters)
		}
	case parent.IsList():
		if method, ok := e.Environment.Methods.List.Get(method); ok {
			list := parent.ToGoSimpleType(false)
			if err, ok := list.(error); err != nil && ok {
				return AsValue(fmt.Errorf("failed to cast '%s' to a Go type: %s", parent.String(), err))
			}
			goList, ok := list.([]any)
			if !ok {
				return AsValue(fmt.Errorf("failed to cast '%s' to []interface{}: %s", parent.String(), err))
			}
			result, err = method(goList, parent, parameters)
		}
	default:
		err = AsValue(errors.Errorf(`'%s' is not callable on %s`, method, parent))
	}
	if err != nil {
		if callErr, ok := err.(ErrInvalidCall); ok {
			return AsValue(fmt.Errorf("invalid call to method '%s' of %s: %s", method, parent.String(), callErr.Error()))
		}
		return AsValue(err)
	}
	if n, ok := parentNode.(*nodes.Name); ok && e.Environment.Context.Has(n.Name.Val) {
		e.Environment.Context.Set(n.Name.Val, parent.Interface())
	}

	return AsValue(result)
}
