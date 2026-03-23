package pystring

import "strings"

// Count returns the number of non-overlapping occurrences of substring sub in the range [start, end]. Optional arguments start and end are interpreted as in slice notation.
//
// If sub is empty, returns the number of empty strings between characters which is the length of the string plus one.
func Count(s, subStr string, start, end *int) int {
	s, _ = Idx(s, start, end)
	return strings.Count(s, subStr)
}

// Count returns the number of non-overlapping occurrences of substring sub in the range [start, end]. Optional arguments start and end are interpreted as in slice notation.
//
// If sub is empty, returns the number of empty strings between characters which is the length of the string plus one.
func (pys PyString) Count(substr PyString, start, end *int) int {
	return Count(string(pys), string(substr), start, end)
}
