package pystring

// IsASCII returns True if all characters in the string are ASCII, False otherwise.
func IsASCII(s string) bool {
	if len(s) == 0 {
		return true
	}

	for _, char := range s {
		if char > 127 {
			return false
		}
	}
	return true
}

// IsASCII returns True if all characters in the string are ASCII, False otherwise.
func (pys PyString) IsASCII() bool {
	return IsASCII(string(pys))
}
