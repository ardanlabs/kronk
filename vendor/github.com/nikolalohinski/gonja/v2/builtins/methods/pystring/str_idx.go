package pystring

import "github.com/nikolalohinski/gonja/v2/builtins/methods/pyerrors"

// Idx replicates indexing behavior in python. As such it supports negative
// indexing and don't crash on out of bound indexes.
func Idx(s string, start, end *int) (string, error) {
	sLen := len(s)
	actualStart := 0
	actualEnd := len(s)
	if end != nil {
		if *end < sLen {
			actualEnd = *end
		}
		if *end < 0 {
			actualEnd = sLen + *end
		}
		if actualEnd < 0 {
			actualEnd = 0
		}
	}

	if start != nil {
		if *start < 0 {
			actualStart = sLen + *start
		} else {
			actualStart = *start
		}
		if actualStart < 0 {
			actualStart = 0
		}
	}

	if actualEnd < actualStart || actualStart < 0 || actualEnd < 0 || actualStart > sLen || actualEnd > sLen {
		return "", pyerrors.ErrIndex
	}

	return s[actualStart:actualEnd], nil
}

// Idx replicates indexing behavior in python. As such it supports negative
// indexing and don't crash on out of bound indexes.
func (pys PyString) Idx(start, end *int) (PyString, error) {
	r, err := Idx(string(pys), start, end)
	return PyString(r), err
}
