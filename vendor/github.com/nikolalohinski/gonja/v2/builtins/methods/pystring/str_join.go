package pystring

import (
	"fmt"
	"strings"
)

// JoinString returns a string which is the concatenation of the strings in iterable.
// A TypeError will be raised if there are any non-string values in iterable,
// including bytes objects. The separator between elements is the string
// providing this method.
func JoinString[T ~string](s T, it []T) string {
	strs := make([]string, len(it))
	for i, v := range it {
		strs[i] = string(v)
	}

	return strings.Join(strs, string(s))
}

func JoinStringer[T fmt.Stringer](s string, it []T) string {
	strs := make([]string, len(it))
	for i, v := range it {
		strs[i] = v.String()
	}

	return strings.Join(strs, s)
}

// JoinString returns a string which is the concatenation of the strings in iterable.
// A TypeError will be raised if there are any non-string values in iterable,
// including bytes objects. The separator between elements is the string.
func (pys PyString) JoinString(it []PyString) PyString {
	return PyString(JoinString(pys, it))
}

// JoinStringer returns a string which is the concatenation of the strings in iterable.
// A TypeError will be raised if there are any non-string values in iterable,
// including bytes objects. The separator between elements is the string.
func (pys PyString) JoinStringer(it []fmt.Stringer) PyString {
	return PyString(JoinStringer(string(pys), it))
}
