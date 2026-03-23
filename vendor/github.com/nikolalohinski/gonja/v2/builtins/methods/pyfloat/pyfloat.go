// Package pyfloat provides Python-compatible float methods.
package pyfloat

import (
	"fmt"
	"math/big"
	"strings"
)

type PyFloat float64

func New(f float64) PyFloat {
	return PyFloat(f)
}

func (pf PyFloat) Float() float64 {
	return float64(pf)
}

func (pf PyFloat) String() string {
	return fmt.Sprintf("%f", float64(pf))
}

func (pf PyFloat) AsIntegerRatio() (int, int) {
	r, _ := big.NewFloat(float64(pf)).Rat(nil)
	return int(r.Num().Int64()), int(r.Denom().Int64())
}

func (pf PyFloat) IsInteger() bool {
	return pf == PyFloat(int(pf))
}

func (pf PyFloat) Hex() string {
	hexVal := fmt.Sprintf("%#.13x", float64(pf))

	// strip the leading zeros padding on the exponent
	hexVal = strings.Replace(hexVal, "p+0", "p+", 1)

	return hexVal
}
