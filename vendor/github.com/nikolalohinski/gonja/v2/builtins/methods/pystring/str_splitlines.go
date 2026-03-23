package pystring

import "strings"

// SplitLines returns a list of the lines in the string, breaking at line boundaries. Line breaks are not included in the resulting list unless keepends is given and true.
//
// This method splits on the following line boundaries. In particular, the boundaries are a superset of universal newlines.
//
// \n - Line Feed
// \r - Carriage Return
// \r\n - Carriage Return + Line Feed
// \v or \x0b - Line Tabulation
// \f or \x0c - Form Feed
// \x1c - File Separator
// \x1d - Group Separator
// \x1e - Record Separator
// \x85 - Next Line (C1 Control Code)
// \u2028 - Line Separator
// \u2029 - Paragraph Separator
// Unlike split() when a delimiter string sep is given, this method returns an empty list for the empty string, and a terminal line break does not result in an extra line:
func SplitLines(s string, keepends bool) []string {
	res := []string{}
	if s == "" {
		return res
	}

	offset := 0
	if keepends {
		offset = +1
	}

	cutset := "\n\r\v\f\x1c\x1d\x1e\u0085\u2028\u2029"
	for index := strings.IndexAny(s, cutset); index != -1; index = strings.IndexAny(s, cutset) {
		offsetMultiplier := 1
		if s[index] == '\r' && index+1 < len(s) && s[index+1] == '\n' {
			offsetMultiplier = 2
		}

		res = append(res, s[:index+offset*offsetMultiplier])
		s = s[index+offsetMultiplier:]
	}
	if s != "" {
		res = append(res, s)
	}

	return res
}

// SplitLines returns a list of the lines in the string, breaking at line boundaries. Line breaks are not included in the resulting list unless keepends is given and true.
//
// This method splits on the following line boundaries. In particular, the boundaries are a superset of universal newlines.
//
// \n - Line Feed
// \r - Carriage Return
// \r\n - Carriage Return + Line Feed
// \v or \x0b - Line Tabulation
// \f or \x0c - Form Feed
// \x1c - File Separator
// \x1d - Group Separator
// \x1e - Record Separator
// \x85 - Next Line (C1 Control Code)
// \u2028 - Line Separator
// \u2029 - Paragraph Separator
// Unlike split() when a delimiter string sep is given, this method returns an empty list for the empty string, and a terminal line break does not result in an extra line:
func (pys PyString) SplitLines(keepends bool) []PyString {
	m := SplitLines(string(pys), keepends)
	resMap := make([]PyString, len(m))
	for i, v := range m {
		resMap[i] = PyString(v)
	}
	return resMap
}
