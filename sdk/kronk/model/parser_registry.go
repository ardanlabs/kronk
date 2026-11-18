package model

// =============================================================================
// Parser Registry
//
// The model package owns the built-in parser set. The registry holds custom
// factories, which are tried in registration order before the built-ins.
// No caller bootstrap or init()-time registration is needed for defaults.
// =============================================================================

// ParserFactory is the constructor signature each parser package's
// New function satisfies. The bool return reports whether this parser
// claims the given Fingerprint; on false, the registry continues to the
// next factory.
type ParserFactory func(Fingerprint) (Parser, bool)

// registeredParsers is the ordered registry. Earlier entries take
// precedence over later ones. The slice is built at startup via
// RegisterParser and read at Model.Load via selectParser — there is
// no concurrent mutation in practice, so no lock is needed.
var registeredParsers []ParserFactory

// RegisterParser appends a custom parser factory to the registry. Call once
// per parser at application bootstrap, before any models are loaded. Custom
// parsers are tried in registration order, ahead of all built-in parsers.
// A custom catch-all prevents subsequent factories and built-ins from running.
func RegisterParser(f ParserFactory) {
	registeredParsers = append(registeredParsers, f)
}

// selectParser tries custom factories first, then the ordered built-in set.
func selectParser(fp Fingerprint) Parser {
	for _, f := range registeredParsers {
		parser, ok := f(fp)
		if !ok {
			continue
		}
		return parser
	}
	return selectDefaultParser(fp)
}
