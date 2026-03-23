package pystring

import "strings"

// RSplit returns a list of the words in the string, using sep as the delimiter string.
// If maxsplit is given, at most maxsplit splits are done, the rightmost ones.
// If sep is not specified or None, any whitespace string is a separator. Except
// for splitting from the right, rsplit() behaves like split() which is described
// in detail below.
func RSplit(s string, delim string, maxSplit int) []string {
	if delim == "" {
		return []string{s}
	}
	if maxSplit == 0 || maxSplit == 1 {
		return []string{s}
	}
	if maxSplit < 0 {
		return strings.Split(s, delim)
	}

	if possibleSplits := Count(s, delim, nil, nil) + 1; possibleSplits <= maxSplit {
		maxSplit = possibleSplits
	}
	res := make([]string, maxSplit)
	for i := maxSplit - 1; i >= 0; i-- {
		idx := strings.LastIndex(s, delim)
		if idx == -1 {
			res[i] = s
			break
		}

		res[i] = s[idx+1:]
		s = s[:idx]
	}
	return res
}

// RSplit returns a list of the words in the string, using sep as the delimiter string.
// If maxsplit is given, at most maxsplit splits are done, the rightmost ones.
// If sep is not specified or None, any whitespace string is a separator. Except
// for splitting from the right, rsplit() behaves like split() which is described
// in detail below.
func (pys PyString) RSplit(delim string, maxSplit int) []PyString {
	m := RSplit(string(pys), delim, maxSplit)
	resMap := make([]PyString, len(m))
	for i, v := range m {
		resMap[i] = PyString(v)
	}
	return resMap
}
