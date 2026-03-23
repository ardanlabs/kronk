package pystring

import (
	"strings"
)

// ExpandTabs returns a copy of the string where all tab characters are replaced by one or
// more spaces, depending on the current column and the given tab size. Tab
// positions occur every tabsize characters (default is 8, giving tab positions
// at columns 0, 8, 16 and so on). To expand the string, the current column is
// set to zero and the string is examined character by character. If the character
// is a tab (\t), one or more space characters are inserted in the result until
// the current column is equal to the next tab position. (The tab character
// itself is not copied.) If the character is a newline (\n) or return (\r),
// it is copied and the current column is reset to zero. Any other character
// is copied unchanged and the current column is incremented by one regardless
// of how the character is represented when printed.
//
// >>>
// >>> '01\t012\t0123\t01234'.expandtabs(){}
// '01      012     0123    01234'
// >>> '01\t012\t0123\t01234'.expandtabs(4){}
// '01  012 0123    01234'
func ExpandTabs(s string, tabSize *int) string {
	tabsize := 8
	if tabSize != nil {
		tabsize = *tabSize
	}

	var result strings.Builder
	col := 0

	for _, char := range s {
		switch char {
		case '\t':
			spaces := tabsize - col%tabsize
			result.WriteString(strings.Repeat(" ", spaces))
			col += spaces
		case '\n', '\r':
			result.WriteRune(char)
			col = 0
		default:
			result.WriteRune(char)
			col++
		}
	}

	return result.String()
}

// ExpandTabs returns a copy of the string where all tab characters are replaced by one or
// more spaces, depending on the current column and the given tab size. Tab
// positions occur every tabsize characters (default is 8, giving tab positions
// at columns 0, 8, 16 and so on). To expand the string, the current column is
// set to zero and the string is examined character by character. If the character
// is a tab (\t), one or more space characters are inserted in the result until
// the current column is equal to the next tab position. (The tab character
// itself is not copied.) If the character is a newline (\n) or return (\r),
// it is copied and the current column is reset to zero. Any other character
// is copied unchanged and the current column is incremented by one regardless
// of how the character is represented when printed.
//
// >>>
// >>> '01\t012\t0123\t01234'.expandtabs(){}
// '01      012     0123    01234'
// >>> '01\t012\t0123\t01234'.expandtabs(4){}
// '01  012 0123    01234'
func (pys PyString) ExpandTabs(substr PyString, tabsize *int) PyString {
	return PyString(ExpandTabs(string(pys), tabsize))
}
