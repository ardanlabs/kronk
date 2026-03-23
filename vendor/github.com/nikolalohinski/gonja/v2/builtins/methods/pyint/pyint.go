// Package pyint provides Python-compatible integer methods.
package pyint

import (
	"fmt"
	"math/big"
	"math/bits"
	"strconv"

	"github.com/nikolalohinski/gonja/v2/builtins/methods/pyerrors"
)

type PyInt int

func New(i int) PyInt {
	return PyInt(i)
}

func (pi PyInt) Int() int {
	return int(pi)
}

func (pi PyInt) String() string {
	return strconv.Itoa(int(pi))
}

func (pi PyInt) BitLength() int {
	return bits.Len(uint(pi))
}

func (pi PyInt) BitCount() int {
	return bits.OnesCount(uint(pi))
}

func (pi PyInt) AsIntegerRatio() (int, int) {
	return int(pi), 1
}

func (pi PyInt) IsInteger() bool {
	return true
}

type ByteOrder string

const (
	BigEndian    ByteOrder = "big"
	LittleEndian ByteOrder = "little"
)

func (pi PyInt) ToBytes(length int, byteorder ByteOrder, signed bool) ([]byte, error) {
	// Create a new big.Int from the PyInt value
	bigIntVal := big.NewInt(int64(pi))

	if !signed && pi < 0 {
		return nil, fmt.Errorf("%w: can't convert negative int to unsigned", pyerrors.ErrOverflow)
	}

	// Determine if we need to handle the sign
	if signed && pi < 0 {
		// If signed and the value is negative, we need to perform two's complement
		maxVal := new(big.Int).Lsh(big.NewInt(1), uint(length*8)) // 2^(length*8)
		bigIntVal.Add(bigIntVal, maxVal)
	}

	// Get the byte representation
	byteArray := bigIntVal.Bytes()

	// Ensure the byte array has the correct length; Possibly do this before
	// conversion to save RAM
	if len(byteArray) > length {
		return nil, fmt.Errorf("%w: int too big to convert", pyerrors.ErrOverflow)
	} else if len(byteArray) < length {
		// If the byte array is too short, pad it
		padLength := length - len(byteArray)
		padding := make([]byte, padLength)
		byteArray = append(padding, byteArray...)
	}

	// Handle byte order
	if byteorder == LittleEndian {
		// Reverse the byte array for little-endian order
		for i, j := 0, len(byteArray)-1; i < j; i, j = i+1, j-1 {
			byteArray[i], byteArray[j] = byteArray[j], byteArray[i]
		}
	}

	return byteArray, nil
}

func (pi PyInt) FromBytes(bytes []byte, byteorder ByteOrder, signed bool) (int64, error) {
	switch byteorder {
	case BigEndian:
		return fromBytesBigEndian(bytes, signed), nil
	case LittleEndian:
		return fromBytesLittleEndian(bytes, signed), nil
	default:
		return 0, fmt.Errorf("%w: byteorder must be either 'little' or 'big'", pyerrors.ErrValue)
	}
}

func fromBytesBigEndian(bytes []byte, signed bool) int64 {
	if len(bytes) == 0 {
		return 0
	}

	// If signed, handle the sign extension
	if signed && bytes[0]&0x80 != 0 {
		tmp := make([]byte, len(bytes))
		for i, b := range bytes {
			tmp[i] = b ^ 0xff
		}
		val := new(big.Int).SetBytes(tmp)
		val = val.Add(val, big.NewInt(1))
		return -val.Int64()
	}

	val := new(big.Int).SetBytes(bytes)
	return val.Int64()
}

func fromBytesLittleEndian(bytes []byte, signed bool) int64 {
	if len(bytes) == 0 {
		return 0
	}

	// Reverse the bytes for little-endian order
	reversedBytes := make([]byte, len(bytes))
	for i, b := range bytes {
		reversedBytes[len(bytes)-1-i] = b
	}

	return fromBytesBigEndian(reversedBytes, signed)
}
