// Package pybool provides Python-compatible boolean methods.
package pybool

type PyBool bool

func New(b bool) PyBool {
	return PyBool(b)
}

func (pb PyBool) Bool() bool {
	return bool(pb)
}

func (pb PyBool) MarshalJSON() ([]byte, error) {
	if bool(pb) {
		return []byte("true"), nil
	}
	return []byte("false"), nil
}

func (pb PyBool) String() string {
	if bool(pb) {
		return "True"
	}
	return "False"
}

func (pb PyBool) Int() int {
	if bool(pb) {
		return 1
	}
	return 0
}

func (pb PyBool) BitLength() int {
	if bool(pb) {
		return 1
	}
	return 0
}

func (pb PyBool) BitCount() int {
	if bool(pb) {
		return 1
	}
	return 0
}

func (pb PyBool) AsIntegerRatio() (int, int) {
	if bool(pb) {
		return 1, 1
	}
	return 0, 1
}
