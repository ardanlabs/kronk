package pystring

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/nikolalohinski/gonja/v2/builtins/methods/pyerrors"
)

// Formatter is implemented by any value that decides how a formatting
// string should be interpreted. If not implemented, the default formatter is used.
type Formatter interface {
	Format(format string) (string, error)
}

func (d Dialect) Format(s string, vargs []any, kwarg map[string]any) (string, error) {

	var res strings.Builder

	scan := NewScanner(s, d)
	for {
		t, s, err := scan.Next()
		if err != nil {
			return "", fmt.Errorf("%w: %s", pyerrors.ErrInternal, err.Error())
		}

		switch {
		case t == Characters:
			res.WriteString(s)

		case t == ReplacementBlock:
			err := d.parseReplacementField(s, &res, vargs, kwarg)
			if err != nil {
				return "", err
			}

		case t == EOF:
			return res.String(), nil

		default:
			return "", fmt.Errorf("%w: Unknown token type %s", pyerrors.ErrInternal, t.String())

		}
	}
}

// parseReplacementField numbers automatic replacement blocks "{}" and normalizes
// all accessor patters (e.g. m['sub'] -> m.sub). Then splits up value part from
// formatting specifications; extracts the value from vargs and kwargs and applies
// the specified formatting.
func (d Dialect) parseReplacementField(s string, res *strings.Builder, vargs []any, kwarg map[string]any) error {
	// Strip initial and final braces
	if s[0] != '{' && s[len(s)-1] != '}' {
		return fmt.Errorf("%w: format didn't have format directives in: %s", pyerrors.ErrInternal, s)
	}
	s = s[1 : len(s)-1]

	// Extract string value of and format directives.
	formatDelimiterIdx := strings.Index(string(s), ":")
	var value, formatSpecifier string
	if formatDelimiterIdx == -1 {
		value = s
	} else {
		value = s[:formatDelimiterIdx]
		formatSpecifier = s[formatDelimiterIdx+1:]
	}

	// formatSpecifier may itself be based on replacement fields
	format, err := d.Format(formatSpecifier, vargs, kwarg)
	if err != nil {
		return fmt.Errorf("%w: failed subformat on value '%s'", err, format)
	}
	_ = format

	// Extract vargs
	if v, err := strconv.Atoi(string(value)); err == nil && v >= 0 {
		if v >= len(vargs) || v < 0 {
			return fmt.Errorf("%w: Replacement index %d out of range for positional args tuple", pyerrors.ErrIndex, v)
		}
		return d.formatReplacementFieldValue(res, vargs[v], string(format))
	}

	// python separates attributes and getAttr but for us they will be one and the same.
	// translate [] accessors to dot notation; e.g. m['sub'] -> m.sub;
	anyVal, err := getNestedKwArgs(simpleJSONPathSplit(string(value)), KwArgs(kwarg))
	if err != nil {
		return err
	}

	return d.formatReplacementFieldValue(res, anyVal, string(format))
}

func (d Dialect) formatReplacementFieldValue(res *strings.Builder, value any, formatStr string) error {
	if dataTypeFormatter, ok := value.(Formatter); ok {
		s, err := dataTypeFormatter.Format(formatStr)
		if err != nil {
			return err
		}
		_, err = res.WriteString(s)
		return err
	}

	dialectFormatter, err := d.NewFormatterSpecFromStr(formatStr)
	if err != nil {
		return err
	}

	formattedString, err := dialectFormatter.Format(value)
	if err != nil {
		return err
	}

	_, err = res.WriteString(formattedString)
	return err
}

func simpleJSONPathSplit(path string) []string {
	parts := []string{}
	var currentPart strings.Builder
	var withinBrackets, withinQuotes bool
	quoteChar := rune(0)

	for _, char := range path {
		switch char {
		case '[', ']':
			if !withinQuotes {
				withinBrackets = char == '['
				part := strings.Trim(currentPart.String(), "'\"")
				if part != "" {
					parts = append(parts, part)
				}
				currentPart.Reset()
				continue
			}
		case '"', '\'':
			if withinQuotes {
				if char == quoteChar {
					withinQuotes = false
					quoteChar = 0
					continue
				}
			} else {
				withinQuotes = true
				quoteChar = char
				continue
			}
		case '.':
			if !withinQuotes && !withinBrackets {
				part := strings.Trim(currentPart.String(), "'\"")
				if part != "" {
					parts = append(parts, part)
				}
				currentPart.Reset()
				continue
			}
		}
		currentPart.WriteRune(char)
	}

	// Add the last part if any
	part := strings.Trim(currentPart.String(), "'\"")
	if part != "" {
		parts = append(parts, part)
	}

	return parts
}
