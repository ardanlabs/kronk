package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveGrammar resolves the grammar field in a SamplingConfig. If the
// grammar value is a .grm filename, the file contents are read and used
// as the grammar content. Otherwise the value is used directly.
func (c *Catalog) ResolveGrammar(sc *SamplingConfig) error {
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

// GrammarFiles returns a sorted list of available grammar filenames.
func (c *Catalog) GrammarFiles() ([]string, error) {
	entries, err := os.ReadDir(c.grammars.grammarPath)
	if err != nil {
		return nil, fmt.Errorf("grammar-files: reading grammars directory: %w", err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		if !strings.HasSuffix(name, ".grm") {
			continue
		}

		files = append(files, name)
	}

	return files, nil
}

// GrammarContent returns the contents of a grammar file by name.
func (c *Catalog) GrammarContent(name string) (string, error) {
	return c.retrieveGrammarScript(name)
}

// retrieveGrammarScript returns the contents of the grammar file.
func (c *Catalog) retrieveGrammarScript(grammarFileName string) (string, error) {
	filePath := filepath.Join(c.grammars.grammarPath, grammarFileName)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("retrieve-grammar-script: reading grammar file: %w", err)
	}

	return string(content), nil
}
