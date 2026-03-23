package pystring

import (
	"strings"
	"unicode"
)

// Title returns a titlecased version of the string where words start with an uppercase
// character and the remaining characters are lowercase.
//
// For example:
//
// >>>
// >>> 'Hello world'.title()
// 'Hello World'
//
// The algorithm uses a simple language-independent definition of a word as
// groups of consecutive letters. The definition works in many contexts but it
// means that apostrophes in contractions and possessives form word boundaries,
// which may not be the desired result:
//
// >>>
// >>> "they're bill's friends from the UK".title(){}
// "They'Re Bill'S Friends From The Uk"
//
// The string.capwords() function does not have this problem, as it splits
// words on spaces only.
func Title(s string) string {
	var res strings.Builder

	prevIsCased := false
	for _, char := range s {
		if prevIsCased {
			res.WriteRune(unicode.ToLower(char))
		} else {
			res.WriteRune(unicode.ToTitle(char))
		}
		prevIsCased = unicode.IsLetter(char)
	}
	
	return res.String()
}

// Title returns a titlecased version of the string where words start with an uppercase
// character and the remaining characters are lowercase.
//
// For example:
//
// >>>
// >>> 'Hello world'.title()
// 'Hello World'
//
// The algorithm uses a simple language-independent definition of a word as
// groups of consecutive letters. The definition works in many contexts but it
// means that apostrophes in contractions and possessives form word boundaries,
// which may not be the desired result:
//
// >>>
// >>> "they're bill's friends from the UK".title(){}
// "They'Re Bill'S Friends From The Uk"
//
// The string.capwords() function does not have this problem, as it splits
// words on spaces only.
func (pys PyString) Title() PyString {
	return PyString(Title(string(pys)))
}
