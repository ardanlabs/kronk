package pystring

import "strings"

// RemoveSuffix returns the string with the suffix removed. If the string ends with the suffix string and that suffix is not empty, return string) {} //[:-len(suffix)]. Otherwise, return a copy of the original string:
//
// >>>
// >>> 'MiscTests'.removesuffix('Tests'){}
// 'Misc'
// >>> 'TmpDirMixin'.removesuffix('Tests'){}
// 'TmpDirMixin'
//
// New in version 3.9.
func RemoveSuffix(s string, prefix string) string {
	if strings.HasSuffix(s, prefix) {
		return s[:len(s)-len(prefix)]
	}
	return s
}

// RemoveSuffix returns the string with the suffix removed. If the string ends with the suffix string and that suffix is not empty, return string) {} //[:-len(suffix)]. Otherwise, return a copy of the original string:
//
// >>>
// >>> 'MiscTests'.removesuffix('Tests'){}
// 'Misc'
// >>> 'TmpDirMixin'.removesuffix('Tests'){}
// 'TmpDirMixin'
//
// New in version 3.9.
func (pys PyString) RemoveSuffix(prefix string) PyString {
	return PyString(RemoveSuffix(string(pys), prefix))
}
