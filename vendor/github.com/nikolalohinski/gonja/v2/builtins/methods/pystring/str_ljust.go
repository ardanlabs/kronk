package pystring

import (
	"strings"
	"unicode/utf8"
)

// LJust returns the string left justified in a string of length width. Padding is
// done using the specified fillchar (default is an ASCII space). The original
// string is returned if width is less than or equal to len(s).
func LJust(s string, width int, fillchar rune) string {
	requiredPadding := int(width) - utf8.RuneCountInString(s)
	if requiredPadding <= 0 {
		return s
	}

	if fillchar == 0 {
		fillchar = ' '
	}

	return s + strings.Repeat(string(fillchar), requiredPadding)
}

// LJust returns the string left justified in a string of length width. Padding is done using the specified fillchar (default is an ASCII space). The original string is returned if width is less than or equal to len(s).
func (pys PyString) LJust(width int, fillchar rune) PyString {
	return PyString(LJust(string(pys), width, fillchar))
}
