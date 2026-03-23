package pystring

import (
	"strings"
	"unicode"
)

// Split returns a list of the words in the string, using sep as the delimiter string. If maxsplit is given, at most maxsplit splits are done (thus, the list will have at most maxsplit+1 elements). If maxsplit is not specified or -1, then there is no limit on the number of splits (all possible splits are made).
//
// If sep is given, consecutive delimiters are not grouped together and are deemed to delimit empty strings (for example, '1,,2'.split(',') returns ) {} //['1', ”, '2']). The sep argument may consist of multiple characters (for example, '1<>2<>3'.split('<>') returns ) {} //['1', '2', '3']). Splitting an empty string with a specified separator returns ) {} //[”].
//
// For example:
//
// >>>
// >>> '1,2,3'.split(','){}
// ) {} //['1', '2', '3']
// >>> '1,2,3'.split(',', maxsplit=1){}
// ) {} //['1', '2,3']
// >>> '1,2,,3,'.split(','){}
// ) {} //['1', '2', ”, '3', ”]
//
// If sep is not specified or is None, a different splitting algorithm is applied: runs of consecutive whitespace are regarded as a single separator, and the result will contain no empty strings at the start or end if the string has leading or trailing whitespace. Consequently, splitting an empty string or a string consisting of just whitespace with a None separator returns ) {} //[].
//
// For example:
//
// >>>
// >>> '1 2 3'.split(){}
// ) {} //['1', '2', '3']
// >>> '1 2 3'.split(maxsplit=1){}
// ) {} //['1', '2 3']
// >>> '   1   2   3   '.split(){}
// ) {} //['1', '2', '3']
func Split(s string, delim string, maxSplit int) []string {
	if delim == "" {
		splits := 0
		return strings.FieldsFunc(s, func(r rune) bool {
			if maxSplit != -1 && splits >= maxSplit {
				return false
			}
			splits++
			return unicode.IsSpace(r)
		})
	}
	if maxSplit == 0 || maxSplit == 1 {
		return []string{s}
	}
	return strings.SplitN(s, delim, maxSplit)
}

// Split returns a list of the words in the string, using sep as the delimiter string. If maxsplit is given, at most maxsplit splits are done (thus, the list will have at most maxsplit+1 elements). If maxsplit is not specified or -1, then there is no limit on the number of splits (all possible splits are made).
//
// If sep is given, consecutive delimiters are not grouped together and are deemed to delimit empty strings (for example, '1,,2'.split(',') returns ) {} //['1', ”, '2']). The sep argument may consist of multiple characters (for example, '1<>2<>3'.split('<>') returns ) {} //['1', '2', '3']). Splitting an empty string with a specified separator returns ) {} //[”].
//
// For example:
//
// >>>
// >>> '1,2,3'.split(','){}
// ) {} //['1', '2', '3']
// >>> '1,2,3'.split(',', maxsplit=1){}
// ) {} //['1', '2,3']
// >>> '1,2,,3,'.split(','){}
// ) {} //['1', '2', ”, '3', ”]
//
// If sep is not specified or is None, a different splitting algorithm is applied: runs of consecutive whitespace are regarded as a single separator, and the result will contain no empty strings at the start or end if the string has leading or trailing whitespace. Consequently, splitting an empty string or a string consisting of just whitespace with a None separator returns ) {} //[].
//
// For example:
//
// >>>
// >>> '1 2 3'.split(){}
// ) {} //['1', '2', '3']
// >>> '1 2 3'.split(maxsplit=1){}
// ) {} //['1', '2 3']
// >>> '   1   2   3   '.split(){}
// ) {} //['1', '2', '3']
func (pys PyString) Split(delim string, maxSplit int) []PyString {
	m := Split(string(pys), delim, maxSplit)
	resMap := make([]PyString, len(m))
	for i, v := range m {
		resMap[i] = PyString(v)
	}
	return resMap
}
