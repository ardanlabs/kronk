package pystring

import "unicode"

// IsAlpha returns True if all characters in the string are alphabetic and there is at least one character, False otherwise. Alphabetic characters are those characters defined in the Unicode character database as “Letter”, i.e., those with general category property being one of “Lm”, “Lt”, “Lu”, “Ll”, or “Lo”. Note that this is different from the Alphabetic property defined in the section 4.10 ‘Letters, Alphabetic, and Ideographic’ of the Unicode Standard.
func IsAlpha(s string) bool {
	if len(s) == 0 {
		return false
	}

	for _, char := range s {
		if !unicode.IsLetter(char) {
			return false
		}
	}
	return true
}

// IsAlpha returns True if all characters in the string are alphabetic and there is at least one character, False otherwise. Alphabetic characters are those characters defined in the Unicode character database as “Letter”, i.e., those with general category property being one of “Lm”, “Lt”, “Lu”, “Ll”, or “Lo”. Note that this is different from the Alphabetic property defined in the section 4.10 ‘Letters, Alphabetic, and Ideographic’ of the Unicode Standard.
func (pys PyString) IsAlpha() bool {
	return IsAlpha(string(pys))
}
