package pystring

// FormatMapWithDialect is an alias for FormatWithDialect.
func FormatMapWithDialect(d Dialect, s string, vargs []any, kwarg map[string]any) (string, error) {
	return FormatWithDialect(d, s, vargs, kwarg)
}

// FormatMap is an alias for Format.
func FormatMap(s string, vargs []any, kwarg map[string]any) (string, error) {
	return Format(s, vargs, kwarg)
}

// FormatMap is an alias for Format.
func (pys PyString) FormatMap(vargs []any, kwarg map[string]any) (PyString, error) {
	return pys.Format(vargs, kwarg)
}

// FormatMapWithDialect is an alias for FormatWithDialect.
func (pys PyString) FormatMapWithDialect(d Dialect, vargs []any, kwarg map[string]any) (PyString, error) {
	return pys.FormatWithDialect(d, vargs, kwarg)
}
