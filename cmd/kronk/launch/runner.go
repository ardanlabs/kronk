// Package launch provides the "kronk launch" command, which starts a
// coding agent (currently OpenCode) pre-wired to the local Kronk server
// and the chat models installed on it.
package launch

// Runner launches a coding agent configured to talk to the local Kronk
// server.
//
// defaultModel is the model the agent should select on start; it is always
// one of the ids in models. models is the full set of chat-capable models
// to expose to the agent. args are passed through to the agent unchanged.
type Runner interface {
	Run(defaultModel string, models []Model, args []string) error
}
