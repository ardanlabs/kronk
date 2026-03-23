// Package tokens provides lexical token types and the lexer.
package tokens

import "fmt"

// Type identifies the type of a token.
type Type int

// Known tokens
const (
	Error Type = iota
	Addition
	Assign
	Colon
	Comma
	Division
	Dot
	Equals
	FloorDivision
	GreaterThan
	GreaterThanOrEqual
	LeftBrace
	LeftBracket
	LeftParenthesis
	LowerThan
	LowerThanOrEqual
	Not
	Is
	In
	And
	Or
	Modulo
	Multiply
	Ne
	Pipe
	Power
	RightBrace
	RightBracket
	RightParenthesis
	Semicolon
	Subtraction
	Tilde
	Whitespace
	Float
	Integer
	Name
	String
	Operator
	BlockBegin
	BlockEnd
	VariableBegin
	VariableEnd
	RawBegin
	RawEnd
	CommentBegin
	CommentEnd
	Comment
	LinecontrolStructureBegin
	LinecontrolStructureEnd
	LinecommentBegin
	LinecommentEnd
	Linecomment
	Data
	Initial
	EOF
)

// Names maps token types to their human readable name.
var Names = map[Type]string{
	Error:                     "Error",
	Addition:                  "Add",
	Assign:                    "Assign",
	Colon:                     "Colon",
	Comma:                     "Comma",
	Division:                  "Div",
	Dot:                       "Dot",
	Equals:                    "Eq",
	Not:                       "Not",
	Is:                        "Is",
	In:                        "In",
	FloorDivision:             "Floordiv",
	GreaterThan:               "Gt",
	GreaterThanOrEqual:        "Gteq",
	LeftBrace:                 "Lbrace",
	LeftBracket:               "Lbracket",
	LeftParenthesis:           "Lparen",
	LowerThan:                 "Lt",
	LowerThanOrEqual:          "Lteq",
	And:                       "And",
	Or:                        "Or",
	Modulo:                    "Mod",
	Multiply:                  "Mul",
	Ne:                        "Ne",
	Pipe:                      "Pipe",
	Power:                     "Pow",
	RightBrace:                "Rbrace",
	RightBracket:              "Rbracket",
	RightParenthesis:          "Rparen",
	Semicolon:                 "Semicolon",
	Subtraction:               "Sub",
	Tilde:                     "Tilde",
	Whitespace:                "Whitespace",
	Float:                     "Float",
	Integer:                   "Integer",
	Name:                      "Name",
	String:                    "String",
	Operator:                  "Operator",
	BlockBegin:                "BlockBegin",
	BlockEnd:                  "BlockEnd",
	VariableBegin:             "VariableBegin",
	VariableEnd:               "VariableEnd",
	RawBegin:                  "RawBegin",
	RawEnd:                    "RawEnd",
	CommentBegin:              "CommentBegin",
	CommentEnd:                "CommentEnd",
	Comment:                   "Comment",
	LinecontrolStructureBegin: "LinecontrolStructureBegin",
	LinecontrolStructureEnd:   "LinecontrolStructureEnd",
	LinecommentBegin:          "LinecommentBegin",
	LinecommentEnd:            "LinecommentEnd",
	Linecomment:               "Linecomment",
	Data:                      "Data",
	Initial:                   "Initial",
	EOF:                       "EOF",
}

// Token represents a unit of lexing
type Token struct {
	Type                  Type
	Val                   string
	Pos                   int
	Line                  int
	Col                   int
	Trim                  bool
	RemoveFirstLineReturn bool
}

func (t Token) String() string {
	val := t.Val
	if len(val) > 1000 {
		val = fmt.Sprintf("%s...%s", val[:10], val[len(val)-5:])
	}

	return fmt.Sprintf("<Token[%s] Val='%s' Pos=%d Line=%d Col=%d>",
		Names[t.Type], val, t.Pos, t.Line, t.Col)
}
