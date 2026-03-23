package pystring

import "unicode"

// IsAlnum returns True if all characters in the string are alphanumeric and there is at
// least one character, False otherwise.
func IsAlnum(s string) bool {
	if len(s) == 0 {
		return false
	}

	for _, char := range s {
		if !unicode.IsLetter(char) && !unicode.In(char, unicode.Nd) && !unicode.IsDigit(char) && !unicode.IsDigit(char) && !unicode.IsNumber(char) {
			return false
		}
	}
	return true
}

// IsAlnum returns True if all characters in the string are alphanumeric and there is at least one character, False otherwise.
func (pys PyString) IsAlnum() bool {
	return IsAlnum(string(pys))
}
