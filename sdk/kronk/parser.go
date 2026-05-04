package kronk

import (
	"sync"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/parsers/gemma"
	"github.com/ardanlabs/kronk/sdk/kronk/parsers/glm"
	"github.com/ardanlabs/kronk/sdk/kronk/parsers/gpt"
	"github.com/ardanlabs/kronk/sdk/kronk/parsers/mistral"
	"github.com/ardanlabs/kronk/sdk/kronk/parsers/qwen"
	"github.com/ardanlabs/kronk/sdk/kronk/parsers/standard"
)

// defaultParsersOnce guards default-parser registration so multiple
// kronk instances per process register only once.
var defaultParsersOnce sync.Once

// registerDefaultParsers registers the parser plugins that ship with
// Kronk. Order matters and follows two rules:
//
//  1. gpt is registered first because it claims solely on chat-template
//     Harmony markers (<|channel|>, <|message|>, etc.). A GPT-OSS model
//     whose GGUF architecture happens to share a prefix with another
//     lineage (e.g. a Qwen-derived gpt-oss build) must still be picked
//     up by gpt rather than the lineage whose architecture prefix it
//     shares.
//
//  2. standard is registered last because it is the catch-all that
//     claims any fingerprint, ensuring every model resolves to a
//     parser even when the more specific parsers all decline.
//
// The middle four (qwen, gemma, glm, mistral) inspect architecture +
// template + name internally and do not overlap, so their relative
// order is irrelevant.
//
// This function is idempotent — calling it multiple times has no effect.
// It is called automatically by NewWithContext, so most callers do not
// need to invoke it directly. Callers that want to register a custom
// parser ahead of the defaults should call model.RegisterParser(custom)
// before NewWithContext; their factory will be tried first because
// selectParser walks registrations in order.
func registerDefaultParsers() {
	defaultParsersOnce.Do(func() {
		model.RegisterParser(gpt.New) // template-only — must be first
		model.RegisterParser(qwen.New)
		model.RegisterParser(gemma.New)
		model.RegisterParser(glm.New)
		model.RegisterParser(mistral.New)
		model.RegisterParser(standard.New) // catch-all — must be last
	})
}
