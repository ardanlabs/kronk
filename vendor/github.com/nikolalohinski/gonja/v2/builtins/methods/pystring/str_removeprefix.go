package pystring

import "strings"

// RemovePrefix returns the string with the prefix removed. If the string starts with the prefix string, return string) {}
// [len(prefix):]. Otherwise, return a copy of the original string:
//
// >>>
// >>> 'TestHook'.removeprefix('Test'){}
// 'Hook'
// >>> 'BaseTestCase'.removeprefix('Test'){}
// 'BaseTestCase'
//
// New in version 3.9.
func RemovePrefix(s string, prefix string) string {
	if strings.HasPrefix(s, prefix) {
		return s[len(prefix):]
	}
	return s
}

// RemovePrefix returns the string with the prefix removed. If the string starts with the prefix string, return string) {} //[len(prefix):]. Otherwise, return a copy of the original string:
//
// >>>
// >>> 'TestHook'.removeprefix('Test'){}
// 'Hook'
// >>> 'BaseTestCase'.removeprefix('Test'){}
// 'BaseTestCase'
//
// New in version 3.9.
func (pys PyString) RemovePrefix(prefix string) PyString {
	return PyString(RemovePrefix(string(pys), prefix))
}
