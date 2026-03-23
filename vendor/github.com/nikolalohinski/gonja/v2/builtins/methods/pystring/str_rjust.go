package pystring

import (
	"strings"
	"unicode/utf8"
)

// RJust returns the string right justified in a string of length width. Padding is done using the specified fillchar (default is an ASCII space). The original string is returned if width is less than or equal to len(s).
func RJust(s string, width int, fillchar rune) string {
	requiredPadding := int(width) - utf8.RuneCountInString(s)
	if requiredPadding <= 0 {
		return s
	}

	if fillchar == 0 {
		fillchar = ' '
	}

	return strings.Repeat(string(fillchar), requiredPadding) + s
}

// RJust returns the string right justified in a string of length width. Padding is done using the specified fillchar (default is an ASCII space). The original string is returned if width is less than or equal to len(s).
func (pys PyString) RJust(width int, fillchar rune) PyString {
	return PyString(RJust(string(pys), width, fillchar))
}
