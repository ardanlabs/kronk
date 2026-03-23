package pystring

// CapWords splits the argument into words using str.split(), capitalizes each word using str.capitalize(),
// and joins the capitalized words using str.join(). If the optional second argument sep is absent
// or None, runs of whitespace characters are replaced by a single space and leading and trailing
// whitespace are removed, otherwise sep is used to split and join the words.
func CapWords(s string) string {
	words := Split(s, "", -1)
	for i, word := range words {
		words[i] = Capitalize(word)
	}
	return JoinString(" ", words)
}

// CapWords splits the argument into words using str.split(), capitalizes each word using str.capitalize(),
// and joins the capitalized words using str.join(). If the optional second argument sep is absent
// or None, runs of whitespace characters are replaced by a single space and leading and trailing
// whitespace are removed, otherwise sep is used to split and join the words.
func (pys PyString) CapWords() PyString {
	return PyString(CapWords(string(pys)))
}
