package pystring

import "strings"

// RStrip returns a copy of the string with trailing characters removed. The chars
// argument is a string specifying the set of characters to be removed. If
// omitted or None, the chars argument defaults to removing whitespace. The
// chars argument is not a suffix; rather, all combinations of its values are stripped:
//
// >>>
// >>> '   spacious   '.rstrip(){}
// '   spacious'
// >>> 'mississippi'.rstrip('ipz'){}
// 'mississ'
//
// See func (pys PyString)removesuffix() for a method that will remove a single suffix string rather than all of a set of characters. For example:
//
// >>>
// >>> 'Monty Python'.rstrip(' Python'){}
// 'M'
// >>> 'Monty Python'.removesuffix(' Python'){}
// 'Monty'
func RStrip(s string, cutset string) string {
	if cutset == "" {
		cutset = " \t\n\r\v\f"
	}
	cutFrom := 0

	// Iterate over the slice of runes in reverse
	runes := []rune(s)
	for i := len(runes) - 1; i >= 0; i-- {
		if !strings.ContainsRune(cutset, runes[i]) {
			break
		}
		cutFrom = i
	}

	return s[:cutFrom]
}

// RStrip returns a copy of the string with trailing characters removed. The chars
// argument is a string specifying the set of characters to be removed. If
// omitted or None, the chars argument defaults to removing whitespace. The
// chars argument is not a suffix; rather, all combinations of its values are stripped:
//
// >>>
// >>> '   spacious   '.rstrip(){}
// '   spacious'
// >>> 'mississippi'.rstrip('ipz'){}
// 'mississ'
//
// See func (pys PyString)removesuffix() for a method that will remove a single suffix string rather than all of a set of characters. For example:
//
// >>>
// >>> 'Monty Python'.rstrip(' Python'){}
// 'M'
// >>> 'Monty Python'.removesuffix(' Python'){}
// 'Monty'
func (pys PyString) RStrip(cutset string) PyString {
	return PyString(RStrip(string(pys), cutset))
}
