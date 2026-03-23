package pystring

import "unicode"

// IsTitle returns True if the string is a titlecased string and there is at least one
// character, for example uppercase characters may only follow uncased characters
// and lowercase characters only cased ones. Returns False otherwise.
func IsTitle(s string) bool {
	if len(s) == 0 {
		return false
	}

	prevIsCased := false
	prevIsUpper := false
	for _, char := range s {
		if unicode.IsUpper(char) {
			if prevIsUpper {
				return false
			}
			if prevIsCased {
				return false
			}

			prevIsUpper = true
			prevIsCased = true
		} else if unicode.IsLower(char) {
			if !prevIsCased {
				return false
			}
			prevIsUpper = false
			prevIsCased = true
		} else {
			prevIsCased = false
		}
	}
	return true
}

// IsTitle returns True if the string is a titlecased string and there is at least one
// character, for example uppercase characters may only follow uncased characters
// and lowercase characters only cased ones. Returns False otherwise.
func (pys PyString) IsTitle() bool {
	return IsTitle(string(pys))
}
