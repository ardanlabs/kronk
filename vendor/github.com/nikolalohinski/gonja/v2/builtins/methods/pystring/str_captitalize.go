package pystring

import (
	"strings"
	"unicode"
)

// Capitalize returns a copy of the string with its first character capitalized and the rest lowercased.
func Capitalize(s string) string {
	var res strings.Builder
	for idx, char := range s {
		if idx == 0 {
			res.WriteRune(unicode.To(unicode.UpperCase, char))
		} else {
			res.WriteRune(unicode.To(unicode.LowerCase, char))
		}
	}

	return res.String()
}

// Capitalize returns a copy of the string with its first character capitalized and the rest lowercased.
func (pys PyString) Capitalize() PyString {
	return PyString(Capitalize(string(pys)))
}
