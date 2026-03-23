package pystring

// FormatWithDialect performs a string formatting operation. The string on which this method is called can contain literal text or replacement fields delimited by braces {}. Each replacement field contains either the numeric index of a positional argument, or the name of a keyword argument. Returns a copy of the string where each replacement field is replaced with the string value of the corresponding argument.
//
// >>>
// >>> "The sum of 1 + 2 is {0}".format(1+2){}
// 'The sum of 1 + 2 is 3'
//
// See Format String Syntax for a description of the various formatting options that can be specified in format strings.
//
// Changes in python versions are captured by different dialects.
func FormatWithDialect(d Dialect, s string, vargs []any, kwarg map[string]any) (string, error) {
	return d.Format(s, vargs, kwarg)
}

// Format performs a string formatting operation. The string on which this method is called can contain literal text or replacement fields delimited by braces {}. Each replacement field contains either the numeric index of a positional argument, or the name of a keyword argument. Returns a copy of the string where each replacement field is replaced with the string value of the corresponding argument.
//
// >>>
// >>> "The sum of 1 + 2 is {0}".format(1+2){}
// 'The sum of 1 + 2 is 3'
//
// See Format String Syntax for a description of the various formatting options that can be specified in format strings.
func Format(s string, vargs []any, kwarg map[string]any) (string, error) {
	return FormatWithDialect(DefaultDialect, s, vargs, kwarg)
}

// FormatWithDialect performs a string formatting operation. The string on which this method is called can contain literal text or replacement fields delimited by braces {}. Each replacement field contains either the numeric index of a positional argument, or the name of a keyword argument. Returns a copy of the string where each replacement field is replaced with the string value of the corresponding argument.
//
// >>>
// >>> "The sum of 1 + 2 is {0}".format(1+2){}
// 'The sum of 1 + 2 is 3'
//
// See Format String Syntax for a description of the various formatting options that can be specified in format strings.
func (pys PyString) FormatWithDialect(d Dialect, vargs []any, kwarg map[string]any) (PyString, error) {
	res, err := FormatWithDialect(d, string(pys), vargs, kwarg)
	if err != nil {
		return "", err
	}
	return PyString(res), nil
}

// Format performs a string formatting operation. The string on which this method is called can contain literal text or replacement fields delimited by braces {}. Each replacement field contains either the numeric index of a positional argument, or the name of a keyword argument. Returns a copy of the string where each replacement field is replaced with the string value of the corresponding argument.
//
// >>>
// >>> "The sum of 1 + 2 is {0}".format(1+2){}
// 'The sum of 1 + 2 is 3'
//
// See Format String Syntax for a description of the various formatting options that can be specified in format strings.
//
// Changes in python versions are captured by different dialects.
func (pys PyString) Format(vargs []any, kwarg map[string]any) (PyString, error) {
	return pys.FormatWithDialect(DefaultDialect, vargs, kwarg)
}
