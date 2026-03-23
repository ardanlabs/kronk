package pystring

import "golang.org/x/text/cases"

// Casefold returns a casefolded copy of the string. Casefolded strings may be used for caseless matching.
//
// Casefolding is similar to lowercasing but more aggressive because it is intended to remove all case distinctions in a string. For example, the German lowercase letter 'ß' is equivalent to "ss". Since it is already lowercase, lower() would do nothing to 'ß'; casefold() converts it to "ss".
//
// The casefolding algorithm is described in section 3.13 ‘Default Case Folding’ of the Unicode Standard.
//
// New in version 3.3.
func Casefold(s string) string {
	return cases.Fold().String(s)
}

// Casefold returns a casefolded copy of the string. Casefolded strings may be used for caseless matching.
//
// Casefolding is similar to lowercasing but more aggressive because it is intended to remove all case distinctions in a string. For example, the German lowercase letter 'ß' is equivalent to "ss". Since it is already lowercase, lower() would do nothing to 'ß'; casefold() converts it to "ss".
//
// The casefolding algorithm is described in section 3.13 ‘Default Case Folding’ of the Unicode Standard.
//
// New in version 3.3.
func (pys PyString) Casefold() PyString {
	return PyString(Casefold(string(pys)))
}
