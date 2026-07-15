package jinja

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

var (
	errBreak    = errors.New("break")
	errContinue = errors.New("continue")
)

// =============================================================================
// Scope
// =============================================================================

type scope struct {
	vars   map[string]Value
	parent *scope
}

func newScope(parent *scope) *scope {
	return &scope{vars: make(map[string]Value, 4), parent: parent}
}

func (s *scope) get(name string) (Value, bool) {
	if v, ok := s.vars[name]; ok {
		return v, true
	}
	if s.parent != nil {
		return s.parent.get(name)
	}
	return Undefined(), false
}

func (s *scope) set(name string, val Value) {
	s.vars[name] = val
}

// =============================================================================
// Evaluator
// =============================================================================

type evaluator struct {
	scope    *scope
	builtins *scope
	output   strings.Builder
}

func (e *evaluator) execNodes(nodes []node) error {
	for _, n := range nodes {
		if err := e.execNode(n); err != nil {
			return err
		}
	}
	return nil
}

func (e *evaluator) execNode(n node) error {
	switch nd := n.(type) {
	case *textNode:
		e.output.WriteString(nd.text)
		return nil

	case *outputNode:
		val, err := e.evalExpr(nd.expression)
		if err != nil {
			return err
		}
		e.output.WriteString(printValue(val))
		return nil

	case *ifNode:
		return e.execIf(nd)

	case *forNode:
		return e.execFor(nd)

	case *setNode:
		return e.execSet(nd)

	case *setBlockNode:
		return e.execSetBlock(nd)

	case *macroNode:
		return e.execMacro(nd)

	case *callNode:
		return e.execCall(nd)

	case *filterBlockNode:
		return e.execFilterBlock(nd)

	case *breakNode:
		return errBreak

	case *continueNode:
		return errContinue

	default:
		return fmt.Errorf("unknown node type %T", n)
	}
}

func (e *evaluator) execIf(n *ifNode) error {
	for _, branch := range n.branches {
		if branch.condition == nil {
			// else branch
			return e.execNodes(branch.body)
		}
		cond, err := e.evalExpr(branch.condition)
		if err != nil {
			return err
		}
		if cond.IsTruthy() {
			return e.execNodes(branch.body)
		}
	}
	return nil
}

func (e *evaluator) execFor(n *forNode) error {
	iterVal, err := e.evalExpr(n.iter)
	if err != nil {
		return err
	}

	var items []Value
	switch {
	case iterVal.IsList():
		items = iterVal.AsList().Items
	case iterVal.IsDict():
		d := iterVal.AsDict()
		for _, key := range d.Keys {
			items = append(items, NewString(key))
		}
	case iterVal.IsString():
		for _, r := range iterVal.AsString() {
			items = append(items, NewString(string(r)))
		}
	default:
		// Empty iteration.
	}

	// Apply filter if present.
	if n.filterExpr != nil {
		var filtered []Value
		for _, item := range items {
			child := newScope(e.scope)
			child.set(n.valVar, item)
			old := e.scope
			e.scope = child
			fv, err := e.evalExpr(n.filterExpr)
			e.scope = old
			if err != nil {
				return err
			}
			if fv.IsTruthy() {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	if len(items) == 0 {
		if n.elseBody != nil {
			return e.execNodes(n.elseBody)
		}
		return nil
	}

	total := int64(len(items))

	// Pre-allocate the loop dict and child scope once, then reuse them
	// across iterations to avoid per-iteration allocations.
	loop := NewDict()
	ld := loop.AsDict()
	ld.Set("length", NewInt(total))

	child := newScope(e.scope)
	child.set("loop", loop)

	for i, item := range items {
		// Set loop variable(s).
		if n.keyVar != "" {
			// Destructuring: for key, val in items
			if item.IsList() && item.AsList().Len() >= 2 {
				child.set(n.keyVar, item.AsList().Get(0))
				child.set(n.valVar, item.AsList().Get(1))
			} else {
				child.set(n.keyVar, NewInt(int64(i)))
				child.set(n.valVar, item)
			}
		} else {
			child.set(n.valVar, item)
		}

		// Update loop object for this iteration.
		idx := int64(i)
		ld.Data["index"] = NewInt(idx + 1)
		ld.Data["index0"] = NewInt(idx)
		ld.Data["revindex"] = NewInt(total - idx)
		ld.Data["revindex0"] = NewInt(total - idx - 1)
		ld.Data["first"] = NewBool(i == 0)
		ld.Data["last"] = NewBool(i == len(items)-1)

		if i > 0 {
			ld.Data["previtem"] = items[i-1]
		} else {
			ld.Data["previtem"] = Undefined()
		}
		if i < len(items)-1 {
			ld.Data["nextitem"] = items[i+1]
		} else {
			ld.Data["nextitem"] = Undefined()
		}

		// cycle function — capture idx by value for closure correctness.
		cycleIdx := idx
		ld.Data["cycle"] = NewCallable("loop.cycle", func(args []Value, kwargs map[string]Value) (Value, error) {
			if len(args) == 0 {
				return Undefined(), nil
			}
			return args[int(cycleIdx)%len(args)], nil
		})

		old := e.scope
		e.scope = child

		err := e.execNodes(n.body)
		e.scope = old

		if err != nil {
			if errors.Is(err, errBreak) {
				break
			}
			if errors.Is(err, errContinue) {
				continue
			}
			return err
		}
	}

	return nil
}

func (e *evaluator) execSet(n *setNode) error {
	val, err := e.evalExpr(n.value)
	if err != nil {
		return err
	}

	if n.namespace != "" {
		nsVal, ok := e.scope.get(n.namespace)
		if !ok || !nsVal.IsDict() {
			return fmt.Errorf("set: namespace %q not found or not a dict", n.namespace)
		}
		nsVal.AsDict().Set(n.target, val)
		return nil
	}

	e.scope.set(n.target, val)
	return nil
}

func (e *evaluator) execSetBlock(n *setBlockNode) error {
	bodyEval := evaluator{
		scope:    e.scope,
		builtins: e.builtins,
	}
	if err := bodyEval.execNodes(n.body); err != nil {
		return err
	}
	e.scope.set(n.target, NewString(bodyEval.output.String()))
	return nil
}

func (e *evaluator) execMacro(n *macroNode) error {
	// Capture the macro body and params.
	params := n.params
	body := n.body
	parentScope := e.scope

	fn := NewCallable(n.name, func(args []Value, kwargs map[string]Value) (Value, error) {
		child := newScope(parentScope)

		// Bind positional args.
		for i, param := range params {
			if i < len(args) {
				child.set(param.name, args[i])
			} else if v, ok := kwargs[param.name]; ok {
				child.set(param.name, v)
			} else if param.defaultVal != nil {
				defEval := evaluator{scope: child, builtins: e.builtins}
				defVal, err := defEval.evalExpr(param.defaultVal)
				if err != nil {
					return Undefined(), err
				}
				child.set(param.name, defVal)
			} else {
				child.set(param.name, Undefined())
			}
		}

		// Set caller if available.
		if caller, ok := kwargs["caller"]; ok {
			child.set("caller", caller)
		}

		macroEval := evaluator{
			scope:    child,
			builtins: e.builtins,
		}

		if err := macroEval.execNodes(body); err != nil {
			return Undefined(), err
		}

		return NewString(macroEval.output.String()), nil
	})

	e.scope.set(n.name, fn)
	return nil
}

func (e *evaluator) execCall(n *callNode) error {
	// Create a caller callable that renders the body.
	callerArgs := n.callerArgs
	body := n.body
	parentScope := e.scope

	caller := NewCallable("caller", func(args []Value, kwargs map[string]Value) (Value, error) {
		child := newScope(parentScope)
		for i, argName := range callerArgs {
			if i < len(args) {
				child.set(argName, args[i])
			}
		}

		callerEval := evaluator{
			scope:    child,
			builtins: e.builtins,
		}
		if err := callerEval.execNodes(body); err != nil {
			return Undefined(), err
		}
		return NewString(callerEval.output.String()), nil
	})

	// Evaluate the call expression with caller in kwargs.
	callEx, ok := n.expr.(*callExpr)
	if ok {
		fn, err := e.evalExpr(callEx.fn)
		if err != nil {
			return err
		}
		if !fn.IsCallable() {
			return fmt.Errorf("call: expression is not callable")
		}

		var args []Value
		for _, a := range callEx.args {
			av, err := e.evalExpr(a)
			if err != nil {
				return err
			}
			args = append(args, av)
		}

		kwargs := map[string]Value{"caller": caller}
		for _, kw := range callEx.kwargs {
			v, err := e.evalExpr(kw.value)
			if err != nil {
				return err
			}
			kwargs[kw.name] = v
		}

		result, err := fn.AsCallable().Fn(args, kwargs)
		if err != nil {
			return err
		}
		e.output.WriteString(printValue(result))
		return nil
	}

	// Fallback: evaluate expression and call with caller.
	fn, err := e.evalExpr(n.expr)
	if err != nil {
		return err
	}
	if !fn.IsCallable() {
		return fmt.Errorf("call: expression is not callable")
	}

	result, err := fn.AsCallable().Fn(nil, map[string]Value{"caller": caller})
	if err != nil {
		return err
	}
	e.output.WriteString(printValue(result))
	return nil
}

func (e *evaluator) execFilterBlock(n *filterBlockNode) error {
	// Render body to string.
	bodyEval := evaluator{
		scope:    e.scope,
		builtins: e.builtins,
	}
	if err := bodyEval.execNodes(n.body); err != nil {
		return err
	}
	bodyStr := bodyEval.output.String()

	// Apply the filter. The filter expression should be a name or filter chain.
	if fe, ok := n.filter.(*nameExpr); ok {
		filterVal, ok := lookupFilter(e.builtins, fe.name)
		if !ok {
			return fmt.Errorf("unknown filter %q", fe.name)
		}
		result, err := filterVal.AsCallable().Fn([]Value{NewString(bodyStr)}, nil)
		if err != nil {
			return err
		}
		e.output.WriteString(printValue(result))
		return nil
	}

	// For more complex filter expressions, just output the body.
	e.output.WriteString(bodyStr)
	return nil
}

// =============================================================================
// Expression evaluation
// =============================================================================

func (e *evaluator) evalExpr(ex expr) (Value, error) {
	switch n := ex.(type) {
	case *litExpr:
		return n.val, nil

	case *nameExpr:
		v, _ := e.scope.get(n.name)
		return v, nil

	case *attrExpr:
		return e.evalAttr(n)

	case *itemExpr:
		return e.evalItem(n)

	case *sliceExpr:
		return e.evalSlice(n)

	case *unaryExpr:
		return e.evalUnary(n)

	case *binExpr:
		return e.evalBinary(n)

	case *condExpr:
		return e.evalCond(n)

	case *callExpr:
		return e.evalCallExpr(n)

	case *filterExpr:
		return e.evalFilter(n)

	case *testExpr:
		return e.evalTestExpr(n)

	case *listExpr:
		items := make([]Value, len(n.items))
		for i, item := range n.items {
			v, err := e.evalExpr(item)
			if err != nil {
				return Undefined(), err
			}
			items[i] = v
		}
		return NewList(items), nil

	case *dictExpr:
		d := NewDict()
		dict := d.AsDict()
		for _, pair := range n.pairs {
			key, err := e.evalExpr(pair.key)
			if err != nil {
				return Undefined(), err
			}
			val, err := e.evalExpr(pair.value)
			if err != nil {
				return Undefined(), err
			}
			dict.Set(printValue(key), val)
		}
		return d, nil

	default:
		return Undefined(), fmt.Errorf("unknown expression type %T", ex)
	}
}

func (e *evaluator) evalAttr(n *attrExpr) (Value, error) {
	obj, err := e.evalExpr(n.obj)
	if err != nil {
		return Undefined(), err
	}

	// Dict attribute access.
	if obj.IsDict() {
		if v, ok := obj.AsDict().Get(n.attr); ok {
			return v, nil
		}

		// Dict methods.
		return e.dictMethod(obj, n.attr)
	}

	// String methods.
	if obj.IsString() {
		return e.stringMethod(obj, n.attr)
	}

	// List methods.
	if obj.IsList() {
		return e.listMethod(obj, n.attr)
	}

	return Undefined(), nil
}

func (e *evaluator) evalItem(n *itemExpr) (Value, error) {
	obj, err := e.evalExpr(n.obj)
	if err != nil {
		return Undefined(), err
	}

	key, err := e.evalExpr(n.key)
	if err != nil {
		return Undefined(), err
	}

	if obj.IsList() {
		idx := int(toInt64(key))
		list := obj.AsList()
		if idx < 0 {
			idx += list.Len()
		}
		if idx < 0 || idx >= list.Len() {
			return Undefined(), nil
		}
		return list.Get(idx), nil
	}

	if obj.IsDict() {
		k := printValue(key)
		if v, ok := obj.AsDict().Get(k); ok {
			return v, nil
		}
		return Undefined(), nil
	}

	if obj.IsString() {
		idx := int(toInt64(key))
		s := obj.AsString()
		runes := []rune(s)
		if idx < 0 {
			idx += len(runes)
		}
		if idx < 0 || idx >= len(runes) {
			return Undefined(), nil
		}
		return NewString(string(runes[idx])), nil
	}

	return Undefined(), nil
}

func (e *evaluator) evalSlice(n *sliceExpr) (Value, error) {
	obj, err := e.evalExpr(n.obj)
	if err != nil {
		return Undefined(), err
	}

	if obj.IsList() {
		list := obj.AsList()
		length := list.Len()
		start, stop, step := resolveSlice(n, e, length)

		// Fast path for the overwhelmingly common step==1 case
		// (e.g. messages[1:] in chat templates). A single allocation
		// of the exact size avoids the geometric growth of append.
		// Aliasing the source slice would be unsafe because List.Append
		// could later mutate beyond Len() into the backing array.
		if step == 1 {
			if start < 0 {
				start = 0
			}
			if stop > length {
				stop = length
			}
			if start >= stop {
				return NewList(nil), nil
			}
			items := make([]Value, stop-start)
			copy(items, list.Items[start:stop])
			return NewList(items), nil
		}

		var items []Value
		if step > 0 {
			// Pre-size to the maximum possible iteration count to
			// avoid append's geometric growth allocations.
			count := (stop - start + step - 1) / step
			if count > 0 {
				items = make([]Value, 0, count)
			}
			for i := start; i < stop; i += step {
				if i >= 0 && i < length {
					items = append(items, list.Items[i])
				}
			}
		} else if step < 0 {
			count := (start - stop - step - 1) / -step
			if count > 0 {
				items = make([]Value, 0, count)
			}
			for i := start; i > stop; i += step {
				if i >= 0 && i < length {
					items = append(items, list.Items[i])
				}
			}
		}
		return NewList(items), nil
	}

	if obj.IsString() {
		s := obj.AsString()

		// ASCII fast path. Most chat-template string slicing operates
		// on tag/role names or message content that is ASCII; for those
		// we can index by byte and skip the []rune conversion entirely.
		// This is the dominant cost in the kronk dense profile because
		// the system prompt can be tens of thousands of bytes.
		if isASCII(s) {
			length := len(s)
			start, stop, step := resolveSlice(n, e, length)

			if step == 1 {
				if start < 0 {
					start = 0
				}
				if stop > length {
					stop = length
				}
				if start >= stop {
					return NewString(""), nil
				}
				return NewString(s[start:stop]), nil
			}

			if step == -1 {
				if start >= length {
					start = length - 1
				}
				if stop < -1 {
					stop = -1
				}
				if start <= stop {
					return NewString(""), nil
				}
				buf := make([]byte, 0, start-stop)
				for i := start; i > stop; i-- {
					buf = append(buf, s[i])
				}
				return NewString(string(buf)), nil
			}
			// Fall through to the general path for unusual steps.
		}

		runes := []rune(s)
		length := len(runes)
		start, stop, step := resolveSlice(n, e, length)
		var result []rune
		if step > 0 {
			for i := start; i < stop; i += step {
				if i >= 0 && i < length {
					result = append(result, runes[i])
				}
			}
		} else if step < 0 {
			for i := start; i > stop; i += step {
				if i >= 0 && i < length {
					result = append(result, runes[i])
				}
			}
		}
		return NewString(string(result)), nil
	}

	return Undefined(), nil
}

// isASCII reports whether s contains only 7-bit ASCII bytes. Used to
// skip the []rune conversion in evalSlice's string branch when byte
// indexing produces the same result as code-point indexing.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func resolveSlice(n *sliceExpr, e *evaluator, length int) (int, int, int) {
	step := 1
	if n.step != nil {
		v, _ := e.evalExpr(n.step)
		step = int(toInt64(v))
		if step == 0 {
			step = 1
		}
	}

	var start, stop int

	if step > 0 {
		start = 0
		stop = length
	} else {
		start = length - 1
		stop = -length - 1
	}

	if n.start != nil {
		v, _ := e.evalExpr(n.start)
		start = int(toInt64(v))
		if start < 0 {
			start += length
		}
	}

	if n.stop != nil {
		v, _ := e.evalExpr(n.stop)
		stop = int(toInt64(v))
		if stop < 0 {
			stop += length
		}
	}

	return start, stop, step
}

func (e *evaluator) evalUnary(n *unaryExpr) (Value, error) {
	operand, err := e.evalExpr(n.operand)
	if err != nil {
		return Undefined(), err
	}

	switch n.op {
	case "not":
		return NewBool(!operand.IsTruthy()), nil
	case "-":
		if operand.IsInt() {
			return NewInt(-operand.AsInt()), nil
		}
		if operand.IsFloat() {
			return NewFloat(-operand.AsFloat()), nil
		}
		return NewInt(0), nil
	case "+":
		return operand, nil
	}

	return Undefined(), fmt.Errorf("unknown unary operator %q", n.op)
}

func (e *evaluator) evalBinary(n *binExpr) (Value, error) {
	// Short-circuit for and/or.
	if n.op == "and" {
		left, err := e.evalExpr(n.left)
		if err != nil {
			return Undefined(), err
		}
		if !left.IsTruthy() {
			return left, nil
		}
		return e.evalExpr(n.right)
	}

	if n.op == "or" {
		left, err := e.evalExpr(n.left)
		if err != nil {
			return Undefined(), err
		}
		if left.IsTruthy() {
			return left, nil
		}
		return e.evalExpr(n.right)
	}

	left, err := e.evalExpr(n.left)
	if err != nil {
		return Undefined(), err
	}

	right, err := e.evalExpr(n.right)
	if err != nil {
		return Undefined(), err
	}

	switch n.op {
	case "+":
		return addValues(left, right), nil
	case "-":
		return subValues(left, right), nil
	case "*":
		return mulValues(left, right), nil
	case "/":
		return divValues(left, right), nil
	case "//":
		return floorDivValues(left, right), nil
	case "%":
		return modValues(left, right), nil
	case "**":
		return powValues(left, right), nil
	case "~":
		return NewString(printValue(left) + printValue(right)), nil
	case "==":
		return NewBool(valuesEqual(left, right)), nil
	case "!=":
		return NewBool(!valuesEqual(left, right)), nil
	case "<":
		return NewBool(compareValues(left, right) < 0), nil
	case ">":
		return NewBool(compareValues(left, right) > 0), nil
	case "<=":
		return NewBool(compareValues(left, right) <= 0), nil
	case ">=":
		return NewBool(compareValues(left, right) >= 0), nil
	case "in":
		return NewBool(containsValue(right, left)), nil
	case "not in":
		return NewBool(!containsValue(right, left)), nil
	}

	return Undefined(), fmt.Errorf("unknown binary operator %q", n.op)
}

func (e *evaluator) evalCond(n *condExpr) (Value, error) {
	cond, err := e.evalExpr(n.condition)
	if err != nil {
		return Undefined(), err
	}

	if cond.IsTruthy() {
		return e.evalExpr(n.trueExpr)
	}

	if n.falseExpr != nil {
		return e.evalExpr(n.falseExpr)
	}

	return Undefined(), nil
}

func (e *evaluator) evalCallExpr(n *callExpr) (Value, error) {
	fn, err := e.evalExpr(n.fn)
	if err != nil {
		return Undefined(), err
	}

	if !fn.IsCallable() {
		return Undefined(), fmt.Errorf("%s is not callable: %s", callableName(n.fn), fn.String())
	}

	args := make([]Value, len(n.args))
	for i, a := range n.args {
		v, err := e.evalExpr(a)
		if err != nil {
			return Undefined(), err
		}
		args[i] = v
	}

	var kwargs map[string]Value
	if len(n.kwargs) > 0 {
		kwargs = make(map[string]Value, len(n.kwargs))
		for _, kw := range n.kwargs {
			v, err := e.evalExpr(kw.value)
			if err != nil {
				return Undefined(), err
			}
			kwargs[kw.name] = v
		}
	}

	return fn.AsCallable().Fn(args, kwargs)
}

func callableName(ex expr) string {
	switch n := ex.(type) {
	case *nameExpr:
		return n.name

	case *attrExpr:
		return callableName(n.obj) + "." + n.attr

	case *itemExpr:
		return callableName(n.obj) + "[item]"

	default:
		return ex.nodeType()
	}
}

func (e *evaluator) evalFilter(n *filterExpr) (Value, error) {
	input, err := e.evalExpr(n.expr)
	if err != nil {
		return Undefined(), err
	}

	// Look up filter in builtins.
	filterVal, ok := lookupFilter(e.builtins, n.name)
	if !ok {
		// Also check scope for user-defined filters.
		filterVal, ok = e.scope.get(n.name)
		if !ok {
			return Undefined(), fmt.Errorf("unknown filter %q", n.name)
		}
	}

	if !filterVal.IsCallable() {
		return Undefined(), fmt.Errorf("filter %q is not callable", n.name)
	}

	// Build args: input as first, then additional args.
	args := make([]Value, 1+len(n.args))
	args[0] = input
	for i, a := range n.args {
		v, err := e.evalExpr(a)
		if err != nil {
			return Undefined(), err
		}
		args[i+1] = v
	}

	var kwargs map[string]Value
	if len(n.kwargs) > 0 {
		kwargs = make(map[string]Value, len(n.kwargs))
		for _, kw := range n.kwargs {
			v, err := e.evalExpr(kw.value)
			if err != nil {
				return Undefined(), err
			}
			kwargs[kw.name] = v
		}
	}

	return filterVal.AsCallable().Fn(args, kwargs)
}

func (e *evaluator) evalTestExpr(n *testExpr) (Value, error) {
	val, err := e.evalExpr(n.expr)
	if err != nil {
		return Undefined(), err
	}

	var testArgs []Value
	for _, a := range n.args {
		v, err := e.evalExpr(a)
		if err != nil {
			return Undefined(), err
		}
		testArgs = append(testArgs, v)
	}

	result, err := evalTest(n.name, val, testArgs)
	if err != nil {
		return Undefined(), err
	}

	if n.negated {
		result = !result
	}

	return NewBool(result), nil
}

// =============================================================================
// String methods
// =============================================================================

func (e *evaluator) stringMethod(obj Value, method string) (Value, error) {
	s := obj.AsString()

	switch method {
	case "strip":
		return e.makeStringFunc("strip", func(args []Value, kwargs map[string]Value) (Value, error) {
			if len(args) > 0 {
				return NewString(strings.Trim(s, printValue(args[0]))), nil
			}
			return NewString(strings.TrimSpace(s)), nil
		}), nil

	case "lstrip":
		return e.makeStringFunc("lstrip", func(args []Value, kwargs map[string]Value) (Value, error) {
			if len(args) > 0 {
				return NewString(strings.TrimLeft(s, printValue(args[0]))), nil
			}
			return NewString(strings.TrimLeftFunc(s, func(r rune) bool {
				return r == ' ' || r == '\t' || r == '\n' || r == '\r'
			})), nil
		}), nil

	case "rstrip":
		return e.makeStringFunc("rstrip", func(args []Value, kwargs map[string]Value) (Value, error) {
			if len(args) > 0 {
				return NewString(strings.TrimRight(s, printValue(args[0]))), nil
			}
			return NewString(strings.TrimRightFunc(s, func(r rune) bool {
				return r == ' ' || r == '\t' || r == '\n' || r == '\r'
			})), nil
		}), nil

	case "split":
		return e.makeStringFunc("split", func(args []Value, kwargs map[string]Value) (Value, error) {
			if len(args) == 0 {
				parts := strings.Fields(s)
				items := make([]Value, len(parts))
				for i, p := range parts {
					items[i] = NewString(p)
				}
				return NewList(items), nil
			}
			sep := printValue(args[0])
			parts := strings.Split(s, sep)
			items := make([]Value, len(parts))
			for i, p := range parts {
				items[i] = NewString(p)
			}
			return NewList(items), nil
		}), nil

	case "startswith":
		return e.makeStringFunc("startswith", func(args []Value, kwargs map[string]Value) (Value, error) {
			if len(args) == 0 {
				return NewBool(false), nil
			}
			return NewBool(strings.HasPrefix(s, printValue(args[0]))), nil
		}), nil

	case "endswith":
		return e.makeStringFunc("endswith", func(args []Value, kwargs map[string]Value) (Value, error) {
			if len(args) == 0 {
				return NewBool(false), nil
			}
			return NewBool(strings.HasSuffix(s, printValue(args[0]))), nil
		}), nil

	case "replace":
		return e.makeStringFunc("replace", func(args []Value, kwargs map[string]Value) (Value, error) {
			if len(args) < 2 {
				return NewString(s), nil
			}
			old := printValue(args[0])
			newStr := printValue(args[1])
			count := -1
			if len(args) > 2 {
				count = int(toInt64(args[2]))
			}
			return NewString(strings.Replace(s, old, newStr, count)), nil
		}), nil

	case "upper":
		return e.makeStringFunc("upper", func(args []Value, kwargs map[string]Value) (Value, error) {
			return NewString(strings.ToUpper(s)), nil
		}), nil

	case "lower":
		return e.makeStringFunc("lower", func(args []Value, kwargs map[string]Value) (Value, error) {
			return NewString(strings.ToLower(s)), nil
		}), nil

	case "title":
		return e.makeStringFunc("title", func(args []Value, kwargs map[string]Value) (Value, error) {
			// Simple title case: capitalize first letter of each word.
			runes := []rune(s)
			inWord := false
			for i, r := range runes {
				if r == ' ' || r == '\t' || r == '\n' {
					inWord = false
				} else if !inWord {
					runes[i] = []rune(strings.ToUpper(string(r)))[0]
					inWord = true
				}
			}
			return NewString(string(runes)), nil
		}), nil

	case "capitalize":
		return e.makeStringFunc("capitalize", func(args []Value, kwargs map[string]Value) (Value, error) {
			if len(s) == 0 {
				return NewString(""), nil
			}
			runes := []rune(s)
			runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
			return NewString(string(runes)), nil
		}), nil

	case "count":
		return e.makeStringFunc("count", func(args []Value, kwargs map[string]Value) (Value, error) {
			if len(args) == 0 {
				return NewInt(0), nil
			}
			return NewInt(int64(strings.Count(s, printValue(args[0])))), nil
		}), nil

	case "find":
		return e.makeStringFunc("find", func(args []Value, kwargs map[string]Value) (Value, error) {
			if len(args) == 0 {
				return NewInt(-1), nil
			}
			return NewInt(int64(strings.Index(s, printValue(args[0])))), nil
		}), nil

	case "format":
		return e.makeStringFunc("format", func(args []Value, kwargs map[string]Value) (Value, error) {
			result := s
			for i, arg := range args {
				placeholder := fmt.Sprintf("{%d}", i)
				result = strings.ReplaceAll(result, placeholder, printValue(arg))
				result = strings.Replace(result, "{}", printValue(arg), 1)
			}
			return NewString(result), nil
		}), nil
	}

	return Undefined(), nil
}

func (e *evaluator) makeStringFunc(name string, fn func([]Value, map[string]Value) (Value, error)) Value {
	return NewCallable("str."+name, fn)
}

// =============================================================================
// List methods
// =============================================================================

func (e *evaluator) listMethod(obj Value, method string) (Value, error) {
	list := obj.AsList()

	switch method {
	case "append":
		return NewCallable("list.append", func(args []Value, kwargs map[string]Value) (Value, error) {
			if len(args) > 0 {
				list.Append(args[0])
			}
			return None(), nil
		}), nil

	case "pop":
		return NewCallable("list.pop", func(args []Value, kwargs map[string]Value) (Value, error) {
			if list.Len() == 0 {
				return Undefined(), fmt.Errorf("pop from empty list")
			}
			idx := list.Len() - 1
			if len(args) > 0 {
				idx = int(toInt64(args[0]))
				if idx < 0 {
					idx += list.Len()
				}
			}
			if idx < 0 || idx >= list.Len() {
				return Undefined(), fmt.Errorf("pop index out of range")
			}
			val := list.Items[idx]
			list.Items = append(list.Items[:idx], list.Items[idx+1:]...)
			return val, nil
		}), nil

	case "insert":
		return NewCallable("list.insert", func(args []Value, kwargs map[string]Value) (Value, error) {
			if len(args) < 2 {
				return None(), nil
			}
			idx := min(max(int(toInt64(args[0])), 0), list.Len())
			list.Items = append(list.Items, Undefined())
			copy(list.Items[idx+1:], list.Items[idx:])
			list.Items[idx] = args[1]
			return None(), nil
		}), nil
	}

	return Undefined(), nil
}

// =============================================================================
// Dict methods
// =============================================================================

func (e *evaluator) dictMethod(obj Value, method string) (Value, error) {
	d := obj.AsDict()

	switch method {
	case "items":
		return NewCallable("dict.items", func(args []Value, kwargs map[string]Value) (Value, error) {
			items := make([]Value, 0, d.Len())
			for _, key := range d.Keys {
				pair := NewList([]Value{NewString(key), d.Data[key]})
				items = append(items, pair)
			}
			return NewList(items), nil
		}), nil

	case "keys":
		return NewCallable("dict.keys", func(args []Value, kwargs map[string]Value) (Value, error) {
			items := make([]Value, 0, d.Len())
			for _, key := range d.Keys {
				items = append(items, NewString(key))
			}
			return NewList(items), nil
		}), nil

	case "values":
		return NewCallable("dict.values", func(args []Value, kwargs map[string]Value) (Value, error) {
			items := make([]Value, 0, d.Len())
			for _, key := range d.Keys {
				items = append(items, d.Data[key])
			}
			return NewList(items), nil
		}), nil

	case "get":
		return NewCallable("dict.get", func(args []Value, kwargs map[string]Value) (Value, error) {
			if len(args) == 0 {
				return Undefined(), nil
			}
			key := printValue(args[0])
			if v, ok := d.Get(key); ok {
				return v, nil
			}
			if len(args) > 1 {
				return args[1], nil
			}
			return None(), nil
		}), nil

	case "pop":
		return NewCallable("dict.pop", func(args []Value, kwargs map[string]Value) (Value, error) {
			if len(args) == 0 {
				return Undefined(), nil
			}
			key := printValue(args[0])
			v, ok := d.Get(key)
			if ok {
				delete(d.Data, key)
				newKeys := make([]string, 0, len(d.Keys)-1)
				for _, k := range d.Keys {
					if k != key {
						newKeys = append(newKeys, k)
					}
				}
				d.Keys = newKeys
				return v, nil
			}
			if len(args) > 1 {
				return args[1], nil
			}
			return Undefined(), nil
		}), nil

	case "update":
		return NewCallable("dict.update", func(args []Value, kwargs map[string]Value) (Value, error) {
			if len(args) > 0 && args[0].IsDict() {
				other := args[0].AsDict()
				for _, key := range other.Keys {
					d.Set(key, other.Data[key])
				}
			}
			return None(), nil
		}), nil
	}

	return Undefined(), nil
}

// =============================================================================
// Arithmetic helpers
// =============================================================================

func addValues(a, b Value) Value {
	if a.IsString() || b.IsString() {
		return NewString(printValue(a) + printValue(b))
	}
	if a.IsList() && b.IsList() {
		items := make([]Value, 0, a.AsList().Len()+b.AsList().Len())
		items = append(items, a.AsList().Items...)
		items = append(items, b.AsList().Items...)
		return NewList(items)
	}
	if a.IsFloat() || b.IsFloat() {
		return NewFloat(toFloat64(a) + toFloat64(b))
	}
	return NewInt(toInt64(a) + toInt64(b))
}

func subValues(a, b Value) Value {
	if a.IsFloat() || b.IsFloat() {
		return NewFloat(toFloat64(a) - toFloat64(b))
	}
	return NewInt(toInt64(a) - toInt64(b))
}

func mulValues(a, b Value) Value {
	if a.IsString() && b.IsInt() {
		return NewString(strings.Repeat(a.AsString(), int(b.AsInt())))
	}
	if a.IsFloat() || b.IsFloat() {
		return NewFloat(toFloat64(a) * toFloat64(b))
	}
	return NewInt(toInt64(a) * toInt64(b))
}

func divValues(a, b Value) Value {
	bf := toFloat64(b)
	if bf == 0 {
		return NewFloat(0)
	}
	return NewFloat(toFloat64(a) / bf)
}

func floorDivValues(a, b Value) Value {
	bi := toInt64(b)
	if bi == 0 {
		return NewInt(0)
	}
	return NewInt(toInt64(a) / bi)
}

func modValues(a, b Value) Value {
	bi := toInt64(b)
	if bi == 0 {
		return NewInt(0)
	}
	return NewInt(toInt64(a) % bi)
}

func powValues(a, b Value) Value {
	return NewFloat(math.Pow(toFloat64(a), toFloat64(b)))
}

func valuesEqual(a, b Value) bool {
	// Cross-type numeric comparison.
	if a.IsNumber() && b.IsNumber() {
		return toFloat64(a) == toFloat64(b)
	}
	// None == None.
	if a.IsNone() && b.IsNone() {
		return true
	}
	return a.Equals(b)
}
