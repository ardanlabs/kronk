package pystring

import "unicode"

// IsNumeric returns True if all characters in the string are numeric and there is at least
// one character, False otherwise. Numeric characters include digit characters and all characters
// that have the Unicode numeric value property, e.g. U+2155, VULGAR FRACTION ONE FIFTH.
// Formally, numeric characters are those with the property value Numeric_Type=Digit,
// Numeric_Type=Decimal, or Numeric_Type=Numeric.
func IsNumeric(s string) bool {
	if len(s) == 0 {
		return false
	}

	for _, char := range s {
		if !unicode.IsDigit(char) && !unicode.IsNumber(char) {
			return false
		}
	}
	return true
}

// IsNumeric returns True if all characters in the string are numeric and there is at least
// one character, False otherwise. Numeric characters include digit characters and all characters
// that have the Unicode numeric value property, e.g. U+2155, VULGAR FRACTION ONE FIFTH.
// Formally, numeric characters are those with the property value Numeric_Type=Digit,
// Numeric_Type=Decimal, or Numeric_Type=Numeric.
func (pys PyString) IsNumeric() bool {
	return IsNumeric(string(pys))
}
