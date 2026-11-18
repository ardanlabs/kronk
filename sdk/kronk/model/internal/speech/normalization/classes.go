package normalization

// These explicit ranges are part of the b12503 pattern specification, not the
// platform's Unicode database. In particular, alpha is intentionally broad.
func inRanges(c rune, ranges ...[2]rune) bool {
	for _, r := range ranges {
		if r[0] <= c && c <= r[1] {
			return true
		}
	}
	return false
}

func space(c rune) bool {
	return inRanges(c, [2]rune{0x9, 0xd}, [2]rune{0x20, 0x20}, [2]rune{0x85, 0x85},
		[2]rune{0xa0, 0xa0}, [2]rune{0x1680, 0x1680}, [2]rune{0x2000, 0x200a},
		[2]rune{0x2028, 0x2029}, [2]rune{0x202f, 0x202f}, [2]rune{0x205f, 0x205f}, [2]rune{0x3000, 0x3000})
}

func punct(c rune) bool {
	return inRanges(c, [2]rune{0x21, 0x2f}, [2]rune{0x3a, 0x40}, [2]rune{0x5b, 0x60}, [2]rune{0x7b, 0x7e},
		[2]rune{0xa1, 0xa9}, [2]rune{0xab, 0xb4}, [2]rune{0xb6, 0xb9}, [2]rune{0xbb, 0xbf},
		[2]rune{0xd7, 0xd7}, [2]rune{0xf7, 0xf7}, [2]rune{0x2010, 0x2027}, [2]rune{0x2030, 0x205e},
		[2]rune{0x20a0, 0x20cf}, [2]rune{0x2100, 0x218f}, [2]rune{0x2190, 0x2bff},
		[2]rune{0x3001, 0x3003}, [2]rune{0x3008, 0x3020}, [2]rune{0x3030, 0x3030},
		[2]rune{0x303d, 0x303d}, [2]rune{0x30fb, 0x30fb}, [2]rune{0xfe10, 0xfe19},
		[2]rune{0xfe30, 0xfe4f}, [2]rune{0xfe50, 0xfe6b}, [2]rune{0xff01, 0xff0f},
		[2]rune{0xff1a, 0xff20}, [2]rune{0xff3b, 0xff40}, [2]rune{0xff5b, 0xff65}, [2]rune{0x1f000, 0x1faff})
}

func control(c rune) bool {
	return inRanges(c, [2]rune{0, 0x1f}, [2]rune{0x7f, 0x9f}, [2]rune{0x200b, 0x200f},
		[2]rune{0x2060, 0x206f}, [2]rune{0xfeff, 0xfeff})
}

func alpha(c rune) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == 0xaa || c == 0xb5 || c == 0xba ||
		c >= 0xc0 && !space(c) && !punct(c) && !control(c)
}

func classMatch(kind, c rune) bool {
	switch kind {
	case 'd':
		return c >= '0' && c <= '9'
	case 's':
		return space(c)
	case 'p':
		return punct(c)
	case 'a':
		return alpha(c)
	case 'w':
		return alpha(c) || c >= '0' && c <= '9'
	case 'u':
		return c >= 'A' && c <= 'Z'
	case 'l':
		return c >= 'a' && c <= 'z'
	case 'h':
		return inRanges(c, [2]rune{0x2e80, 0x2fdf}, [2]rune{0x3005, 0x3005}, [2]rune{0x3007, 0x3007},
			[2]rune{0x3021, 0x3029}, [2]rune{0x3038, 0x303b}, [2]rune{0x3400, 0x4dbf},
			[2]rune{0x4e00, 0x9fff}, [2]rune{0xf900, 0xfaff}, [2]rune{0x20000, 0x323af})
	case 'c':
		return control(c)
	}
	return false
}

func lower(c rune) rune {
	if c >= 'A' && c <= 'Z' {
		return c + 32
	}
	return c
}

func swapCase(c rune) rune {
	if c >= 'a' && c <= 'z' {
		return c - 32
	}
	return lower(c)
}
