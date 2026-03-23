package pystring

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/nikolalohinski/gonja/v2/builtins/methods/pyerrors"
)

// Token represents a token in the input string
type Token int

const (
	EOF Token = iota
	Characters
	ReplacementBlock
	Unknown
)

func (t Token) String() string {
	switch t {
	case EOF:
		return "EOF"
	case Characters:
		return "a"
	case ReplacementBlock:
		return "b"
	default:
		return "?"
	}
}

type pyStringScanner struct {
	index                      int
	input                      string
	automaticReplacementsFound int
	dialect                    Dialect
}

func NewScanner(s string, d Dialect) *pyStringScanner {
	return &pyStringScanner{
		input:   s,
		dialect: d,
	}
}

// Will return either
// EOF, "", nil
// Block, "...", nil
// Characters, "...", nil
// Unknown, "...", err - in case of malformed
func (s *pyStringScanner) Next() (Token, string, error) {
	if s.index >= len(s.input) {
		return EOF, "", nil
	}

	// find position of first, non escaped, left brace.
	// If not found, return the rest of the string as characters
	remainder := s.input[s.index:]

	braceIndex := indexFirstNonEscapedRune(remainder, '{')
	if braceIndex == -1 {
		s.index = len(s.input)
		return Characters, string(remainder), nil
	}

	// is the brace after position 0, then return all that came before it as characters first
	if braceIndex > 0 {
		s.index += braceIndex
		return Characters, string(remainder[:braceIndex]), nil
	}

	closingBrace := indexOfClosingBrace(remainder)
	if closingBrace == -1 {
		return Unknown, "", fmt.Errorf("%w: couldn't find closing brace", pyerrors.ErrValue)
	}
	s.index += closingBrace + 1

	var err error
	res := string(remainder[:closingBrace+1])
	res, err = s.maybePopulateAutomaticReplacement(res)
	if err != nil {
		return Unknown, "", err
	}

	return ReplacementBlock, res, nil
}

func (s *pyStringScanner) maybePopulateAutomaticReplacement(block string) (string, error) {
	foundManualFieldSpec := s.automaticReplacementsFound < 0
	foundAutomaticFieldSpec := s.automaticReplacementsFound > 0

	valueStopsAt := strings.IndexAny(block, ":}")
	if valueStopsAt != -1 && unicode.IsDigit(rune(block[1])) {
		if _, err := strconv.Atoi(block[1:valueStopsAt]); err == nil {
			foundManualFieldSpec = true
			s.automaticReplacementsFound = -1
		}
	}
	if block == "{}" || strings.HasPrefix(block, "{:") {
		foundAutomaticFieldSpec = true
		block = "{" + strconv.Itoa(s.automaticReplacementsFound) + strings.TrimPrefix(block, "{")
		s.automaticReplacementsFound++
	}

	if foundAutomaticFieldSpec && foundManualFieldSpec {
		return "", fmt.Errorf("%w: cannot switch from manual field specification to automatic field numbering", pyerrors.ErrValue)
	}

	// does the format block contain automatic replacement specifiers?
	formatDelim := strings.Index(block, ":")
	if formatDelim >= 0 {
		valueSpec := block[:formatDelim]
		formatSpec := block[formatDelim:]
		openBraceIndex := strings.Index(formatSpec, "{")
		if openBraceIndex == -1 {
			return block, nil
		}
		closeBraceIndex := strings.Index(formatSpec[openBraceIndex:], "}")
		if closeBraceIndex == -1 {
			return block, nil
		}

		beforeInnerReplacementBlock := formatSpec[:openBraceIndex]
		afterInnerReplacementBlock := formatSpec[openBraceIndex+closeBraceIndex+1:]
		innerReplacementBlock := formatSpec[openBraceIndex : openBraceIndex+closeBraceIndex+1]

		innerReplacementBlock, err := s.maybePopulateAutomaticReplacement(innerReplacementBlock)
		if err != nil {
			return "", err
		}
		return valueSpec + beforeInnerReplacementBlock + innerReplacementBlock + afterInnerReplacementBlock, nil
	}

	return block, nil
}

// find the matching close brace ignoring cases such as
// We will apply a strict interpretation of the format specifiers in validating this.
// {:{}} - using replacement block as the format specifier.
// {:{name}} - using replacement block as the format specifier.
// {:{0}} - using replacement block as the format specifier.
//
// TODO: Should we support "}" or "{" as a padding character
// python does not allow this but this more of an omission when reading the pep-3101
func indexOfClosingBrace(s string) int {
	openBraces := 0
	for i, r := range s {
		if r == '{' {
			openBraces++
		}
		if r == '}' {
			openBraces--
		}
		if r == '}' && openBraces == 0 {
			return i
		}
	}
	return -1
}

// indexFirstNonEscapedRune returns the index of the first occurrence of the non-escaped rune 'needle' in the string 's'.
// If the rune is not found, it returns -1.
// The function considers a rune as escaped if it is followed by the same rune.
// For example, in the string "hello{{", the first '{' is considered escaped because it is followed by another '{'.
// Parameters:
// - s: The input string to search in.
// - needle: The rune to search for.
// Returns:
// - The index of the first occurrence of the non-escaped rune.
// - -1 if the rune is not found.
func indexFirstNonEscapedRune(s string, needle rune) int {
	l := len(s)
	for i, r := range s {
		if r == needle && (i == l-1 || i < (l-1) && rune(s[i+1]) != needle) {
			return i
		}
	}
	return -1
}
