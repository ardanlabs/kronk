package exec

import (
	"math"
	"reflect"
	"strings"

	"github.com/pkg/errors"

	"github.com/nikolalohinski/gonja/v2/config"
	"github.com/nikolalohinski/gonja/v2/loaders"
	"github.com/nikolalohinski/gonja/v2/nodes"
	"github.com/nikolalohinski/gonja/v2/tokens"
)

var typeOfValuePtr = reflect.TypeFor[*Value]()

type ErrInvalidCall error

type Evaluator struct {
	Config      *config.Config
	Environment *Environment
	Loader      loaders.Loader
}

func (e *Evaluator) Eval(node nodes.Expression) *Value {
	switch n := node.(type) {
	case *nodes.None:
		return AsValue(nil)
	case *nodes.String:
		return AsValue(n.Val)
	case *nodes.Integer:
		return AsValue(n.Val)
	case *nodes.Float:
		return AsValue(n.Val)
	case *nodes.Bool:
		return AsValue(n.Val)
	case *nodes.List:
		return e.evalList(n)
	case *nodes.Tuple:
		return e.evalTuple(n)
	case *nodes.Dict:
		return e.evalDict(n)
	case *nodes.Pair:
		return e.evalPair(n)
	case *nodes.Name:
		return e.evalName(n)
	case *nodes.Call:
		return e.evalCall(n)
	case *nodes.GetItem:
		return e.evalGetItem(n)
	case *nodes.GetAttribute:
		return e.evalGetAttribute(n)
	case *nodes.GetSlice:
		return e.evalGetSlice(n)
	case *nodes.Error:
		return AsValue(n.Error)
	case *nodes.Negation:
		result := e.Eval(n.Term)
		if result.IsError() {
			return result
		}
		return result.Negate()
	case *nodes.BinaryExpression:
		return e.evalBinaryExpression(n)
	case *nodes.UnaryExpression:
		return e.evalUnaryExpression(n)
	case *nodes.FilteredExpression:
		return e.EvaluateFiltered(n)
	case *nodes.TestExpression:
		return e.EvalTest(n)
	default:
		return AsValue(errors.Errorf(`Unknown expression type "%T"`, n))
	}
}

func (e *Evaluator) evalBinaryExpression(node *nodes.BinaryExpression) *Value {
	var (
		left  *Value
		right *Value
	)
	left = e.Eval(node.Left)
	if left.IsError() {
		return AsValue(errors.Wrapf(left, `Unable to evaluate left parameter %s`, node.Left))
	}

	switch node.Operator.Token.Val {
	// These operators allow lazy right expression evluation
	case "and", "or":
	default:
		right = e.Eval(node.Right)
		if right.IsError() {
			return AsValue(errors.Wrapf(right, `Unable to evaluate right parameter %s`, node.Right))
		}
	}

	switch node.Operator.Token.Type {
	case tokens.Addition:
		if left.IsList() {
			if !right.IsList() {
				return AsValue(errors.Wrapf(right, `Unable to concatenate list to %s`, node.Right))
			}

			v := &Value{Val: reflect.ValueOf([]any{})}

			for ix := 0; ix < left.getResolvedValue().Len(); ix++ {
				v.Val = reflect.Append(v.Val, left.getResolvedValue().Index(ix))
			}

			for ix := 0; ix < right.getResolvedValue().Len(); ix++ {
				v.Val = reflect.Append(v.Val, right.getResolvedValue().Index(ix))
			}

			return v
		}
		if left.IsFloat() || right.IsFloat() {
			// Result will be a float
			return AsValue(left.Float() + right.Float())
		}

		if left.IsString() || right.IsString() {
			return AsValue(left.String() + right.String())
		}

		// Result will be an integer
		return AsValue(left.Integer() + right.Integer())
	case tokens.Subtraction:
		if left.IsFloat() || right.IsFloat() {
			// Result will be a float
			return AsValue(left.Float() - right.Float())
		}
		// Result will be an integer
		return AsValue(left.Integer() - right.Integer())
	case tokens.Multiply:
		if left.IsFloat() || right.IsFloat() {
			// Result will be float
			return AsValue(left.Float() * right.Float())
		}
		if left.IsString() {
			return AsValue(strings.Repeat(left.String(), right.Integer()))
		}
		// Result will be int
		return AsValue(left.Integer() * right.Integer())
	case tokens.Division:
		// Float division
		return AsValue(left.Float() / right.Float())
	case tokens.FloorDivision:
		// Int division
		return AsValue(int(left.Float() / right.Float()))
	case tokens.Modulo:
		// Result will be int
		return AsValue(left.Integer() % right.Integer())
	case tokens.Power:
		return AsValue(math.Pow(left.Float(), right.Float()))
	case tokens.Tilde:
		return AsValue(strings.Join([]string{left.String(), right.String()}, ""))
	case tokens.And:
		if !left.IsTrue() {
			return AsValue(false)
		}
		right = e.Eval(node.Right)
		if right.IsError() {
			return AsValue(errors.Wrapf(right, `Unable to evaluate right parameter %s`, node.Right))
		}
		return AsValue(right.IsTrue())
	case tokens.Or:
		if left.IsTrue() {
			return AsValue(true)
		}
		right = e.Eval(node.Right)
		if right.IsError() {
			return AsValue(errors.Wrapf(right, `Unable to evaluate right parameter %s`, node.Right))
		}
		return AsValue(right.IsTrue())
	case tokens.LowerThanOrEqual:
		if left.IsFloat() || right.IsFloat() {
			return AsValue(left.Float() <= right.Float())
		}
		if left.IsString() || right.IsString() {
			return AsValue(left.String() <= right.String())
		}
		return AsValue(left.Integer() <= right.Integer())
	case tokens.GreaterThanOrEqual:
		if left.IsFloat() || right.IsFloat() {
			return AsValue(left.Float() >= right.Float())
		}
		if left.IsString() || right.IsString() {
			return AsValue(left.String() >= right.String())
		}
		return AsValue(left.Integer() >= right.Integer())
	case tokens.Equals:
		return AsValue(left.EqualValueTo(right))
	case tokens.GreaterThan:
		if left.IsFloat() || right.IsFloat() {
			return AsValue(left.Float() > right.Float())
		}
		if left.IsString() || right.IsString() {
			return AsValue(left.String() > right.String())
		}

		return AsValue(left.Integer() > right.Integer())
	case tokens.LowerThan:
		if left.IsFloat() || right.IsFloat() {
			return AsValue(left.Float() < right.Float())
		}
		if left.IsString() || right.IsString() {
			return AsValue(left.String() < right.String())
		}

		return AsValue(left.Integer() < right.Integer())
	case tokens.Ne:
		return AsValue(!left.EqualValueTo(right))
	case tokens.In:
		return AsValue(right.Contains(left))
	default:
		return AsValue(errors.Errorf(`Unknown operator "%s"`, node.Operator.Token))
	}
}

func (e *Evaluator) evalUnaryExpression(expr *nodes.UnaryExpression) *Value {
	result := e.Eval(expr.Term)
	if result.IsError() {
		return AsValue(errors.Wrapf(result, `Unable to evaluate term %s`, expr.Term))
	}
	if expr.Negative {
		if result.IsNumber() {
			switch {
			case result.IsFloat():
				return AsValue(-1 * result.Float())
			case result.IsInteger():
				return AsValue(-1 * result.Integer())
			default:
				return AsValue(errors.New("Operation between a number and a non-(float/integer) is not possible"))
			}
		} else {
			return AsValue(errors.Errorf("Negative sign on a non-number expression %s", expr.Position()))
		}
	}
	return result
}

func (e *Evaluator) evalList(node *nodes.List) *Value {
	values := ValuesList{}
	for _, val := range node.Val {
		value := e.Eval(val)
		values = append(values, value)
	}
	return AsValue(values)
}

func (e *Evaluator) evalTuple(node *nodes.Tuple) *Value {
	values := ValuesList{}
	for _, val := range node.Val {
		value := e.Eval(val)
		values = append(values, value)
	}
	return AsValue(values)
}

func (e *Evaluator) evalDict(node *nodes.Dict) *Value {
	pairs := []*Pair{}
	for _, pair := range node.Pairs {
		p := e.evalPair(pair)
		if p.IsError() {
			return AsValue(errors.Wrapf(p, `Unable to evaluate pair "%s"`, pair))
		}
		pairs = append(pairs, p.Interface().(*Pair))
	}
	return AsValue(&Dict{pairs})
}

func (e *Evaluator) evalPair(node *nodes.Pair) *Value {
	key := e.Eval(node.Key)
	if key.IsError() {
		return AsValue(errors.Wrapf(key, `Unable to evaluate key "%s"`, node.Key))
	}
	value := e.Eval(node.Value)
	if value.IsError() {
		return AsValue(errors.Wrapf(value, `Unable to evaluate value "%s"`, node.Value))
	}
	return AsValue(&Pair{key, value})
}

func (e *Evaluator) evalName(node *nodes.Name) *Value {
	val, ok := e.Environment.Context.Get(node.Name.Val)
	if !ok && e.Config.StrictUndefined {
		return AsValue(errors.Errorf(`Unable to evaluate name "%s"`, node.Name.Val))
	}
	return ToValue(val)
}

func (e *Evaluator) evalGetItem(node *nodes.GetItem) *Value {
	value := e.Eval(node.Node)
	if value.IsError() {
		return AsValue(errors.Wrapf(value, `unable to evaluate target %s`, node.Node))
	}
	if node.Arg == nil {
		if e.Config.StrictUndefined {
			return AsValue(errors.Wrapf(value, `argument is undefined to access: %s`, node.Node))
		} else {
			return AsValue(nil)
		}
	}

	argument := e.Eval(node.Arg)
	var key any
	switch {
	case argument != nil && argument.IsString():
		key = argument.String()
	case argument != nil && argument.IsInteger():
		key = argument.Integer()
	case argument.IsNil() && e.Config.StrictUndefined:
		return AsValue(errors.Wrapf(value, `argument is undefined to access: %s`, node.Node))
	default:
		return AsValue(errors.Wrapf(value, `argument %s does not evaluate to string or integer in: %s`, node.Arg, node.Node))
	}

	item, found := value.GetItem(key)
	if !found && argument.IsString() {
		item, found = value.GetAttribute(argument.String())
	}
	if !found {
		if item.IsError() {
			return AsValue(errors.Wrapf(item, `unable to evaluate %s`, node))
		}
		if e.Config.StrictUndefined {
			return AsValue(errors.Errorf(`unable to evaluate %s: item '%s' not found`, node, node.Arg))
		}
		return AsValue(nil)
	}
	return item
}

func (e *Evaluator) evalGetSlice(node *nodes.GetSlice) *Value {
	value := e.Eval(node.Node)
	if value.IsError() {
		return AsValue(errors.Wrapf(value, `unable to evaluate target %s`, node.Node))
	}
	if !value.CanSlice() {
		return AsValue(errors.Wrapf(value, `can not slice %s`, node.Node))
	}
	start := 0
	end := value.Len()
	if node.Start != nil {
		startValue := e.Eval(node.Start)
		if startValue.IsError() {
			return AsValue(errors.Wrapf(value, `unable to slice starting index %s`, node.Start))
		}
		if !startValue.IsInteger() {
			return AsValue(errors.Wrapf(value, `slice starting index is not an integer: %s`, startValue))
		}
		start = startValue.Integer()
		if start < 0 {
			start = value.Len() + start
		}
	}
	if node.End != nil {
		endValue := e.Eval(node.End)
		if endValue.IsError() {
			return AsValue(errors.Wrapf(value, `unable to slice starting index %s`, node.Start))
		}
		if !endValue.IsInteger() {
			return AsValue(errors.Wrapf(value, `slice starting index is not an integer: %s`, endValue))
		}
		end = endValue.Integer()
		if end < 0 {
			end = value.Len() + end
		}
	}

	return value.Slice(start, end)
}

func (e *Evaluator) evalGetAttribute(node *nodes.GetAttribute) *Value {
	value := e.Eval(node.Node)
	if value.IsError() {
		return AsValue(errors.Wrapf(value, `Unable to evaluate target %s`, node.Node))
	}

	if node.Attribute != "" {
		attr, found := value.GetAttribute(node.Attribute)
		if !found {
			attr, found = value.GetItem(node.Attribute)
		}
		if !found {
			if attr.IsError() {
				return AsValue(errors.Wrapf(attr, `Unable to evaluate %s`, node))
			}
			if e.Config.StrictUndefined {
				return AsValue(errors.Errorf(`Unable to evaluate %s: attribute '%s' not found`, node, node.Attribute))
			}
			return AsValue(nil)
		}
		return attr
	} else {
		item, found := value.GetItem(node.Index)
		if !found {
			if item.IsError() {
				return AsValue(errors.Wrapf(item, `Unable to evaluate %s`, node))
			}
			if e.Config.StrictUndefined {
				return AsValue(errors.Errorf(`Unable to evaluate %s: item %d not found`, node, node.Index))
			}
			return AsValue(nil)
		}
		return item
	}
}

func (e *Evaluator) evalVarArgs(node *nodes.Call) ([]reflect.Value, error) {
	params := &VarArgs{
		Args:   []*Value{},
		KwArgs: map[string]*Value{},
	}
	for _, param := range node.Args {
		value := e.Eval(param)
		if value.IsError() {
			return nil, value
		}
		params.Args = append(params.Args, value)
	}

	for key, param := range node.Kwargs {
		value := e.Eval(param)
		if value.IsError() {
			return nil, value
		}
		params.KwArgs[key] = value
	}
	return []reflect.Value{reflect.ValueOf(params)}, nil
}

func (e *Evaluator) evalParams(node *nodes.Call, fn *Value) ([]reflect.Value, error) {
	// TODO: add the ability to detect the function signature and see if it wants a pointer to the evaluator

	args := node.Args
	t := fn.Val.Type()

	if len(args) != t.NumIn() && !(len(args) >= t.NumIn()-1 && t.IsVariadic()) {
		return nil, errors.Errorf(
			"function input argument count (%d) of '%s' must be equal to the calling argument count (%d)",
			t.NumIn(),
			node.String(),
			len(args),
		)
	}

	// Output arguments
	if t.NumOut() != 1 && t.NumOut() != 2 {
		msg := "'%s' must have exactly 1 or 2 output arguments, the second argument must be of type error"
		return nil, errors.Errorf(msg, node.String())
	}

	// Evaluate all parameters
	var parameters []reflect.Value

	numArgs := t.NumIn()
	isVariadic := t.IsVariadic()
	var functionArgument reflect.Type

	for index, argument := range args {
		evaluatedArgument := e.Eval(argument)
		if evaluatedArgument.IsError() {
			return nil, evaluatedArgument
		}

		if isVariadic {
			if index >= numArgs-1 {
				functionArgument = t.In(numArgs - 1).Elem()
			} else {
				functionArgument = t.In(index)
			}
		} else {
			functionArgument = t.In(index)
		}

		if functionArgument != typeOfValuePtr {
			// Function's argument is not a *gonja.Value, then we have to check whether input argument is of the same type as the function's argument
			if !isVariadic && functionArgument != reflect.TypeOf(evaluatedArgument.Interface()) && functionArgument.Kind() != reflect.Interface {
				return nil, errors.Errorf(
					"function input argument %d of '%s' must be of type %s or *gonja.Value (not %T)",
					index,
					node.String(),
					functionArgument.String(),
					evaluatedArgument.Interface(),
				)
			} else if functionArgument != reflect.TypeOf(evaluatedArgument.Interface()) && functionArgument.Kind() != reflect.Interface {
				return nil, errors.Errorf(
					"function variadic input argument of '%s' must be of type %s or *gonja.Value (not %T)",
					node.String(),
					functionArgument.String(),
					evaluatedArgument.Interface(),
				)
			}
			parameters = append(parameters, reflect.ValueOf(evaluatedArgument.Interface()))
		} else {
			parameters = append(parameters, reflect.ValueOf(evaluatedArgument))
		}
	}

	// Check if any of the values are invalid
	for _, p := range parameters {
		if p.Kind() == reflect.Invalid {
			return nil, errors.Errorf("calling a function using an invalid parameter")
		}
	}

	return parameters, nil
}
