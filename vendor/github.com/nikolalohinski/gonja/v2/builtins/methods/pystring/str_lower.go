package pystring

import (
	"strings"
	"unicode"
)

// Lower returns a copy of the string with all the cased characters converted to lowercase.
//
// The lowercasing algorithm used is described in section 3.13 ‘Default Case Folding’ of the Unicode Standard.
func Lower(s string) string {
	var res strings.Builder
	for _, char := range s {
		res.WriteRune(unicode.To(unicode.LowerCase, char))
	}

	return res.String()
}

// Lower returns a copy of the string with all the cased characters converted to lowercase.
//
// The lowercasing algorithm used is described in section 3.13 ‘Default Case Folding’ of the Unicode Standard.
func (pys PyString) Lower() PyString {
	return PyString(Lower(string(pys)))
}
