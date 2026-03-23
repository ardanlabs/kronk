package pystring

import (
	"fmt"

	"github.com/nikolalohinski/gonja/v2/builtins/methods/pyerrors"
	"golang.org/x/text/encoding/charmap"
)

// Encode returns the string encoded to bytes.
// encoding defaults to 'utf-8'; see Standard Encodings for possible values.
// errors controls how encoding errors are handled. If 'strict' (the default), a UnicodeError exception is raised. Other possible values are 'ignore', 'replace', 'xmlcharrefreplace', 'backslashreplace' and any other name registered via codecs.register_error(). See Error Handlers for details.
// For performance reasons, the value of errors is not checked for validity unless an encoding error actually occurs, Python Development Mode is enabled or a debug build is used.
func Encode(str, encoding, errors string) ([]byte, error) {
	switch encoding {
	case "utf-8", "utf_8", "U8", "UTF", "utf8", "cp65001": // UTF-8 and its aliases
		return []byte(str), nil
	case "latin_1", "iso-8859-1", "iso8859-1", "8859", "cp819", "latin", "latin1", "L1": // ISO-8859-1 and its aliases
		encoder := charmap.ISO8859_1.NewEncoder()
		result, err := encoder.Bytes([]byte(str))
		if err != nil && errors == "strict" {
			return nil, fmt.Errorf("failed to encode %s to ISO-8859-1: %s", str, err)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("%w: unsupported encoding '%s'", pyerrors.ErrArguments, encoding)
	}
}

// Encode returns the string encoded to bytes.
// encoding defaults to 'utf-8'; see Standard Encodings for possible values.
// errors controls how encoding errors are handled. If 'strict' (the default), a UnicodeError exception is raised. Other possible values are 'ignore', 'replace', 'xmlcharrefreplace', 'backslashreplace' and any other name registered via codecs.register_error(). See Error Handlers for details.
// For performance reasons, the value of errors is not checked for validity unless an encoding error actually occurs, Python Development Mode is enabled or a debug build is used.
func (pys PyString) Encode(encoding string, errors string) ([]byte, error) {
	return Encode(string(pys), encoding, errors)
}
