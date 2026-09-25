// Package metadatafmt formats GGUF metadata for terminal output.
package metadatafmt

import (
	"fmt"
	"strings"
)

// Value truncates long array values while preserving scalar and short values.
func Value(value string) string {
	if len(value) < 2 || value[0] != '[' {
		return value
	}

	inner := value[1 : len(value)-1]
	elements := strings.Split(inner, " ")

	if len(elements) <= 6 {
		return value
	}

	first := elements[:3]

	return fmt.Sprintf("[%s, ...]", strings.Join(first, ", "))
}
