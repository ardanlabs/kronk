// Package examples provides a documentation generator for sdk/docs/examples.
package examples

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type example struct {
	name        string
	displayName string
	description string
	code        string
}

var exampleMeta = map[string]struct {
	displayName string
	description string
}{
	"audio":     {"Audio", "Process audio files using audio models"},
	"chat":      {"Chat", "Interactive chat with conversation history"},
	"embedding": {"Embedding", "Generate embeddings for semantic search"},
	"question":  {"Question", "Ask a single question to a model"},
	"rerank":    {"Rerank", "Rerank documents by relevance to a query"},
	"response":  {"Response", "Interactive chat using the Response API with tool calling"},
	"vision":    {"Vision", "Analyze images using vision models"},
}

var exampleOrder = []string{"question", "chat", "response", "embedding", "rerank", "vision", "audio"}

func Run() error {
	examplesDir := "examples"
	outputDir := "cmd/server/api/frontends/bui/src/components"

	var exs []example

	for _, name := range exampleOrder {
		meta, ok := exampleMeta[name]
		if !ok {
			continue
		}

		mainFile := filepath.Join(examplesDir, name, "main.go")
		content, err := os.ReadFile(mainFile)
		if err != nil {
			fmt.Printf("Warning: could not read %s: %v\n", mainFile, err)
			continue
		}

		exs = append(exs, example{
			name:        name,
			displayName: meta.displayName,
			description: meta.description,
			code:        string(content),
		})
	}

	tsx := generateExamplesTSX(exs)

	outputPath := filepath.Join(outputDir, "DocsSDKExamples.tsx")
	if err := os.WriteFile(outputPath, []byte(tsx), 0644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	fmt.Printf("Generated %s\n", outputPath)
	return nil
}

func generateExamplesTSX(exs []example) string {
	var b strings.Builder

	b.WriteString(`import CodeBlock from './CodeBlock';

`)

	for _, ex := range exs {
		varName := ex.name + "Example"
		b.WriteString(fmt.Sprintf("const %s = `%s`;\n\n", varName, escapeForTemplateLiteral(ex.code)))
	}

	b.WriteString("export default function DocsSDKExamples() {\n")
	b.WriteString("  return (\n")
	b.WriteString("    <div>\n")
	b.WriteString("      <div className=\"page-header\">\n")
	b.WriteString("        <h2>SDK Examples</h2>\n")
	b.WriteString("        <p>Complete working examples demonstrating how to use the Kronk SDK</p>\n")
	b.WriteString("      </div>\n\n")

	b.WriteString("      <div className=\"doc-layout\">\n")
	b.WriteString("        <div className=\"doc-content\">\n")

	for _, ex := range exs {
		anchor := toAnchor("example-" + ex.name)
		varName := ex.name + "Example"

		b.WriteString(fmt.Sprintf("\n          <div className=\"card\" id=\"%s\">\n", anchor))
		b.WriteString(fmt.Sprintf("            <h3>%s</h3>\n", ex.displayName))
		b.WriteString(fmt.Sprintf("            <p className=\"doc-description\">%s</p>\n", ex.description))
		b.WriteString(fmt.Sprintf("            <CodeBlock code={%s} language=\"go\" />\n", varName))
		b.WriteString("          </div>\n")
	}

	b.WriteString("        </div>\n")

	b.WriteString("\n        <nav className=\"doc-sidebar\">\n")
	b.WriteString("          <div className=\"doc-sidebar-content\">\n")
	b.WriteString("            <div className=\"doc-index-section\">\n")
	b.WriteString("              <span className=\"doc-index-header\">Examples</span>\n")
	b.WriteString("              <ul>\n")

	for _, ex := range exs {
		anchor := toAnchor("example-" + ex.name)
		b.WriteString(fmt.Sprintf("                <li><a href=\"#%s\">%s</a></li>\n", anchor, ex.displayName))
	}

	b.WriteString("              </ul>\n")
	b.WriteString("            </div>\n")
	b.WriteString("          </div>\n")
	b.WriteString("        </nav>\n")

	b.WriteString("      </div>\n")
	b.WriteString("    </div>\n")
	b.WriteString("  );\n")
	b.WriteString("}\n")

	return b.String()
}

func escapeForTemplateLiteral(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, "${", "\\${")

	return s
}

func toAnchor(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ReplaceAll(s, " ", "-")

	return s
}
