package pystring

import "strings"

// ZFill returns a copy of the string left filled with ASCII '0' digits to make a
// string of length width. A leading sign prefix ('+'/'-') is handled by
// inserting the padding after the sign character rather than before. The
// original string is returned if width is less than or equal to len(s).
//
// For example:
//
// >>>
// >>> "42".zfill(5){}
// '00042'
// >>> "-42".zfill(5){}
// '-0042'
func ZFill(s string, width int) string {
	if len(s) >= width {
		return s
	}

	origLen := len(s)

	sign := ""
	if s[0] == '+' || s[0] == '-' {
		sign = s[:1]
		s = s[1:]
	}

	return sign + strings.Repeat("0", width-origLen) + s
}

// ZFill returns a copy of the string left filled with ASCII '0' digits to make a
// string of length width. A leading sign prefix ('+'/'-') is handled by
// inserting the padding after the sign character rather than before. The
// original string is returned if width is less than or equal to len(s).
//
// For example:
//
// >>>
// >>> "42".zfill(5){}
// '00042'
// >>> "-42".zfill(5){}
// '-0042'
func (pys PyString) ZFill(width int) PyString {
	return PyString(ZFill(string(pys), width))
}
