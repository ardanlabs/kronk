package pystring

import "unicode"

// IsLower returns True if all cased characters in the string are lowercase and there is at least one cased character, False otherwise.
func IsLower(s string) bool {
	hasCased := false
	for _, char := range s {
		if unicode.IsUpper(char) {
			return false
		} else if unicode.IsLower(char) {
			hasCased = true
		}
	}
	return hasCased
}

// IsLower returns True if all cased characters in the string are lowercase and there is at least one cased character, False otherwise.
func (pys PyString) IsLower() bool {
	return IsLower(string(pys))
}
