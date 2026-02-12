package model

import "unicode/utf8"

// extractCompleteUTF8 separates a byte slice into complete UTF-8 codepoints
// and any trailing bytes that form an incomplete (but valid prefix of a)
// multi-byte sequence. This handles multi-byte characters (like emoji) that
// get split across multiple BPE tokens.
//
// Bytes that can never form valid UTF-8 (lone continuation bytes, overlong
// encodings, etc.) are passed through in complete rather than buffered
// indefinitely.
func extractCompleteUTF8(b []byte) (complete []byte, remainder []byte) {
	if utf8.Valid(b) {
		return b, nil
	}

	n := len(b)
	i := n

	for i > 0 {
		i--
		c := b[i]

		if c < 0x80 {
			break
		}

		if c&0xC0 != 0x80 {
			var expected int
			switch {
			case c&0xE0 == 0xC0:
				expected = 2
			case c&0xF0 == 0xE0:
				expected = 3
			case c&0xF8 == 0xF0:
				expected = 4
			default:
				break
			}

			have := n - i
			if expected > 0 && have < expected {
				return b[:i], b[i:]
			}

			break
		}
	}

	return b, nil
}
