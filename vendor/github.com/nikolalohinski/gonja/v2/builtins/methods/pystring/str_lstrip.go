package pystring

import "strings"

// LStrip returns a copy of the string with leading characters removed. The chars
// argument is a string specifying the set of characters to be removed. If omitted
// or None, the chars argument defaults to removing whitespace. The chars argument
// is not a prefix; rather, all combinations of its values are stripped:
//
// >>>
// >>> '   spacious   '.lstrip(){}
// 'spacious   '
// >>> 'www.example.com'.lstrip('cmowz.'){}
// 'example.com'
//
// See func removeprefix() for a method that will remove a single
// prefix string rather than all of a set of characters. For example:
//
// >>>
// >>> 'Arthur: three!'.lstrip('Arthur: '){}
// 'ee!'
// >>> 'Arthur: three!'.removeprefix('Arthur: '){}
// 'three!'
func LStrip(s string, cutset string) string {
	if cutset == "" {
		cutset = " \t\n\r\v\f"
	}
	cutFrom := 0

	for i, c := range s {
		if !strings.ContainsRune(cutset, c) {
			break
		}
		cutFrom = i + 1
	}

	return s[cutFrom:]
}

// LStrip returns a copy of the string with leading characters removed. The chars argument is a string specifying the set of characters to be removed. If omitted or None, the chars argument defaults to removing whitespace. The chars argument is not a prefix; rather, all combinations of its values are stripped:
//
// >>>
// >>> '   spacious   '.lstrip(){}
// 'spacious   '
// >>> 'www.example.com'.lstrip('cmowz.'){}
// 'example.com'
//
// See func removeprefix() for a method that will remove a single prefix string rather than all of a set of characters. For example:
//
// >>>
// >>> 'Arthur: three!'.lstrip('Arthur: '){}
// 'ee!'
// >>> 'Arthur: three!'.removeprefix('Arthur: '){}
// 'three!'
func (pys PyString) LStrip(cutset string) PyString {
	return PyString(LStrip(string(pys), cutset))
}
