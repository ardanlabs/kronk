package pystring

import "strings"

// RPartition splits the string at the last occurrence of sep, and returns a 3-tuple containing the part before the separator,
// the separator itself, and the part after the separator. If the separator is not found, returns a 3-tuple containing
// two empty strings, followed by the string itself.
func RPartition(s string, delim string) (string, string, string) {
	i := strings.LastIndex(s, delim)
	if i == -1 || delim == "" {
		return "", "", s
	}
	return s[:i], delim, s[i+len(delim):]
}

// RPartition splits the string at the last occurrence of sep, and returns a 3-tuple containing the part before the separator, the separator itself, and the part after the separator. If the separator is not found, returns a 3-tuple containing two empty strings, followed by the string itself.
func (pys PyString) RPartition(delim string) (PyString, PyString, PyString) {
	r1, r2, r3 := RPartition(string(pys), delim)
	return PyString(r1), PyString(r2), PyString(r3)
}
