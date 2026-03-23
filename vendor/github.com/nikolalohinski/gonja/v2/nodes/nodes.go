// Package nodes provides AST node types for the template parser.
package nodes

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/nikolalohinski/gonja/v2/tokens"
	u "github.com/nikolalohinski/gonja/v2/utils"
)

// ----------------------------------------------------------------------------
// Interfaces
//
// There are 3 main classes of nodes: Expressions and type nodes,
// controlStructure nodes, and declaration nodes. The node names usually
// match the corresponding Go spec production names to which they
// correspond. The node fields correspond to the individual parts
// of the respective productions.
//
// All nodes contain position information marking the beginning of
// the corresponding source text segment; it is accessible via the
// Pos accessor method. Nodes may contain additional position info
// for language constructs where comments may be found between parts
// of the construct (typically any larger, parenthesized subpart).
// That position information is needed to properly position comments
// when printing the construct.

// Node is the interface implemented by all node types.
type Node interface {
	fmt.Stringer
	Position() *tokens.Token
}

// Expression represents an evaluable expression part
type Expression interface {
	Node
}

// ControlStructure represents a controlStructure block "{% %}"
type ControlStructure interface {
	Node
}

// Template is the root node of any template
type Template struct {
	Identifier string
	Nodes      []Node
	Blocks     BlockSet
	Macros     map[string]*Macro
	Parent     *Template
}

func (t *Template) Position() *tokens.Token { return t.Nodes[0].Position() }
func (t *Template) String() string {
	return fmt.Sprintf("template(%s)", t.Identifier)
}

func (t *Template) GetBlocks(name string) []*Wrapper {
	var blocks []*Wrapper
	if t.Parent != nil {
		blocks = t.Parent.GetBlocks(name)
	} else {
		blocks = []*Wrapper{}
	}
	block, exists := t.Blocks[name]
	if exists {
		blocks = append([]*Wrapper{block}, blocks...)
	}
	return blocks
}

type Trim struct {
	Left  bool
	Right bool
}

type Data struct {
	Data                                 *tokens.Token
	Trim                                 Trim
	RemoveFirstLineReturn                bool
	RemoveTrailingWhiteSpaceFromLastLine bool
}

func (d *Data) Position() *tokens.Token { return d.Data }

// String returns a string representation of the Data node.
func (d *Data) String() string {
	return fmt.Sprintf("data(%s)", u.Ellipsis(d.Data.Val, 20))
}

// A Comment node represents a single {# #} comment.
type Comment struct {
	Start *tokens.Token // Opening token
	Text  string        // Comment text
	End   *tokens.Token // Closing token
}

func (c *Comment) Position() *tokens.Token { return c.Start }

// String returns a string representation of the Comment node.
func (c *Comment) String() string {
	return fmt.Sprintf("comment(%s)", u.Ellipsis(c.Text, 20))
}

// Output represents a printable expression node {{ }}
type Output struct {
	Start       *tokens.Token
	Expression  Expression
	Condition   Expression
	Alternative Expression
	End         *tokens.Token
}

func (o *Output) Position() *tokens.Token { return o.Start }
func (o *Output) String() string {
	if o.Alternative != nil {
		return fmt.Sprintf("output(%s if %s else %s)", o.Expression, o.Condition, o.Alternative)
	}
	if o.Condition != nil {
		return fmt.Sprintf("output(%s if %s)", o.Expression, o.Condition)
	}
	return fmt.Sprintf("output(%s)", o.Expression)
}

type FilteredExpression struct {
	Expression Expression
	Filters    []*FilterCall
}

func (expr *FilteredExpression) Position() *tokens.Token {
	return expr.Expression.Position()
}
func (expr *FilteredExpression) String() string {
	return fmt.Sprintf("filtered_expression(%s)", expr.Expression)
}

type FilterCall struct {
	Token *tokens.Token

	Name   string
	Args   []Expression
	Kwargs map[string]Expression

	// filterFunc FilterFunction
}

type TestExpression struct {
	Expression Expression
	Test       *TestCall
}

func (expr *TestExpression) String() string {
	return fmt.Sprintf("%s %s", expr.Expression, expr.Test)
}
func (expr *TestExpression) Position() *tokens.Token {
	return expr.Expression.Position()
}

type TestCall struct {
	Token *tokens.Token

	Name   string
	Args   []Expression
	Kwargs map[string]Expression

	// testFunc TestFunction
}

func (tc *TestCall) String() string {
	return fmt.Sprintf("test(%s)", tc.Name)
}

type String struct {
	Location *tokens.Token
	Val      string
}

func (s *String) Position() *tokens.Token { return s.Location }
func (s *String) String() string          { return fmt.Sprintf("'%s'", s.Location.Val) }

type Integer struct {
	Location *tokens.Token
	Val      int
}

func (i *Integer) Position() *tokens.Token { return i.Location }
func (i *Integer) String() string          { return i.Location.Val }

type Float struct {
	Location *tokens.Token
	Val      float64
}

func (f *Float) Position() *tokens.Token { return f.Location }
func (f *Float) String() string          { return f.Location.Val }

type Bool struct {
	Location *tokens.Token
	Val      bool
}

func (b *Bool) Position() *tokens.Token { return b.Location }
func (b *Bool) String() string          { return b.Location.Val }

type Name struct {
	Name *tokens.Token
}

func (n *Name) Position() *tokens.Token { return n.Name }
func (n *Name) String() string {
	return n.Position().Val
}

type None struct {
	Location *tokens.Token
}

func (n *None) Position() *tokens.Token { return n.Location }
func (n *None) String() string {
	return n.Location.Val
}

type List struct {
	Location *tokens.Token
	Val      []Expression
}

func (l *List) Position() *tokens.Token { return l.Location }
func (l *List) String() string          { return l.Location.Val }

type Tuple struct {
	Location *tokens.Token
	Val      []Expression
}

func (t *Tuple) Position() *tokens.Token { return t.Location }
func (t *Tuple) String() string          { return t.Location.Val }

type Dict struct {
	Token *tokens.Token
	Pairs []*Pair
}

func (d *Dict) Position() *tokens.Token { return d.Token }
func (d *Dict) String() string          { return d.Token.Val }

type Pair struct {
	Key   Expression
	Value Expression
}

func (p *Pair) Position() *tokens.Token { return p.Key.Position() }
func (p *Pair) String() string {
	return fmt.Sprintf("%s: %s", p.Key, p.Value)
}

type Variable struct {
	Location *tokens.Token

	Parts []*VariablePart
}

func (v *Variable) Position() *tokens.Token { return v.Location }
func (v *Variable) String() string {
	parts := make([]string, 0, len(v.Parts))
	for _, p := range v.Parts {
		switch p.Type {
		case VarTypeInt:
			parts = append(parts, strconv.Itoa(p.I))
		case VarTypeIdent:
			parts = append(parts, p.S)
		default:
			panic("unimplemented")
		}
	}
	return strings.Join(parts, ".")
}

const (
	VarTypeInt = iota
	VarTypeIdent
)

type VariablePart struct {
	Type int
	S    string
	I    int

	IsFunctionCall bool
	// callingArgs    []functionCallArgument // needed for a function call, represents all argument nodes (Node supports nested function calls)
	Args   []Expression
	Kwargs map[string]Expression
}

func (vp *VariablePart) String() string {
	return fmt.Sprintf("variable_part(%s %d)", vp.S, vp.I)
}

type Call struct {
	Location *tokens.Token
	Func     Node
	Args     []Expression
	Parent   Node
	Kwargs   map[string]Expression
}

func (c *Call) Position() *tokens.Token { return c.Location }
func (c *Call) String() string {
	return fmt.Sprintf("call(%s, %s)", c.Args, c.Kwargs)
}

type GetItem struct {
	Location *tokens.Token
	Node     Node
	Arg      Node
}

func (g *GetItem) Position() *tokens.Token { return g.Location }
func (g *GetItem) String() string {
	return fmt.Sprintf("%s[%s]", g.Node, g.Arg)
}

type GetSlice struct {
	Location *tokens.Token
	Node     Node
	Start    Node
	End      Node
}

func (g *GetSlice) Position() *tokens.Token { return g.Location }
func (g *GetSlice) String() string {
	return fmt.Sprintf("%s[%s:%s]", g.Node, g.Start, g.End)
}

type GetAttribute struct {
	Location  *tokens.Token
	Node      Node
	Attribute string
	Index     int
}

func (g *GetAttribute) Position() *tokens.Token { return g.Location }
func (g *GetAttribute) String() string {
	var param string
	if g.Attribute != "" {
		param = g.Attribute
	} else {
		param = strconv.Itoa(g.Index)
	}
	return fmt.Sprintf("%s.%s", g.Node, param)
}

type Negation struct {
	Term     Expression
	Operator *tokens.Token
}

func (n *Negation) Position() *tokens.Token { return n.Operator }
func (n *Negation) String() string {
	return fmt.Sprintf("!%s", n.Term)
}

type UnaryExpression struct {
	Negative bool
	Term     Expression
	Operator *tokens.Token
}

func (u *UnaryExpression) Position() *tokens.Token { return u.Operator }
func (u *UnaryExpression) String() string {
	t := u.Operator

	return fmt.Sprintf("%s%s", t.Val, u.Term)
}

type BinaryExpression struct {
	Left     Expression
	Right    Expression
	Operator *BinOperator
}

func (expr *BinaryExpression) Position() *tokens.Token { return expr.Left.Position() }
func (expr *BinaryExpression) String() string {
	return fmt.Sprintf("%s %s %s", expr.Left, expr.Operator.Token.Val, expr.Right)
}

type BinOperator struct {
	Token *tokens.Token
}

func (op BinOperator) Position() *tokens.Token { return op.Token }
func (op BinOperator) String() string          { return op.Token.String() }

type ControlStructureBlock struct {
	Location         *tokens.Token
	Name             string
	ControlStructure ControlStructure
}

func (s ControlStructureBlock) Position() *tokens.Token { return s.Location }
func (s ControlStructureBlock) String() string {
	return fmt.Sprintf("%s %s", s.Name, s.ControlStructure)
}

type Wrapper struct {
	Location *tokens.Token
	Nodes    []Node
	EndTag   string
	Trim     *Trim
	LStrip   bool
}

func (w Wrapper) Position() *tokens.Token { return w.Location }
func (w Wrapper) String() string {
	return fmt.Sprintf("wrapper(%s,%s)", w.Nodes, w.EndTag)
}

type Macro struct {
	Location *tokens.Token
	Name     string
	Kwargs   []*Pair
	Wrapper  *Wrapper
}

func (m *Macro) Position() *tokens.Token { return m.Location }
func (m *Macro) String() string {
	return fmt.Sprintf("%s(%s)", m.Name, m.Kwargs)
}

type Error struct {
	Location *tokens.Token
	Error    error
}

func (c *Error) Position() *tokens.Token { return c.Location }
func (c *Error) String() string {
	return c.Error.Error()
}
