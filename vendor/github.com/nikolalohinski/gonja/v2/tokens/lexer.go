package tokens

import (
	"fmt"

	// "encoding/json"
	"regexp"
	// "strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/nikolalohinski/gonja/v2/config"
	log "github.com/sirupsen/logrus"

	"github.com/nikolalohinski/gonja/v2/logging"
)

// EOF is an arbitraty value for End Of File
const rEOF = -1

var escapedStrings = map[string]string{
	`\"`: `"`,
	`\'`: `'`,
}

// var pattern = regexp.MustCompile(`(?m)(?P<key>\w+):\s+(?P<value>\w+)$`)

// lexFn represents the state of the scanner
// as a function that returns the next state.
type lexFn func() lexFn

// Lexer holds the state of the scanner.
type Lexer struct {
	Input string // the string being scanned.
	Start int    // start position of this item.
	Pos   int    // current position in the input.
	Width int    // width of last rune read from input.
	Line  int    // Current line in the input
	Col   int    // Current position in the line
	// Position Position // Current lexing position in the input
	Config               *config.Config // The lexer configuration
	Tokens               chan *Token    // channel of scanned tokens.
	delimiters           []rune
	RawControlStructures rawControlStructure
	rawEnd               *regexp.Regexp
}

// TODO: set from env
type rawControlStructure map[string]*regexp.Regexp

func escapeCharsClashingRegexp(s string) string {
	s = strings.ReplaceAll(s, "[", "\\[")
	s = strings.ReplaceAll(s, "]", "\\]")
	return s
}

// NewLexer creates a new scanner for the input string.
func NewLexer(input string, config *config.Config) *Lexer {
	return &Lexer{
		Input:  input,
		Tokens: make(chan *Token),
		Config: config,
		RawControlStructures: rawControlStructure{
			"raw":     regexp.MustCompile(fmt.Sprintf(`%s-?\s*endraw`, escapeCharsClashingRegexp(config.BlockStartString))),
			"comment": regexp.MustCompile(fmt.Sprintf(`%s-?\s*endcomment`, escapeCharsClashingRegexp(config.BlockStartString))),
		},
	}
}

func Lex(input string, config *config.Config) *Stream {
	l := NewLexer(input, config)
	go l.Run()
	return NewStream(l.Tokens)
}

// errorf returns an error token and terminates the scan
// by passing back a nil pointer that will be the next
// state, terminating Lexer.Run.
func (l *Lexer) errorf(format string, args ...any) lexFn {
	l.Tokens <- &Token{
		Type: Error,
		Val:  fmt.Sprintf(format, args...),
		Pos:  l.Pos,
	}
	return nil
}

// Position return the current position in the input
func (l *Lexer) Position() *Position {
	return &Position{
		Offset: l.Pos,
		Line:   l.Line,
		Column: l.Col,
	}
}

func (l *Lexer) Current() string {
	return l.Input[l.Start:l.Pos]
}

// Run lexes the input by executing state functions until
// the state is nil.
func (l *Lexer) Run() {
	for state := l.lexData; state != nil; {
		state = state()
	}
	close(l.Tokens) // No more tokens will be delivered.
}

// next returns the next rune in the input.
func (l *Lexer) next() (rune rune) {
	if l.Pos >= len(l.Input) {
		l.Width = 0
		return rEOF
	}
	rune, l.Width = utf8.DecodeRuneInString(l.Input[l.Pos:])
	l.Pos += l.Width
	if rune == '\n' {
		l.Line++
		l.Col = 1
	}
	return rune
}

// emit passes a Token back to the client.
func (l *Lexer) emit(t Type) {
	l.processAndEmit(t, nil)
}

func (l *Lexer) processAndEmit(t Type, fn func(string) string) {
	line, col := ReadablePosition(l.Start, l.Input)
	val := l.Input[l.Start:l.Pos]
	if fn != nil {
		val = fn(val)
	}
	l.Tokens <- &Token{
		Type: t,
		Val:  val,
		Pos:  l.Start,
		Line: line,
		Col:  col,
	}
	l.Start = l.Pos
}

// backup steps back one rune.
// Can be called only once per call of next.
func (l *Lexer) backup() {
	l.Pos -= l.Width
}

// peek returns but does not consume
// the next rune in the input.
func (l *Lexer) peek() rune {
	rune := l.next()
	l.backup()
	return rune
}

// accept consumes the next rune
// if it's from the valid set.
func (l *Lexer) accept(valid string) bool {
	if strings.ContainsRune(valid, l.next()) {
		return true
	}
	l.backup()
	return false
}

func (l *Lexer) pushDelimiter(r rune) {
	l.delimiters = append(l.delimiters, r)
}

func (l *Lexer) hasPrefix(prefix string) bool {
	return strings.HasPrefix(l.Input[l.Pos:], prefix)
}

func (l *Lexer) popDelimiter(r rune) bool {
	if len(l.delimiters) == 0 {
		l.errorf(`Unexpected delimiter "%c"`, r)
		return false
	}
	last := len(l.delimiters) - 1
	expected := l.delimiters[last]
	if r != expected {
		l.errorf(`Unbalanced delimiters, expected "%c", got "%c"`, expected, r)
		return false
	}
	// l.delimiters[last] = nil // Erase element (write zero value)
	l.delimiters = l.delimiters[:last]
	return true
}

// return whether or not we are expecting r as the next delimiter
func (l *Lexer) expectDelimiter(r rune) bool {
	if len(l.delimiters) == 0 {
		return false
	}
	expected := l.delimiters[len(l.delimiters)-1]
	return r == expected
}

func (l *Lexer) lexData() lexFn {
	for {
		if l.hasPrefix(l.Config.CommentStartString) {
			if l.Pos > l.Start {
				l.emit(Data)
			}
			return l.lexComment
		}

		if l.hasPrefix(l.Config.VariableStartString) {
			if l.Pos > l.Start {
				l.emit(Data)
			}
			return l.lexVariable
		}

		if l.hasPrefix(l.Config.BlockStartString) {
			if l.Pos > l.Start {
				l.emit(Data)
			}
			return l.lexBlock
		}

		if l.next() == rEOF {
			break
		}
	}
	// Correctly reached EOF.
	if l.Pos > l.Start {
		l.emit(Data)
	}
	l.emit(EOF) // Useful to make EOF a token.
	return nil  // Stop the run loop.
}

func (l *Lexer) remaining() string {
	return l.Input[l.Pos:]
}

func (l *Lexer) lexRaw() lexFn {
	loc := l.rawEnd.FindStringIndex(l.remaining())
	if loc == nil {
		return l.errorf(`Unable to find raw closing controlStructure`)
	}
	l.Pos += loc[0]
	l.emit(Data)
	l.rawEnd = nil
	return l.lexBlock
	// regexp.MustCompile(`(?m)(?P<key>\w+):\s+(?P<value>\w+)$`)
	// idx := pattern
}

func (l *Lexer) lexComment() lexFn {
	l.Pos += len(l.Config.CommentStartString)
	l.accept("-")
	l.emit(CommentBegin)
	i := strings.Index(l.Input[l.Pos:], l.Config.CommentEndString)
	if i < 0 {
		return l.errorf("unclosed comment")
	}
	l.Pos += i
	if l.Input[l.Pos-1] == '-' {
		l.Pos -= 1
	}
	l.emit(Data)
	l.accept("-")
	l.Pos += len(l.Config.CommentEndString)
	l.emit(CommentEnd)
	return l.lexData
}

func (l *Lexer) lexVariable() lexFn {
	if logging.Enabled() {
		log.WithFields(log.Fields{
			"pos":       l.Pos,
			"input":     l.Input,
			"remaining": l.remaining(),
		}).Trace("Lexer.lexVariable")
	}
	l.Pos += len(l.Config.VariableStartString)
	l.accept("-")
	l.emit(VariableBegin)
	return l.lexExpression
}

func (l *Lexer) lexVariableEnd() lexFn {
	l.accept("-")
	l.Pos += len(l.Config.VariableEndString)
	l.emit(VariableEnd)
	return l.lexData
}

func (l *Lexer) lexBlock() lexFn {
	l.Pos += len(l.Config.BlockStartString)
	l.accept("-")
	l.accept("+")
	l.emit(BlockBegin)
	for isSpace(l.peek()) {
		l.next()
	}
	if len(l.Current()) > 0 {
		l.emit(Whitespace)
	}
	controlStructure := l.nextIdentifier()
	l.emit(Name)
	re, exists := l.RawControlStructures[controlStructure]
	if exists {
		l.rawEnd = re
	}
	return l.lexExpression
}

func (l *Lexer) lexBlockEnd() lexFn {
	l.accept("-")
	l.accept("+")
	l.Pos += len(l.Config.BlockEndString)
	l.emit(BlockEnd)
	if l.rawEnd != nil {
		return l.lexRaw
	} else {
		return l.lexData
	}
}

func (l *Lexer) lexExpression() lexFn {
	if logging.Enabled() {
		log.WithFields(log.Fields{
			"pos":       l.Pos,
			"input":     l.Input,
			"remaining": l.remaining(),
		}).Trace("lexExpression")
	}
	for {
		if !l.expectDelimiter(l.peek()) {
			if l.hasPrefix(l.Config.VariableEndString) {
				return l.lexVariableEnd
			}

			if l.hasPrefix(l.Config.BlockEndString) {
				return l.lexBlockEnd
			}
		}

		r := l.next()
		if logging.Enabled() {
			log.WithFields(log.Fields{"rune": r}).Trace("lexExpression")
		}
		switch {
		case isEOF(r):
			return l.lexEOF
		case isSpace(r):
			return l.lexSpace
		case isNumeric(r):
			return l.lexNumber
		case r == '"' || r == '\'':
			l.backup()
			return l.lexString
		case r == ',':
			l.emit(Comma)
		case r == '|':
			l.emit(Pipe)
		case r == '+':
			if l.hasPrefix(l.Config.BlockEndString) {
				l.backup()
				return l.lexBlockEnd
			} else {
				l.emit(Addition)
			}
		case r == '-':
			if l.hasPrefix(l.Config.BlockEndString) {
				l.backup()
				return l.lexBlockEnd
			} else if l.hasPrefix(l.Config.VariableEndString) {
				l.backup()
				return l.lexVariableEnd
			} else {
				l.emit(Subtraction)
			}
		case r == '~':
			l.emit(Tilde)
		case r == ':':
			l.emit(Colon)
		case r == '.':
			l.emit(Dot)
		case r == '%':
			l.emit(Modulo)
		case r == '/':
			if l.accept("/") {
				l.emit(FloorDivision)
			} else {
				l.emit(Division)
			}
		case r == '<':
			if l.accept("=") {
				l.emit(LowerThanOrEqual)
			} else {
				l.emit(LowerThan)
			}
		case r == '>':
			if l.accept("=") {
				l.emit(GreaterThanOrEqual)
			} else {
				l.emit(GreaterThan)
			}
		case r == '*':
			if l.accept("*") {
				l.emit(Power)
			} else {
				l.emit(Multiply)
			}
		case r == '!':
			if l.accept("=") {
				l.emit(Ne)
			} else {
				// l.emit(Not)
				l.errorf(`Unexpected "!"`)
			}
		case r == '=':
			if l.accept("=") {
				l.emit(Equals)
			} else {
				l.emit(Assign)
			}
		case r == '(':
			l.emit(LeftParenthesis)
			l.pushDelimiter(')')
		case r == '{':
			l.emit(LeftBrace)
			l.pushDelimiter('}')
		case r == '[':
			l.emit(LeftBracket)
			l.pushDelimiter(']')
		case r == ')':
			if !l.popDelimiter(')') {
				return nil
			}
			l.emit(RightParenthesis)
		case r == '}':
			if !l.popDelimiter('}') {
				return nil
			}
			l.emit(RightBrace)
		case r == ']':
			if !l.popDelimiter(']') {
				return nil
			}
			l.emit(RightBracket)
		// in
		case r == 'i' && l.accept("n"):
			if !isSpace(l.peek()) {
				return l.lexIdentifier
			}
			l.emit(In)
		// is
		case r == 'i' && l.accept("s"):
			if !isSpace(l.peek()) {
				return l.lexIdentifier
			}
			l.emit(Is)
		// and
		case r == 'a' && l.accept("n"):
			if !(l.accept("d") && isSpace(l.peek())) {
				return l.lexIdentifier
			}
			l.emit(And)
		// or
		case r == 'o' && l.accept("r"):
			if !isSpace(l.peek()) {
				return l.lexIdentifier
			}
			l.emit(Or)
		// not
		case r == 'n' && l.accept("o"):
			if !(l.accept("t") && (isSpace(l.peek()) || l.peek() == '(')) {
				return l.lexIdentifier
			}
			l.emit(Not)
		case isAlphaNumeric(r):
			return l.lexIdentifier
		}
	}
}

func (l *Lexer) lexSpace() lexFn {
	for isSpace(l.peek()) {
		l.next()
	}
	l.emit(Whitespace)
	return l.lexExpression
}

func (l *Lexer) lexEOF() lexFn {
	if len(l.delimiters) > 0 {
		last := len(l.delimiters) - 1
		expected := l.delimiters[last]
		l.errorf(`Unbalanced delimiters, expected "%c", got EOF`, expected)
	}
	l.emit(EOF)
	return nil
}

func (l *Lexer) nextIdentifier() string {
	for {
		switch r := l.next(); {
		case isAlphaNumeric(r):
			// absorb.
		default:
			l.backup()
			// l.emit(Name)
			return l.Current()
		}
	}
}

func (l *Lexer) lexIdentifier() lexFn {
	l.nextIdentifier()
	l.emit(Name)
	return l.lexExpression
}

func (l *Lexer) lexNumber() lexFn {
	tokType := Integer
	for {
		switch r := l.next(); {
		case isNumeric(r):
			continue
		case r == '.':
			if n := l.peek(); isNumeric(n) || isSpace(n) {
				if tokType != Float {
					tokType = Float
					continue
				}
				return l.errorf("two dots in numeric token")
			}
		case isAlphaNumeric(r) && tokType == Integer:
			return l.lexIdentifier
		}
		l.backup()
		l.emit(tokType)
		return l.lexExpression
	}
}

func unescape(str string) string {
	str = str[1 : len(str)-1]
	for escaped, unescaped := range escapedStrings {
		str = strings.ReplaceAll(str, escaped, unescaped)
	}
	return str
}

func (l *Lexer) lexString() lexFn {
	quote := l.next() // should be either ' or "
	var prev rune
	near := make([]rune, 0)
	for r := l.next(); r != quote || prev == '\\'; r, prev = l.next(), r {
		// only keep near context of the current line in case of error
		if (len(near) == 0 || near[len(near)-1] != '\n') && r != rEOF {
			near = append(near, r)
		}
		if r == rEOF {
			return l.errorf(`%s`, string(near))
		}
	}
	l.processAndEmit(String, unescape)
	return l.lexExpression
}

// isSpace reports whether r is a space character.
func isSpace(r rune) bool {
	return r == ' ' || r == '\t'
}

// isAlphaNumeric reports whether r is an alphabetic, digit, or underscore.
func isAlphaNumeric(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isNumeric(r rune) bool {
	return unicode.IsDigit(r)
}

func isEOF(r rune) bool {
	return r == rEOF
}
