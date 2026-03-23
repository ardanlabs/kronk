package pystring

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/nikolalohinski/gonja/v2/builtins/methods/pyerrors"
	"golang.org/x/exp/utf8string"
)

// FormatSpec represents the format specification for formatting values
// format_spec     ::=  [[fill]align][sign]["z"]["#"]["0"][width][grouping_option]["." precision][type]
type FormatSpec struct {
	dialect             Dialect
	Fill                rune // Fill character
	Align               rune // Alignment character ('<' for left, '>' for right, '^' for center, '=' for numeric only)
	Sign                rune // Sign character ('+' for both, '-' for negative only, ' ' for leading space)
	CoercesNegativeZero bool // coerces negative zero floating-point values to positive zero after rounding to the format precision.
	Alternate           bool // Alternate form ('#' for alternative form)
	ZeroPadding         bool // Zero padding ('0' for zero padding)
	MinWidth            uint // Minimum width
	GroupingOption      rune // Grouping option (',' or '_')
	Precision           uint // Precision
	Type                rune // Type character ('b', 'c', 'd', 'o', 'x', 'X', 'e', 'E', 'f', 'F', 'g', 'G', '%')
}

func NewFormatterSpecFromStr(format string) (FormatSpec, error) {
	return DefaultDialect.NewFormatterSpecFromStr(format)
}

func (d Dialect) NewFormatterSpecFromStr(format string) (FormatSpec, error) {
	spec := FormatSpec{
		dialect: d,
	}

	s := utf8string.NewString(format)

	// For when multiples runes are parsed in 1 loop we need to
	// skip the runes we parsed.
	skipUntil := -1
	for idx, char := range format {
		if idx < skipUntil {
			continue
		}

		// If the next rune is an alignment character, skip this iteration.
		var maybeNextRune rune
		if idx == 0 && idx+1 < len(format) {
			maybeNextRune = s.At(idx + 1)
			if maybeNextRune == '<' || maybeNextRune == '>' || maybeNextRune == '^' || maybeNextRune == '=' {
				continue
			}
		}

		switch char {
		case '<', '>', '^', '=':
			if spec.Sign != 0 || spec.CoercesNegativeZero || spec.Alternate || spec.MinWidth > 0 || spec.Precision > 0 || spec.Type != 0 {
				return spec, fmt.Errorf("%w: Invalid format specifier '%s' - Alignment may only be first or second character", pyerrors.ErrValue, format)
			}
			spec.Align = char
			if idx > 0 {
				spec.Fill = s.At(0)
			}

		case '+', '-', ' ':
			if spec.Sign != 0 || spec.CoercesNegativeZero || spec.Alternate || spec.MinWidth > 0 || spec.Precision > 0 || spec.Type != 0 {
				return spec, fmt.Errorf("%w: Invalid format specifier '%s' - unexpected sign '%s' ", pyerrors.ErrValue, format, string(char))
			}
			spec.Sign = char

		case 'z':
			if !spec.dialect.enableCoercesNegativeZeroToPositive {
				return spec, fmt.Errorf("%w: Invalid format specifier '%s' - unexpected float coercion sign '%s' ", pyerrors.ErrValue, format, string(char))
			}
			if spec.CoercesNegativeZero || spec.Alternate || spec.MinWidth > 0 || spec.Precision > 0 || spec.Type != 0 {
				return spec, fmt.Errorf("%w: Invalid format specifier '%s' - unexpected float coercion sign '%s' ", pyerrors.ErrValue, format, string(char))
			}
			spec.CoercesNegativeZero = true

		case '#':
			if spec.Alternate || spec.MinWidth > 0 || spec.Precision > 0 || spec.Type != 0 {
				return spec, fmt.Errorf("%w: Invalid format specifier '%s' - unexpected alternative sign '%s' ", pyerrors.ErrValue, format, string(char))
			}
			spec.Alternate = true

		case '_', ',':
			if spec.Precision > 0 || spec.Type != 0 {
				return spec, fmt.Errorf("%w: Invalid format specifier '%s' - unexpected grouping sign '%s' ", pyerrors.ErrValue, format, string(char))
			}
			spec.GroupingOption = char

		case '.':
			if spec.Precision != 0 || spec.Type != 0 || idx+1 >= len(format) {
				return spec, fmt.Errorf("%w: Invalid format specifier '%s' - unexpected precision '%s' ", pyerrors.ErrValue, format, string(char))
			}
			r, _ := utf8.DecodeRuneInString(format[idx+1:])
			if r == utf8.RuneError || !unicode.IsDigit(r) {
				return spec, fmt.Errorf("%w: Invalid format specifier '%s' - unexpected precision '%s' ", pyerrors.ErrValue, format, string(r))
			}

			firstDigit := format[idx+1:]
			offset := IndexFirstNonDigit(firstDigit)
			skipUntil = idx + 1 + offset
			completeNumber := firstDigit[:offset]
			width, err := strconv.Atoi(completeNumber)
			if err != nil {
				return spec, fmt.Errorf("%w: Invalid format specifier '%s': '%s' ", pyerrors.ErrInternal, format, err.Error())
			}
			spec.Precision = uint(width)

		case 'b', 'c', 'd', 'o', 'x', 'X', 'e', 'E', 'f', 'F', 'g', 'G', '%', 'n', 's':
			if spec.Type != 0 {
				return spec, fmt.Errorf("%w: Invalid format specifier '%s' - unexpected type '%s' ", pyerrors.ErrValue, format, string(char))
			}
			spec.Type = char

		default:
			// MinWidth
			if unicode.IsDigit(char) {
				if spec.Type != 0 || spec.Precision > 0 {
					return spec, fmt.Errorf("%w: Invalid format specifier '%s' - unexpected minwidth '%s' ", pyerrors.ErrValue, format, string(char))
				}

				if char == '0' {
					// This setting may affect alignment and fill but that depends on the dialect and data type passed.
					spec.ZeroPadding = true
					continue
				}

				firstDigit := format[idx:]
				offset := IndexFirstNonDigit(firstDigit)
				skipUntil = idx + offset
				completeNumber := firstDigit[:offset]
				width, err := strconv.Atoi(completeNumber)
				if err != nil {
					return spec, fmt.Errorf("%w: Invalid format specifier '%s': '%s' ", pyerrors.ErrInternal, format, err.Error())
				}
				spec.MinWidth = uint(width)
			} else {
				return spec, fmt.Errorf("%w: Invalid format specifier '%s' - unexpected '%s' at pos %d; NextRund: %s", pyerrors.ErrValue, format, string(char), idx, string(maybeNextRune))
			}
		}
	}

	if err := spec.Validate(); err != nil {
		return spec, err
	}

	return spec, nil
}

func (f FormatSpec) String() string {
	//res := fmt.Sprintf("FormatSpec{Fill: %c, Align: %c, Sign: %c, Alternate: %v, ZeroPadding: %v, MinWidth: %d, Precision: %d, Type: %c}", f.Fill, f.Align, f.Sign, f.Alternate, f.ZeroPadding, f.MinWidth, f.Precision, f.Type)
	res := ""
	if f.Fill != 0 {
		res += fmt.Sprintf("%c", f.Fill)
	}
	if f.Align != 0 {
		res += fmt.Sprintf("%c", f.Align)
	}
	if f.Sign != 0 {
		res += fmt.Sprintf("%c", f.Sign)
	}
	if f.Alternate {
		res += "#"
	}
	if f.ZeroPadding {
		res += "0"
	}
	if f.MinWidth > 0 {
		res += strconv.Itoa(int(f.MinWidth))
	}
	if f.GroupingOption != 0 {
		res += string(f.GroupingOption)
	}
	if f.Precision > 0 {
		res += fmt.Sprintf(".%d", f.Precision)
	}
	if f.Type != 0 {
		res += fmt.Sprintf("%c", f.Type)
	}
	return res
}

func (f FormatSpec) AlignIsValid() bool {
	return f.Align == '<' || f.Align == '>' || f.Align == '^' || f.Align == '=' || f.Align == 0
}

func (f FormatSpec) SignIsValid() bool {
	return f.Sign == '+' || f.Sign == '-' || f.Sign == ' ' || f.Sign == 0
}

func (f FormatSpec) TypeIsValid() bool {
	return f.Type == '%' ||
		f.Type == 'b' ||
		f.Type == 'c' ||
		f.Type == 'd' ||
		f.Type == 'e' ||
		f.Type == 'E' ||
		f.Type == 'f' ||
		f.Type == 'F' ||
		f.Type == 'g' ||
		f.Type == 'G' ||
		f.Type == 'n' ||
		f.Type == 'o' ||
		f.Type == 's' ||
		f.Type == 'x' ||
		f.Type == 'X' ||
		f.Type == 0
}

func (f FormatSpec) GroupingOptionIsValid() bool {
	return f.GroupingOption == '_' || f.GroupingOption == ',' || f.GroupingOption == 0
}

func (f FormatSpec) ExpectFloatType() bool {
	return f.Type == 'e' || f.Type == 'E' || f.Type == 'f' || f.Type == 'F' || f.Type == 'g' || f.Type == 'G' || f.Type == '%' || f.Precision > 0
}

func (f FormatSpec) ExpectIntType() bool {
	return f.Type == 'b' || f.Type == 'c' || f.Type == 'd' || f.Type == 'o' || f.Type == 'x' || f.Type == 'X' || f.Type == 'n'
}

func (f FormatSpec) ExpectNumericType() bool {
	return f.ExpectFloatType() || f.ExpectIntType() || f.Sign != 0 || f.Align == '=' || f.GroupingOption != 0 || f.Alternate
}

func (f FormatSpec) ExpectStringType() bool {
	return (f.Type == 's' || f.Type == 'c' ||
		(f.Type == 0) && (f.Precision == 0 && f.Sign == 0 && !f.Alternate && f.Align == 0))
}

func (f FormatSpec) Validate() error {
	if !f.AlignIsValid() {
		return fmt.Errorf("%w: Invalid alignment character: %c", pyerrors.ErrValue, f.Align)
	}
	if !f.SignIsValid() {
		return fmt.Errorf("%w: Invalid sign character: %c", pyerrors.ErrValue, f.Sign)
	}
	if !f.TypeIsValid() {
		return fmt.Errorf("%w: Invalid type character: %c", pyerrors.ErrValue, f.Type)
	}
	if !f.GroupingOptionIsValid() {
		return fmt.Errorf("%w: Invalid grouping option: %c", pyerrors.ErrValue, f.GroupingOption)
	}

	if f.ExpectIntType() && f.Precision > 0 {
		return fmt.Errorf("%w: Precision only allowed for float types, not %c", pyerrors.ErrValue, f.Type)
	}

	expectString := f.ExpectStringType()
	if expectString && f.Sign != 0 {
		return fmt.Errorf("%w: Sign not allowed with string format specifier 's'", pyerrors.ErrValue)
	}
	if expectString && f.Align == '=' {
		return fmt.Errorf("%w: '=' alignment not allowed with string format specifier 's'", pyerrors.ErrValue)
	}
	if expectString && f.GroupingOption != 0 {
		return fmt.Errorf("%w: '=' grouping only allowed with float and int types", pyerrors.ErrValue)
	}
	if expectString && f.Precision > 0 {
		return fmt.Errorf("%w: Precision not allowed with string format specifier 's'", pyerrors.ErrValue)
	}

	if f.GroupingOption != 0 && (f.Type != 'd' && f.Type != 'b' && f.Type != 'o' && f.Type != 'x' && f.Type != 'X') {
		return fmt.Errorf("%w: Cannot specify '%s' with '%s'", pyerrors.ErrValue, string(f.GroupingOption), string(f.Type))
	}

	expectedIntType := f.ExpectIntType()
	if f.Alternate && !expectedIntType && f.Type != 0 {
		return fmt.Errorf("%w: Alternate form (#) only allowed with integer types, not %c", pyerrors.ErrValue, f.Type)
	}
	if expectedIntType && f.Precision > 0 {
		return fmt.Errorf("%w: Precision not allowed with integer format specifier '%c'", pyerrors.ErrValue, f.Type)
	}

	if f.Alternate && f.Type == 'c' {
		return fmt.Errorf("%w: Alternate form (#) not allowed with integer format specifier '%c'", pyerrors.ErrValue, f.Type)
	}
	if f.Fill != 0 && f.Align == 0 {
		return fmt.Errorf("%w: Fill character only allowed with alignment", pyerrors.ErrValue)
	}
	if f.Fill == '}' || f.Fill == '{' {
		return fmt.Errorf("%w: Single '%s' encountered in format string", pyerrors.ErrValue, string(f.Fill))
	}

	return nil
}

func (f FormatSpec) updateSpecWithDataCategory(valueCat ValueCategory) FormatSpec {
	switch valueCat {
	case ValueCategoryString:
		// Changed in version 3.10: Preceding the width field by '0' no longer affects the default alignment for strings.
		if f.ZeroPadding {
			if f.Align == 0 && f.dialect.zeroPaddingAlignment != 0 {
				f.Align = f.dialect.zeroPaddingAlignment
			}
			if f.Fill == 0 {
				f.Fill = '0'
			}
		}

	case ValueCategoryBool:
	case ValueCategoryInt, ValueCategoryFloat:
		// When no explicit alignment is given, preceding the width field by a zero ('0') character enables sign-aware
		// zero-padding for numeric types. This is equivalent to a fill character of '0' with an alignment type of '='.
		// Changed in version 3.10: Preceding the width field by '0' no longer affects the default alignment for strings.
		if f.ZeroPadding {
			if f.Align == 0 {
				f.Align = '='
			}
			if f.Fill == 0 {
				f.Fill = '0'
			}
		}
	}

	return f
}

func (f FormatSpec) Format(v any) (string, error) {
	s, valueCat, err := f.FormatValue(v)
	if err != nil {
		return "", err
	}

	f = f.updateSpecWithDataCategory(valueCat)

	// Can we skip padding and grouping?
	if f.MinWidth == 0 && f.GroupingOption == 0 {
		return s, nil
	}

	// defaults
	if f.Fill == 0 {
		f.Fill = ' '
	}
	if f.Align == 0 {
		f.Align = '<' // the default for most objects

		if valueCat == ValueCategoryFloat || valueCat == ValueCategoryInt {
			f.Align = '>' // default for numbers
		}
	}

	sign := ""
	if valueCat == ValueCategoryFloat || valueCat == ValueCategoryInt {
		if strings.HasPrefix(s, "-") {
			sign = "-"
		} else if f.Sign == ' ' {
			sign = " "
		} else if f.Sign == '+' {
			sign = "+"
		}
		s = strings.Trim(s, sign)
	}

	if f.Alternate {
		switch {
		case f.Type == 'o':
			sign += "0o"
			s = strings.TrimPrefix(s, "0o")

		case f.Type == 'x':
			sign += "0x"
			s = strings.TrimPrefix(s, "0x")

		case f.Type == 'X':
			sign += "0X"
			s = strings.TrimPrefix(s, "0X")

		case f.Type == 'b':
			sign += "0b"
			s = strings.TrimPrefix(s, "0b")
		}
	}

	groupingInterval := 3
	if f.Type == 'b' || f.Type == 'x' || f.Type == 'X' || f.Type == 'o' {
		groupingInterval = 4
	}
	if f.GroupingOption != 0 && (valueCat == ValueCategoryFloat || valueCat == ValueCategoryInt) {
		tmp := []string{}
		for sLen := len(s); sLen > 0; sLen = len(s) {
			// First batch might be smaller than the grouping interval.
			take := sLen % groupingInterval
			if take == 0 {
				take = groupingInterval
			}
			tmp = append(tmp, s[:take])
			s = s[take:]
		}

		s = strings.Join(tmp, string(f.GroupingOption))
	}

	// Avoid panics in strings.Repeat
	sLen := utf8.RuneCountInString(s)
	requiredPadding := max(int(f.MinWidth)-sLen-utf8.RuneCountInString(sign), 0)

	switch f.Align {
	case '<':
		return sign + s + strings.Repeat(string(f.Fill), requiredPadding), nil
	case '>':
		return strings.Repeat(string(f.Fill), requiredPadding) + sign + s, nil
	case '^':
		leftPad := requiredPadding / 2
		rightPad := requiredPadding - leftPad
		return strings.Repeat(string(f.Fill), leftPad) + sign + s + strings.Repeat(string(f.Fill), rightPad), nil

	case '=', 0:
		var res strings.Builder

		res.WriteString(sign)
		for i := range requiredPadding {
			posFromLast := (requiredPadding + sLen) - i
			writePadding := (posFromLast)%(groupingInterval+1) == 0
			if writePadding && f.GroupingOption != 0 {
				res.WriteRune(f.GroupingOption)
			} else {
				res.WriteRune(f.Fill)
			}
		}
		res.WriteString(s)

		return res.String(), nil
	}
	return s, nil

}

type ValueCategory int

const (
	ValueCategoryUnknown ValueCategory = iota
	ValueCategoryString
	ValueCategoryBool
	ValueCategoryInt
	ValueCategoryFloat
)

func (f FormatSpec) FormatValue(v any) (string, ValueCategory, error) {

	switch tv := v.(type) {
	case string:

		// Only added for compatibility with python.
		//if f.Fill == ' ' {
		//	return "", ValueCategoryString, fmt.Errorf("%w: Space not allowed in string format specifier", pyerrors.ErrValue)
		//}
		if f.Sign != 0 && !f.dialect.tryTypeJugglingString {
			return "", ValueCategoryString, fmt.Errorf("%w: Sign not allowed in string format specifier", pyerrors.ErrValue)
		}
		if f.Alternate && !f.dialect.tryTypeJugglingString {
			return "", ValueCategoryString, fmt.Errorf("%w: Alternate form (#) not allowed in string format specifier", pyerrors.ErrValue)
		}
		if f.GroupingOption != 0 && !f.dialect.tryTypeJugglingString {
			return "", ValueCategoryString, fmt.Errorf("%w: Cannot specify '%s' with 's'", pyerrors.ErrValue, string(f.GroupingOption))
		}

		// We expect this library to be used where type safety isn't guaranteed; Therefore
		// we will try to be forgiving and try to convert string into the implied type.
		// before failing.
		switch f.Type {
		case 0, 's':
			// Truncation needed?
			l := len(tv)
			if f.Precision > 0 && uint(l) > f.Precision {
				tv = tv[:f.Precision]
			}
			return tv, ValueCategoryString, nil

		case 'e', 'E', 'f', 'F', 'g', 'G', '%':
			if !f.dialect.tryTypeJugglingString {
				return "", ValueCategoryString, fmt.Errorf("unsupported value type: %T for format type %c", v, f.Type)
			}

			floatVal, err := strconv.ParseFloat(tv, 64)
			if err != nil {
				return "", ValueCategoryString, fmt.Errorf("unsupported value type: %T for format type %c", v, f.Type)
			}

			s, err := f.FormatFloat(floatVal)
			return s, ValueCategoryFloat, err

		case 'b', 'c', 'd', 'o', 'x', 'X':
			if !f.dialect.tryTypeJugglingString {
				return "", ValueCategoryString, fmt.Errorf("unsupported value type: %T for format type %c", v, f.Type)
			}

			intVal, err := strconv.ParseInt(tv, 10, 64)
			if err != nil {
				return "", ValueCategoryString, fmt.Errorf("unsupported value type: %T for format type %c", v, f.Type)
			}

			s, err := f.FormatInt(intVal)
			return s, ValueCategoryInt, err
		}

		if f.Type != 0 {
			return "", ValueCategoryString, fmt.Errorf("unsupported value type: %T for format type %c", v, f.Type)
		}

		return tv, ValueCategoryString, nil

	case bool:
		res, err := f.FormatBool(tv)
		return res, ValueCategoryBool, err

	case int:
		res, err := f.FormatInt(int64(tv))
		return res, ValueCategoryInt, err
	case int8:
		res, err := f.FormatInt(int64(tv))
		return res, ValueCategoryInt, err
	case int16:
		res, err := f.FormatInt(int64(tv))
		return res, ValueCategoryInt, err
	case int32:
		res, err := f.FormatInt(int64(tv))
		return res, ValueCategoryInt, err
	case int64:
		res, err := f.FormatInt(tv)
		return res, ValueCategoryInt, err

	case uint:
		res, err := f.FormatInt(int64(tv))
		return res, ValueCategoryInt, err
	case uint8:
		res, err := f.FormatInt(int64(tv))
		return res, ValueCategoryInt, err
	case uint16:
		res, err := f.FormatInt(int64(tv))
		return res, ValueCategoryInt, err
	case uint32:
		res, err := f.FormatInt(int64(tv))
		return res, ValueCategoryInt, err
	case uint64:
		res, err := f.FormatInt(int64(tv))
		return res, ValueCategoryInt, err

	case float32:
		res, err := f.FormatFloat(float64(tv))
		return res, ValueCategoryFloat, err
	case float64:
		res, err := f.FormatFloat(tv)
		return res, ValueCategoryFloat, err

	case complex64:
		return "", ValueCategoryUnknown, fmt.Errorf("unsupported value type: %T", v)
	case complex128:
		return "", ValueCategoryUnknown, fmt.Errorf("unsupported value type: %T", v)

	default:
		return "", ValueCategoryUnknown, fmt.Errorf("unsupported value type: %T", v)
	}
}

func (f FormatSpec) FormatBool(value bool) (string, error) {
	if f.MinWidth != 0 || f.Align != 0 || f.Sign != 0 || f.Alternate || f.ZeroPadding {
		if value {
			return f.FormatInt(1)
		}
		return f.FormatInt(0)
	}

	if value {
		return "True", nil
	} else {
		return "False", nil
	}
}

// FormatInt formats an integer according to the given type.
func (f FormatSpec) FormatInt(value int64) (string, error) {

	if f.Precision > 0 {
		return "", fmt.Errorf("%w: Precision not allowed with integer format specifier", pyerrors.ErrValue)
	}

	valueStr, err := f.formatInt(value)
	if err != nil {
		return "", err
	}

	if value < 0 {
		return valueStr, nil
	}

	if f.Sign == '+' {
		return "+" + valueStr, nil
	}
	if f.Sign == ' ' {
		return " " + valueStr, nil
	}

	return valueStr, nil
}

func (f FormatSpec) formatInt(value int64) (string, error) {
	switch f.Type {
	case 'b':
		if f.Alternate {
			return fmt.Sprintf("%#b", value), nil
		}
		return strconv.FormatInt(value, 2), nil
	case 'c':
		return string(rune(value)), nil
	case 'd', 'n', 0:
		return strconv.Itoa(int(value)), nil
	case 'o':
		if f.Alternate {
			return fmt.Sprintf("%O", value), nil
		}
		return strconv.FormatInt(value, 8), nil
	case 'x':
		if f.Alternate {
			return fmt.Sprintf("%#x", value), nil
		}
		return strconv.FormatInt(value, 16), nil
	case 'X':
		if f.Alternate {
			return strings.ToUpper(fmt.Sprintf("%#x", value)), nil
		}
		//return fmt.Sprintf("%#X", value), nil
		return strings.ToUpper(strconv.FormatInt(value, 16)), nil

	case 'e', 'E', 'f', 'F', 'g', 'G', '%':
		return f.FormatFloat(float64(value))

	default:
		return "", fmt.Errorf("%w: unsupported format type: %s for integer datatype", pyerrors.ErrValue, string(f.Type))
	}
}

var negativeZero = math.Copysign(0, -1)

func (f FormatSpec) FormatFloat(value float64) (string, error) {
	if f.CoercesNegativeZero && value == negativeZero {
		value = 0.0
	}

	valueStr, err := f.formatFloat(value)
	if err != nil {
		return "", err
	}

	// Only positive number require special sign padding
	if value < 0 {
		return valueStr, nil
	}

	if f.Sign == '+' {
		return "+" + valueStr, nil
	}
	if f.Sign == ' ' {
		return " " + valueStr, nil
	}

	return valueStr, nil

}

// FormatFloat formats a float64 according to the given type.
// From: https://docs.python.org/3/library/string.html#string.Formatter
func (f FormatSpec) formatFloat(value float64) (string, error) {
	precision := int(f.Precision)
	if precision == 0 {
		precision = 6 // Default precision in python.
	}

	switch f.Type {
	case 'e':
		// e seems to always imply alternate form in python
		if f.Alternate {
			return fmt.Sprintf("%#."+strconv.Itoa(precision)+"e", value), nil
		}
		return strconv.FormatFloat(value, 'e', precision, 64), nil
	case 'E':
		// e seems to always imply alternate form in python
		if f.Alternate {
			return strings.ToUpper(fmt.Sprintf("%#."+strconv.Itoa(precision)+"e", value)), nil
		}
		return strings.ToUpper(strconv.FormatFloat(value, 'e', precision, 64)), nil
	case 'f':
		if f.Alternate {
			return fmt.Sprintf("%#."+strconv.Itoa(precision)+"f", value), nil
		}
		return strconv.FormatFloat(value, 'f', precision, 64), nil
	case 'F':

		if f.Alternate {
			return strings.ToUpper(fmt.Sprintf("%#."+strconv.Itoa(precision)+"f", value)), nil
		}
		return strings.ToUpper(strconv.FormatFloat(value, 'f', precision, 64)), nil

	case 'n':
		if f.Alternate {
			return fmt.Sprintf("%#."+strconv.Itoa(precision)+"g", value), nil
		}
		return strconv.FormatFloat(value, 'g', precision, 64), nil
	case '%':
		// % seems to always imply alternate form in python
		if f.Alternate {
			return fmt.Sprintf("%#."+strconv.Itoa(precision)+"f%%", value*100), nil
		}
		return strconv.FormatFloat(value*100, 'f', precision, 64) + "%", nil

	case 'g':
		if f.Alternate {
			return fmt.Sprintf("%#."+strconv.Itoa(precision)+"g", value), nil
		}
		return strconv.FormatFloat(value, 'g', precision, 64), nil
	case 'G':
		if f.Alternate {
			return fmt.Sprintf("%#."+strconv.Itoa(precision)+"G", value), nil
		}
		return strconv.FormatFloat(value, 'G', precision, 64), nil

	case 0: // None
		if f.Alternate {
			return fmt.Sprintf("%#."+strconv.Itoa(precision)+"f", value), nil
		}

		return strconv.FormatFloat(value, 'g', precision, 64), nil
	default:
		return "", fmt.Errorf("unsupported format type: %c for float data type", f.Type)
	}
}

func IndexFirstNonDigit(s string) int {
	for i, char := range s {
		if !unicode.IsDigit(char) {
			return i
		}
	}
	return len(s)
}
