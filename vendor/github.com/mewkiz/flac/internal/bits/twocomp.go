package bits

// IntN returns the signed two's complement of x with the specified integer bit
// width.
//
// Examples of unsigned (n-bit width) x values on the left and decoded values on
// the right:
//
//	0b011 -> 3
//	0b010 -> 2
//	0b001 -> 1
//	0b000 -> 0
//	0b111 -> -1
//	0b110 -> -2
//	0b101 -> -3
//	0b100 -> -4
func IntN(x uint64, n uint) int64 {
	signBitMask := uint64(1 << (n - 1))
	if x&signBitMask == 0 {
		// positive.
		return int64(x)
	}
	// negative.
	v := int64(x ^ signBitMask) // clear sign bit.
	v -= int64(signBitMask)
	return v
}
