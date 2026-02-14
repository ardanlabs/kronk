package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// retrieveGrammarScript returns the contents of the grammar file.
func (c *Catalog) retrieveGrammarScript(grammarFileName string) (string, error) {
	filePath := filepath.Join(c.grammars.grammarPath, grammarFileName)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("retrieve-grammar-script: reading grammar file: %w", err)
	}

	return string(content), nil
}

// resolveGrammar resolves the grammar field in a SamplingConfig. If the
// grammar value is a .grm filename, the file contents are read and used
// as the grammar content. Otherwise the value is used directly.
func (c *Catalog) resolveGrammar(sc *SamplingConfig) error {
	if sc.Grammar == "" {
		return nil
	}

	if !strings.HasSuffix(sc.Grammar, ".grm") {
		return nil
	}

	content, err := c.retrieveGrammarScript(sc.Grammar)
	if err != nil {
		return fmt.Errorf("resolve-grammar: %w", err)
	}

	sc.Grammar = content

	return nil
}
