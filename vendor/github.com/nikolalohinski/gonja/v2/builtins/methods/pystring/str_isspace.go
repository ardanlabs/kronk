package pystring

import "unicode"

// IsSpace returns True if there are only whitespace characters in the string and there is at least one character, False otherwise.
//
// A character is whitespace if in the Unicode character database (see unicodedata), either its general category is Zs (“Separator, space”), or its bidirectional class is one of WS, B, or S.
func IsSpace(s string) bool {
	if len(s) == 0 {
		return false
	}

	for _, char := range s {
		if !unicode.IsSpace(char) {
			return false
		}

	}
	return true
}

// IsSpace returns True if there are only whitespace characters in the string and there is at least one character, False otherwise.
//
// A character is whitespace if in the Unicode character database (see unicodedata), either its general category is Zs (“Separator, space”), or its bidirectional class is one of WS, B, or S.
func (pys PyString) IsSpace() bool {
	return IsSpace(string(pys))
}
