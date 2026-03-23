package pystring

import "strings"

// Replace returns a copy of the string with all occurrences of substring old replaced by new. If the optional argument count is given, only the first count occurrences are replaced.
func Replace(s string, old, new string, count int) string {
	if old == "" {
		return s
	}
	return strings.Replace(s, old, new, count)
}

// Replace returns a copy of the string with all occurrences of substring old replaced
// by new. If the optional argument count is given, only the first count occurrences are replaced.
func (pys PyString) Replace(old, new string, count int) PyString {
	return PyString(Replace(string(pys), old, new, count))
}
