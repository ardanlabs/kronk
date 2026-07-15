package bits

// DecodeZigZag decodes a ZigZag encoded integer and returns it.
//
// Examples of ZigZag encoded values on the left and decoded values on the
// right:
//
//	0 =>  0
//	1 => -1
//	2 =>  1
//	3 => -2
//	4 =>  2
//	5 => -3
//	6 =>  3
//
// ref: https://developers.google.com/protocol-buffers/docs/encoding
func DecodeZigZag(x uint32) int32 {
	return int32(x>>1) ^ -int32(x&1)
}

// EncodeZigZag encodes a given integer to ZigZag-encoding.
//
// Examples of integer input on the left and corresponding ZigZag encoded values
// on the right:
//
//	 0 => 0
//	-1 => 1
//	 1 => 2
//	-2 => 3
//	 2 => 4
//	-3 => 5
//	 3 => 6
//
// ref: https://developers.google.com/protocol-buffers/docs/encoding
func EncodeZigZag(x int32) uint32 {
	if x < 0 {
		x = -x
		return uint32(x)<<1 - 1
	}
	return uint32(x) << 1
}
