// Package examples provides a documentation generator for sdk/docs/examples.
package examples

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type example struct {
	name        string
	displayName string
	description string
	code        string
}

var skipDirs = map[string]bool{
	"samples": true,
	"yzma":    true,
}

func Run() error {
	examplesDir := "examples"
	outputDir := "cmd/server/api/frontends/bui/src/components"

	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		return fmt.Errorf("reading examples directory: %w", err)
	}

	var exs []example

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		if skipDirs[name] {
			continue
		}

		mainFile := filepath.Join(examplesDir, name, "main.go")
		content, err := os.ReadFile(mainFile)
		if err != nil {
			fmt.Printf("Warning: could not read %s: %v\n", mainFile, err)
			continue
		}

		description := extractDescription(string(content))
		displayName := cases.Title(language.English).String(name)

		exs = append(exs, example{
			name:        name,
			displayName: displayName,
			description: description,
			code:        string(content),
		})
	}

	slices.SortFunc(exs, func(a, b example) int {
		return strings.Compare(a.name, b.name)
	})

	tsx := generateExamplesTSX(exs)

	outputPath := filepath.Join(outputDir, "DocsSDKExamples.tsx")
	if err := os.WriteFile(outputPath, []byte(tsx), 0644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	fmt.Printf("Generated %s\n", outputPath)

	if err := updateLayoutExamples(outputDir, exs); err != nil {
		return fmt.Errorf("updating Layout.tsx: %w", err)
	}

	return nil
}

func extractDescription(code string) string {
	lines := strings.Split(code, "\n")

	if len(lines) == 0 || !strings.HasPrefix(lines[0], "//") {
		return ""
	}

	desc := strings.TrimPrefix(lines[0], "//")
	desc = strings.TrimSpace(desc)

	return desc
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

func updateLayoutExamples(outputDir string, exs []example) error {
	layoutPath := filepath.Join(outputDir, "Layout.tsx")
	content, err := os.ReadFile(layoutPath)
	if err != nil {
		return fmt.Errorf("reading Layout.tsx: %w", err)
	}

	layoutStr := string(content)

	// Find the SDK section and replace the examples items.
	const startMarker = "{ page: 'docs-sdk-examples', label: 'Examples' },"
	const endMarker = "id: 'docs-cli-sub',"

	startIdx := strings.Index(layoutStr, startMarker)
	if startIdx == -1 {
		return fmt.Errorf("could not find SDK examples marker in Layout.tsx")
	}

	endIdx := strings.Index(layoutStr[startIdx:], endMarker)
	if endIdx == -1 {
		return fmt.Errorf("could not find CLI section marker in Layout.tsx")
	}

	// Build the new examples items.
	var items strings.Builder
	items.WriteString(startMarker + "\n")
	for _, ex := range exs {
		anchor := toAnchor("example-" + ex.name)
		items.WriteString(fmt.Sprintf("          { page: 'docs-sdk-examples', label: '%s', hash: '%s' },\n", ex.displayName, anchor))
	}
	items.WriteString("        ],\n      },\n      {\n        ")

	// Replace the section.
	newLayout := layoutStr[:startIdx] + items.String() + layoutStr[startIdx+endIdx:]

	if err := os.WriteFile(layoutPath, []byte(newLayout), 0644); err != nil {
		return fmt.Errorf("writing Layout.tsx: %w", err)
	}

	return nil
}
