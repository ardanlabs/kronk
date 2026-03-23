package pystring

import (
	"strings"
	"unicode/utf8"
)

// Center returns the string centered in a string of length width. Padding is done using the specified
// fillchar (default is an ASCII space). The original string is returned if width
// is less than or equal to len(s).
func Center(s string, width int, fillchar rune) string {
	requiredPadding := int(width) - utf8.RuneCountInString(s)
	if requiredPadding <= 0 {
		return s
	}

	if fillchar == 0 {
		fillchar = ' '
	}

	rightPad := requiredPadding / 2
	leftPad := requiredPadding - rightPad
	return strings.Repeat(string(fillchar), leftPad) + s + strings.Repeat(string(fillchar), rightPad)
}

// Center returns the string centered in a string of length width. Padding is done using the specified
// fillchar (default is an ASCII space). The original string is returned if width
// is less than or equal to len(s).
func (pys PyString) Center(width int, fillchar rune) PyString {
	return PyString(Center(string(pys), width, fillchar))
}
