package pystring

import "unicode"

// IsPrintable returns True if all characters in the string are printable or the string is
// empty, False otherwise. Nonprintable characters are those characters defined in
// the Unicode character database as “Other” or “Separator”, excepting the ASCII
// space (0x20) which is considered printable. (Note that printable characters in
// this context are those which should not be escaped when repr() is invoked
// on a string. It has no bearing on the handling of strings written to
// sys.stdout or sys.stderr.)
func IsPrintable(s string) bool {
	if len(s) == 0 {
		return true
	}

	for _, char := range s {
		if !unicode.IsPrint(char) {
			return false
		}
	}
	return true
}

func (pys PyString) IsPrintable() bool {
	return IsPrintable(string(pys))
}
