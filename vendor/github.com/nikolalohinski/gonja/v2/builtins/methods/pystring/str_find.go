package pystring

import (
	"strings"
)

// Find returns the lowest index in the string where substring sub is found within the slice [start:end].
// Optional arguments start and end are interpreted as in slice notation.
// Returns -1 if sub is not found.
func Find(s, subStr string, start, end *int) int {
	s, _ = Idx(s, start, end)
	if s == "" {
		return 0
	}
	return strings.Index(s, subStr)
}

// Find returns the lowest index in the string where substring sub is found within the slice [start:end].
// Optional arguments start and end are interpreted as in slice notation.
// Returns -1 if sub is not found.
func (pys PyString) Find(substr PyString, start, end *int) int {
	return Find(string(pys), string(substr), start, end)
}
