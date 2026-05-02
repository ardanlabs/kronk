package kronk

import (
	"sync"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/processors/gemma"
	"github.com/ardanlabs/kronk/sdk/kronk/processors/glm"
	"github.com/ardanlabs/kronk/sdk/kronk/processors/gpt"
	"github.com/ardanlabs/kronk/sdk/kronk/processors/mistral"
	"github.com/ardanlabs/kronk/sdk/kronk/processors/qwen"
	"github.com/ardanlabs/kronk/sdk/kronk/processors/standard"
)

// defaultProcessorsOnce guards default-processor registration so multiple
// kronk instances per process register only once.
var defaultProcessorsOnce sync.Once

// registerDefaultProcessors registers the processor plugins that ship with
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
//     processor even when the more specific processors all decline.
//
// The middle four (qwen, gemma, glm, mistral) inspect architecture +
// template + name internally and do not overlap, so their relative
// order is irrelevant.
//
// This function is idempotent — calling it multiple times has no effect.
// It is called automatically by NewWithContext, so most callers do not
// need to invoke it directly. Callers that want to register a custom
// processor ahead of the defaults should call model.RegisterProcessor(custom)
// before NewWithContext; their factory will be tried first because
// selectProcessor walks registrations in order.
func registerDefaultProcessors() {
	defaultProcessorsOnce.Do(func() {
		model.RegisterProcessor(gpt.New) // template-only — must be first
		model.RegisterProcessor(qwen.New)
		model.RegisterProcessor(gemma.New)
		model.RegisterProcessor(glm.New)
		model.RegisterProcessor(mistral.New)
		model.RegisterProcessor(standard.New) // catch-all — must be last
	})
}
