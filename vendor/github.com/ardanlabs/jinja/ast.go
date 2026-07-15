package jinja

// node is the interface for all AST nodes.
type node interface {
	nodeType() string
}

// expr is the interface for expression nodes (evaluable).
type expr interface {
	node
	exprNode()
}

// =========================================================================
// Template-level nodes
// =========================================================================

// textNode represents literal text output.
type textNode struct {
	text string
}

func (n *textNode) nodeType() string { return "text" }

// outputNode represents {{ expression }} output.
type outputNode struct {
	expression expr
}

func (n *outputNode) nodeType() string { return "output" }

// condBranch holds one branch of an if/elif/else chain.
// condition is nil for the else branch.
type condBranch struct {
	condition expr
	body      []node
}

// ifNode represents an if/elif/else chain.
type ifNode struct {
	branches []condBranch
}

func (n *ifNode) nodeType() string { return "if" }

// forNode represents a for loop.
type forNode struct {
	keyVar     string
	valVar     string
	iter       expr
	body       []node
	elseBody   []node
	filterExpr expr
	recursive  bool
}

func (n *forNode) nodeType() string { return "for" }

// setNode represents variable assignment.
type setNode struct {
	target    string
	namespace string
	value     expr
}

func (n *setNode) nodeType() string { return "set" }

// setBlockNode represents a block set assignment: {% set var %}body{% endset %}.
type setBlockNode struct {
	target string
	body   []node
}

func (n *setBlockNode) nodeType() string { return "setBlock" }

// macroParam holds a single macro parameter definition.
type macroParam struct {
	name       string
	defaultVal expr
}

// macroNode represents a macro definition.
type macroNode struct {
	name   string
	params []macroParam
	body   []node
}

func (n *macroNode) nodeType() string { return "macro" }

// callNode represents {% call(args) macro_expr %} body {% endcall %}.
type callNode struct {
	callerArgs []string
	expr       expr
	body       []node
}

func (n *callNode) nodeType() string { return "call" }

// filterBlockNode represents {% filter filterexpr %} body {% endfilter %}.
type filterBlockNode struct {
	filter expr
	body   []node
}

func (n *filterBlockNode) nodeType() string { return "filterBlock" }

// =========================================================================
// Expression nodes
// =========================================================================

// litExpr represents a literal value (string, int, float, bool, none).
type litExpr struct {
	val Value
}

func (n *litExpr) nodeType() string { return "literal" }
func (n *litExpr) exprNode()        {}

// nameExpr represents a variable reference.
type nameExpr struct {
	name string
}

func (n *nameExpr) nodeType() string { return "name" }
func (n *nameExpr) exprNode()        {}

// attrExpr represents attribute access (a.b).
type attrExpr struct {
	obj  expr
	attr string
}

func (n *attrExpr) nodeType() string { return "attr" }
func (n *attrExpr) exprNode()        {}

// itemExpr represents subscript access (a[b]).
type itemExpr struct {
	obj expr
	key expr
}

func (n *itemExpr) nodeType() string { return "item" }
func (n *itemExpr) exprNode()        {}

// sliceExpr represents slice access (a[start:stop:step]).
type sliceExpr struct {
	obj   expr
	start expr
	stop  expr
	step  expr
}

func (n *sliceExpr) nodeType() string { return "slice" }
func (n *sliceExpr) exprNode()        {}

// unaryExpr represents a unary operation (not, -, +).
type unaryExpr struct {
	op      string
	operand expr
}

func (n *unaryExpr) nodeType() string { return "unary" }
func (n *unaryExpr) exprNode()        {}

// binExpr represents a binary operation.
type binExpr struct {
	op    string
	left  expr
	right expr
}

func (n *binExpr) nodeType() string { return "binary" }
func (n *binExpr) exprNode()        {}

// condExpr represents an inline ternary (a if cond else b).
type condExpr struct {
	trueExpr  expr
	condition expr
	falseExpr expr
}

func (n *condExpr) nodeType() string { return "condExpr" }
func (n *condExpr) exprNode()        {}

// kwarg holds a single keyword argument.
type kwarg struct {
	name  string
	value expr
}

// callExpr represents a function/method call.
type callExpr struct {
	fn     expr
	args   []expr
	kwargs []kwarg
}

func (n *callExpr) nodeType() string { return "call" }
func (n *callExpr) exprNode()        {}

// filterExpr represents a pipe filter chain (value | filter(args)).
type filterExpr struct {
	expr   expr
	name   string
	args   []expr
	kwargs []kwarg
}

func (n *filterExpr) nodeType() string { return "filter" }
func (n *filterExpr) exprNode()        {}

// testExpr represents an is/is not test (value is testname).
type testExpr struct {
	expr    expr
	name    string
	negated bool
	args    []expr
}

func (n *testExpr) nodeType() string { return "test" }
func (n *testExpr) exprNode()        {}

// listExpr represents an array literal [a, b, c].
type listExpr struct {
	items []expr
}

func (n *listExpr) nodeType() string { return "list" }
func (n *listExpr) exprNode()        {}

// dictPair holds a single key-value pair in a dict literal.
type dictPair struct {
	key   expr
	value expr
}

// dictExpr represents a dict literal {k: v}.
type dictExpr struct {
	pairs []dictPair
}

func (n *dictExpr) nodeType() string { return "dict" }
func (n *dictExpr) exprNode()        {}
