package models

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const grammarLocalFolder = "grammars"

// ResolveGrammar resolves the grammar field in a SamplingConfig. If the
// grammar value is a .grm filename, the file contents are read and used
// as the grammar content. Otherwise the value is used directly.
func (m *Models) ResolveGrammar(sc *SamplingConfig) error {
	if sc.Grammar == "" {
		return nil
	}

	if !strings.HasSuffix(sc.Grammar, ".grm") {
		return nil
	}

	filePath := filepath.Join(m.basePath, grammarLocalFolder, sc.Grammar)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("resolve-grammar: reading grammar file: %w", err)
	}

	sc.Grammar = string(content)

	return nil
}
