package pystring

import "unicode"

// IsUpper returns True if all cased characters ) {} //[4] in the string are uppercase and there is at least one cased character, False otherwise.
//
// >>>
// >>> 'BANANA'.isupper(){}
// True
// >>> 'banana'.isupper(){}
// False
// >>> 'baNana'.isupper(){}
// False
// >>> ' '.isupper(){}
// False
func IsUpper(s string) bool {
	hasCased := false
	for _, char := range s {
		if unicode.IsLower(char) {
			return false
		} else if unicode.IsUpper(char) {
			hasCased = true
		}
	}
	return hasCased
}

// IsUpper returns True if all cased characters ) {} //[4] in the string are uppercase and there is at least one cased character, False otherwise.
//
// >>>
// >>> 'BANANA'.isupper(){}
// True
// >>> 'banana'.isupper(){}
// False
// >>> 'baNana'.isupper(){}
// False
// >>> ' '.isupper(){}
// False
func (pys PyString) IsUpper() bool {
	return IsUpper(string(pys))
}
