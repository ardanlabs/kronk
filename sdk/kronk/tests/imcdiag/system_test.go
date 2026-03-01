package imcdiag_test

import _ "embed"

//go:embed system_prompt.txt
var SystemPrompt string

//go:embed turns_vision.txt
var turnsVisionRaw string

//go:embed turns_text.txt
var turnsTextRaw string
