package pystring

// DefaultDialect captures the format specifier behavior for Python 3.11.
// There have been multiple changes in Python regarding how format specifiers are handled;
// to enable all possible formats we capture these changes as feature flags which can be opted
// in or out of.
var DefaultDialect = NewDialect(3.11)
var DialectPython3_11 = NewDialect(3.11)
var DialectPython3_10 = NewDialect(3.10)
var DialectPython3_0 = NewDialect(3.0)

type Dialect struct {
	// Enabled strings to first be converted to numeric values before failing type validations
	// that require numeric values.
	// e.g.
	// 'foo={:=-10.5G}'.format(42.1234) => always correct
	// 'foo={:=-10.5G}'.format("42.1234") => fails if not set to true.
	// This is not how python works but can simplify usage where even numbers
	// are often provided as strings.
	// NOTE: BETA feature
	tryTypeJugglingString bool

	// added in 3.10
	zeroPaddingAlignment rune

	// Added in 3.11
	enableCoercesNegativeZeroToPositive bool
}

type DialectOption func(*Dialect)

func NewDialect(version float64, options ...DialectOption) Dialect {
	res := Dialect{
		zeroPaddingAlignment:                '=',
		tryTypeJugglingString:               false,
		enableCoercesNegativeZeroToPositive: false,
	}

	// Sources for per version adjustments: https://docs.python.org/3/library/string.html#string.Formatter

	// Changed in version 3.10: Preceding the width field by '0' no longer affects the default alignment for strings.
	if version >= 3.10 {
		res.zeroPaddingAlignment = 0
	}

	// The 'z' option coerces negative zero floating-point values to positive zero after rounding to the format precision.
	// This option is only valid for floating-point presentation types.
	//
	// Changed in version 3.11: Added the 'z' option (see also PEP 682).
	if version >= 3.11 {
		res.enableCoercesNegativeZeroToPositive = true
	}

	for _, option := range options {
		option(&res)
	}

	return res
}

func (d Dialect) CloneWithOptions(options ...DialectOption) Dialect {
	res := d
	for _, option := range options {
		option(&res)
	}
	return res
}

func WithTypeJugglingString(d *Dialect) {
	d.tryTypeJugglingString = true
}
