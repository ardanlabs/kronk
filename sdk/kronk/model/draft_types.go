package model

// draftKind identifies a strategy for logging/metrics only. Behavioral
// dispatch remains behind backend and capability interfaces.
type draftKind struct{ value uint8 }

var (
	draftClassic         = draftKind{}         // Separate-GGUF, vocab-matched draft.
	draftMTPQwen         = draftKind{value: 1} // Embedded own-KV MTP head.
	draftMTPQwenSeparate = draftKind{value: 2} // Separate-file own-KV MTP head.
	draftMTPGemma4       = draftKind{value: 3} // Separate-file shared-KV assistant.
)

func (dk draftKind) String() string {
	switch dk {
	case draftClassic:
		return "classic-separate"
	case draftMTPQwen:
		return "mtp-qwen"
	case draftMTPQwenSeparate:
		return "mtp-qwen-separate"
	case draftMTPGemma4:
		return "mtp-gemma4-shared"
	default:
		return "unknown"
	}
}
