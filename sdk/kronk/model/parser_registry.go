package model

// =============================================================================
// Parser Registry
//
// The registry holds an ordered list of parser factories. selectParser
// walks the list in registration order and returns the first parser that
// claims the given Fingerprint. Bootstrap is responsible for explicit
// registration:
//
// model.RegisterParser(gpt.New) // template-only — must be first
// model.RegisterParser(xmlfunc.New)
// model.RegisterParser(gemma.New)
// model.RegisterParser(glm.New)
// model.RegisterParser(mistral.New)
// model.RegisterParser(standard.New) // catch-all — must be last
//
// There is no init()-time auto-registration. Every binary that needs
// parser support enumerates the parsers it wants. This keeps the
// wired-in set visible via `grep RegisterParser` and avoids accidental
// dependencies pulled in by blank imports.
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

// RegisterParser appends a parser factory to the registry. Call once
// per parser at server bootstrap, before any models are loaded. Order
// matters: the catch-all parser (standard) must be registered last so
// the more specific parsers get first chance to claim.
func RegisterParser(f ParserFactory) {
	registeredParsers = append(registeredParsers, f)
}

// selectParser walks the registered factories in registration order and
// returns the first Parser that claims the fingerprint.
//
// Returns nil if no factory claims — bootstrap should always register a
// catch-all (typically parsers/standard.New, registered last) so this
// never happens in production.
func selectParser(fp Fingerprint) Parser {
	for _, f := range registeredParsers {
		parser, ok := f(fp)
		if !ok {
			continue
		}
		return parser
	}
	return nil
}
