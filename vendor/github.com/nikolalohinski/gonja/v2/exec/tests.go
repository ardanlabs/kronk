package exec

import (
	"fmt"
	"reflect"

	"github.com/pkg/errors"

	"github.com/nikolalohinski/gonja/v2/nodes"
)

// TestFunction is the type test functions must fulfill is
// type TestFunction func(*Evaluator, *Value, *VarArgs) (bool, error)
// but we use an so as to support the legacy type
// type TestFunction func(*Context, *Value, *VarArgs) (bool, error)
// in a backwards compatible way
type TestFunction any

func (e *Evaluator) EvalTest(expr *nodes.TestExpression) *Value {
	value := e.Eval(expr.Expression)

	return e.ExecuteTest(expr.Test, value)
}

func (e *Evaluator) ExecuteTest(tc *nodes.TestCall, v *Value) *Value {
	params := &VarArgs{
		Args:   []*Value{},
		KwArgs: map[string]*Value{},
	}

	for _, param := range tc.Args {
		value := e.Eval(param)
		if value.IsError() {
			return AsValue(errors.Wrapf(value, `Unable to evaluate parameter %s`, param))
		}
		params.Args = append(params.Args, value)
	}

	for key, param := range tc.Kwargs {
		value := e.Eval(param)
		if value.IsError() {
			return AsValue(errors.Wrapf(value, `Unable to evaluate parameter %s`, param))
		}
		params.KwArgs[key] = value
	}

	return e.ExecuteTestByName(tc.Name, v, params)
}

func (e *Evaluator) ExecuteTestByName(name string, in *Value, params *VarArgs) *Value {
	test, ok := e.Environment.Tests.Get(name)
	if !e.Environment.Tests.Exists(name) || !ok {
		return AsValue(errors.Errorf("test '%s' not found", name))
	}

	if err := e.Environment.Tests.validate(name, test); err != nil {
		return AsValue(fmt.Errorf("test '%s' is invalid: %q", name, err))
	}

	testFn := reflect.ValueOf(test)
	firstArgument := reflect.ValueOf(e)
	if testFn.Type().In(0) == reflect.TypeFor[*Context]() {
		firstArgument = reflect.ValueOf(e.Environment.Context)
	}
	arguments := []reflect.Value{
		firstArgument,
		reflect.ValueOf(in),
		reflect.ValueOf(params),
	}
	results := testFn.Call(arguments)
	result := results[0].Bool()
	var err error
	if !results[1].IsNil() {
		err = results[1].Interface().(error)
	}
	if callErr, ok := err.(ErrInvalidCall); ok && err != nil {
		return AsValue(fmt.Errorf("invalid call to test '%s': %s", name, callErr.Error()))
	} else if err != nil {
		return AsValue(fmt.Errorf("unable to execute test '%s': %s", name, err.Error()))
	} else {
		return AsValue(result)
	}
}
