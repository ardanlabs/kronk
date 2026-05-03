package model

// =============================================================================
// Processor Registry
//
// The registry holds an ordered list of processor factories. selectProcessor
// walks the list in registration order and returns the first processor that
// claims the given Fingerprint. Bootstrap is responsible for explicit
// registration:
//
// model.RegisterProcessor(gpt.New) // template-only — must be first
// model.RegisterProcessor(qwen.New)
// model.RegisterProcessor(gemma.New)
// model.RegisterProcessor(glm.New)
// model.RegisterProcessor(mistral.New)
// model.RegisterProcessor(standard.New) // catch-all — must be last
//
// There is no init()-time auto-registration. Every binary that needs
// processor support enumerates the processors it wants. This keeps the
// wired-in set visible via `grep RegisterProcessor` and avoids accidental
// dependencies pulled in by blank imports.
// =============================================================================

// ProcessorFactory is the constructor signature each processor package's
// New function satisfies. The bool return reports whether this processor
// claims the given Fingerprint; on false, the registry continues to the
// next factory.
type ProcessorFactory func(Fingerprint) (Processor, bool)

// registeredProcessors is the ordered registry. Earlier entries take
// precedence over later ones. The slice is built at startup via
// RegisterProcessor and read at Model.Load via selectProcessor — there is
// no concurrent mutation in practice, so no lock is needed.
var registeredProcessors []ProcessorFactory

// RegisterProcessor appends a processor factory to the registry. Call once
// per processor at server bootstrap, before any models are loaded. Order
// matters: the catch-all processor (standard) must be registered last so
// the more specific processors get first chance to claim.
func RegisterProcessor(f ProcessorFactory) {
	registeredProcessors = append(registeredProcessors, f)
}

// selectProcessor walks the registered factories in registration order and
// returns the first Processor that claims the fingerprint.
//
// Returns nil if no factory claims — bootstrap should always register a
// catch-all (typically processors/standard.New, registered last) so this
// never happens in production.
func selectProcessor(fp Fingerprint) Processor {
	for _, f := range registeredProcessors {
		processor, ok := f(fp)
		if !ok {
			continue
		}
		return processor
	}
	return nil
}
