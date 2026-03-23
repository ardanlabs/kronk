package pystring

import "unicode"

// IsDecimal returns True if all characters in the string are decimal characters and there is at least one character, False otherwise. Decimal characters are those that can be used to form numbers in base 10, e.g. U+0660, ARABIC-INDIC DIGIT ZERO. Formally a decimal character is a character in the Unicode General Category “Nd”.
func IsDecimal(s string) bool {
	if len(s) == 0 {
		return false
	}

	for _, char := range s {
		if !unicode.In(char, unicode.Nd) {
			return false
		}
	}
	return true
}

// IsDecimal returns True if all characters in the string are decimal characters and there is at least one character, False otherwise. Decimal characters are those that can be used to form numbers in base 10, e.g. U+0660, ARABIC-INDIC DIGIT ZERO. Formally a decimal character is a character in the Unicode General Category “Nd”.
func (pys PyString) IsDecimal() bool {
	return IsDecimal(string(pys))
}
