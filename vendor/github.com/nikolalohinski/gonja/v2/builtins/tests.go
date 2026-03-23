package builtins

import (
	"errors"
	"reflect"
	"strings"

	"github.com/nikolalohinski/gonja/v2/exec"
)

var Tests = exec.NewTestSet(map[string]exec.TestFunction{
	"boolean":     testBoolean,
	"callable":    testCallable,
	"defined":     testDefined,
	"divisibleby": testDivisibleby,
	"eq":          testEqual,
	"equalto":     testEqual,
	"==":          testEqual,
	"escaped":     testEscaped,
	"even":        testEven,
	"false":       testFalse,
	"filter":      testFilter,
	"float":       testFloat,
	"ge":          testGreaterEqual,
	">=":          testGreaterEqual,
	"gt":          testGreaterThan,
	"greaterthan": testGreaterThan,
	">":           testGreaterThan,
	"in":          testIn,
	"integer":     testInteger,
	"iterable":    testIterable,
	"le":          testLessEqual,
	"<=":          testLessEqual,
	"lower":       testLower,
	"lt":          testLessThan,
	"lessthan":    testLessThan,
	"<":           testLessThan,
	"mapping":     testMapping,
	"ne":          testNotEqual,
	"!=":          testNotEqual,
	"none":        testNone,
	"number":      testNumber,
	"odd":         testOdd,
	"sameas":      testSameas,
	"sequence":    testSequence,
	"string":      testString,
	"test":        testTest,
	"true":        testTrue,
	"undefined":   testUndefined,
	"upper":       testUpper,
})

func testBoolean(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	return in.IsBool(), nil
}

func testCallable(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	return in.IsCallable(), nil
}

func testDefined(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	return !(in.IsError() || in.IsNil()), nil
}

func testDivisibleby(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	param := params.First()
	if param.Integer() == 0 {
		return false, nil
	}
	return in.Integer()%param.Integer() == 0, nil
}

func testEqual(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	param := params.First()
	return in.EqualValueTo(param), nil
}

func testEven(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	if !in.IsInteger() {
		return false, nil
	}
	return in.Integer()%2 == 0, nil
}

func testFalse(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	return !in.Bool(), nil
}

func testFloat(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	return in.IsFloat(), nil
}

func testGreaterEqual(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	param := params.Args[0]
	if !in.IsNumber() || !param.IsNumber() {
		return false, nil
	}
	return in.Float() >= param.Float(), nil
}

func testGreaterThan(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	var to float64
	if err := params.Take(
		exec.PositionalArgument("to", nil, exec.NumberArgument(&to)),
	); err != nil {
		return false, exec.ErrInvalidCall(err)
	}

	return in.Float() > to, nil
}

func testIn(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	seq := params.First()
	return seq.Contains(in), nil
}

func testInteger(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	return in.IsInteger(), nil
}

func testIterable(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	return in.IsDict() || in.IsList() || in.IsString(), nil
}

func testSequence(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	return in.IsList(), nil
}

func testLessEqual(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	param := params.Args[0]
	if !in.IsNumber() || !param.IsNumber() {
		return false, nil
	}
	return in.Float() <= param.Float(), nil
}

func testLower(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	if !in.IsString() {
		return false, nil
	}
	return strings.ToLower(in.String()) == in.String(), nil
}

func testLessThan(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	param := params.Args[0]
	if !in.IsNumber() || !param.IsNumber() {
		return false, nil
	}
	return in.Float() < param.Float(), nil
}

func testMapping(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	return in.IsDict(), nil
}

func testNotEqual(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	param := params.Args[0]
	return in.Interface() != param.Interface(), nil
}

func testNone(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	return in.IsNil(), nil
}

func testNumber(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	return in.IsNumber(), nil
}

func testOdd(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	if !in.IsInteger() {
		return false, nil
	}
	return in.Integer()%2 == 1, nil
}

func testSameas(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	param := params.Args[0]
	if in.IsNil() && param.IsNil() {
		return true, nil
	} else if param.Val.CanAddr() && in.Val.CanAddr() {
		return param.Val.Addr() == in.Val.Addr(), nil
	}
	return reflect.Indirect(param.Val) == reflect.Indirect(in.Val), nil
}

func testString(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	return in.IsString(), nil
}

func testTrue(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	return in.Bool(), nil
}

func testUndefined(ctx *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	defined, err := testDefined(ctx, in, params)
	return !defined, err
}

func testUpper(_ *exec.Context, in *exec.Value, params *exec.VarArgs) (bool, error) {
	if !in.IsString() {
		return false, nil
	}
	return strings.ToUpper(in.String()) == in.String(), nil
}

func testEscaped(_ *exec.Evaluator, in *exec.Value, params *exec.VarArgs) (bool, error) {
	if in.IsError() {
		return false, errors.New(in.Error())
	}
	if err := params.ExpectNothing(); err != nil {
		return false, exec.ErrInvalidCall(err)
	}
	escaped := in.Escaped()
	return escaped == in.String(), nil
}

func testTest(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) (bool, error) {
	if in.IsError() {
		return false, errors.New(in.Error())
	}
	if err := params.ExpectNothing(); err != nil {
		return false, exec.ErrInvalidCall(err)
	}
	return e.Environment.Tests.Exists(in.String()), nil
}

func testFilter(e *exec.Evaluator, in *exec.Value, params *exec.VarArgs) (bool, error) {
	if in.IsError() {
		return false, errors.New(in.Error())
	}
	if err := params.ExpectNothing(); err != nil {
		return false, exec.ErrInvalidCall(err)
	}
	return e.Environment.Filters.Exists(in.String()), nil
}
