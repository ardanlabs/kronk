package pystring

import "strings"

// Partition splits the string at the first occurrence of sep, and returns a 3-tuple
// containing the part before the separator, the separator itself, and the part
// after the separator. If the separator is not found, returns a 3-tuple containing
// the string itself, followed by two empty strings.
func Partition(s string, delim string) (string, string, string) {
	before, after, ok := strings.Cut(s, delim)
	if !ok || delim == "" {
		return s, "", ""
	}
	return before, delim, after
}

// Partition splits the string at the first occurrence of sep, and returns a 3-tuple
// containing the part before the separator, the separator itself, and the part
// after the separator. If the separator is not found, returns a 3-tuple containing
// the string itself, followed by two empty strings.
func (pys PyString) Partition(delim string) (PyString, PyString, PyString) {
	r1, r2, r3 := Partition(string(pys), delim)
	return PyString(r1), PyString(r2), PyString(r3)
}
