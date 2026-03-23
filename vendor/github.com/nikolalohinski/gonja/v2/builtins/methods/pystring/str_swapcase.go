package pystring

import (
	"strings"
	"unicode"
)

// SwapCase returns a copy of the string with uppercase characters converted to lowercase
// and vice versa. Note that it is not necessarily true that
// s.swapcase().swapcase() == s.
func SwapCase(s string) string {
	var res strings.Builder
	for _, char := range s {
		if unicode.IsUpper(char) {
			res.WriteRune(unicode.ToLower(char))
		} else if unicode.IsLower(char) {
			res.WriteRune(unicode.ToUpper(char))
		} else {
			res.WriteRune(char)
		}
	}
	return res.String()
}

// SwapCase returns a copy of the string with uppercase characters converted to lowercase
// and vice versa. Note that it is not necessarily true that
// s.swapcase().swapcase() == s.
func (pys PyString) SwapCase() PyString {
	return PyString(SwapCase(string(pys)))
}
