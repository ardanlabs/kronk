package pystring

// Strip returns a copy of the string with the leading and trailing characters removed. The chars argument is a string specifying the set of characters to be removed. If omitted or None, the chars argument defaults to removing whitespace. The chars argument is not a prefix or suffix; rather, all combinations of its values are stripped:
//
// >>>
// >>> '   spacious   '.strip(){}
// 'spacious'
// >>> 'www.example.com'.strip('cmowz.'){}
// 'example'
//
// The outermost leading and trailing chars argument values are stripped from the string. Characters are removed from the leading end until reaching a string character that is not contained in the set of characters in chars. A similar action takes place on the trailing end. For example:
//
// >>>
// >>> comment_string = '#....... Section 3.2.1 Issue #32 .......'
// >>> comment_string.strip('.#! '){}
// 'Section 3.2.1 Issue #32'
func Strip(s string, cutset string) string {
	return LStrip(RStrip(s, cutset), cutset)
}

// Strip returns a copy of the string with the leading and trailing characters removed. The chars argument is a string specifying the set of characters to be removed. If omitted or None, the chars argument defaults to removing whitespace. The chars argument is not a prefix or suffix; rather, all combinations of its values are stripped:
//
// >>>
// >>> '   spacious   '.strip(){}
// 'spacious'
// >>> 'www.example.com'.strip('cmowz.'){}
// 'example'
//
// The outermost leading and trailing chars argument values are stripped from the string. Characters are removed from the leading end until reaching a string character that is not contained in the set of characters in chars. A similar action takes place on the trailing end. For example:
//
// >>>
// >>> comment_string = '#....... Section 3.2.1 Issue #32 .......'
// >>> comment_string.strip('.#! '){}
// 'Section 3.2.1 Issue #32'
func (pys PyString) Strip(cutset string) PyString {
	return PyString(Strip(string(pys), cutset))
}
