// Package pystring provides Python-compatible string methods.
package pystring

type PyString string

func New(s string) PyString {
	return PyString(s)
}

func (pys PyString) String() string {
	return string(pys)
}
