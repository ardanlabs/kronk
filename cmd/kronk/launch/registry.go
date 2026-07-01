package launch

import (
	"fmt"
	"sort"
	"strings"
)

// registry maps a lower-cased agent name to its Runner. New agents are
// added here as they are supported.
var registry = map[string]Runner{
	"opencode": openCode{},
	"claude":   claudeCode{},
	"codex":    codex{},
	"copilot":  copilot{},
	"pi":       pi{},
	"openclaw": openClaw{},
	"hermes":   hermes{},
	"vscode":   vsCode{},
}

// lookup returns the Runner registered for name.
func lookup(name string) (Runner, error) {
	r, ok := registry[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return nil, fmt.Errorf("unknown agent %q (supported: %s)", name, strings.Join(supported(), ", "))
	}

	return r, nil
}

// supported returns the sorted list of registered agent names.
func supported() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}
