package pystring

import "strings"

// RFind returns the highest index in the string where substring sub is found, such
// that sub is contained within s[start:end]. Optional arguments start
// and end are interpreted as in slice notation. Returns -1 on failure.
func RFind(s string, substr string, start, end *int) int {
	if substr == "" {
		return len(s)
	}
	if s == "" && substr == "" {
		return 0
	}

	s, _ = Idx(s, start, end)
	return strings.LastIndex(s, substr)
}

// RFind returns the highest index in the string where substring sub is found, such
// that sub is contained within s[start:end]. Optional arguments start
// and end are interpreted as in slice notation. Returns -1 on failure.
func (pys PyString) RFind(substr string, start, end *int) int {
	return RFind(string(pys), substr, start, end)
}
