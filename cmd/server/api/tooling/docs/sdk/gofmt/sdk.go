// Package gofmt provides a documentation generator for sdk/docs/kronk and models.
package gofmt

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

type packageDocs struct {
	name        string
	importPath  string
	description string
	constants   []constGroup
	variables   []variable
	types       []typeDef
	functions   []function
}

type constGroup struct {
	name    string
	code    string
	comment string
}

type variable struct {
	name    string
	code    string
	comment string
}

type typeDef struct {
	name         string
	code         string
	comment      string
	constructors []function
	methods      []function
	isExported   bool
}

type function struct {
	name      string
	signature string
	comment   string
	receiver  string
}

// =============================================================================

func Run() error {
	pkg := flag.String("pkg", "all", "Package to generate docs for: kronk, model, or all")
	flag.Parse()

	packages := make(map[string]string)
	packages["kronk"] = "github.com/ardanlabs/kronk/sdk/kronk"
	packages["model"] = "github.com/ardanlabs/kronk/sdk/kronk/model"

	outputDir := "/Users/bill/code/go/src/github.com/ardanlabs/kronk/cmd/server/api/frontends/bui/src/components"

	switch *pkg {
	case "kronk":
		if err := generateDocs(packages["kronk"], outputDir, "DocsSDKKronk.tsx", "Kronk"); err != nil {
			return fmt.Errorf("generating kronk docs: %w", err)
		}

	case "model":
		if err := generateDocs(packages["model"], outputDir, "DocsSDKModel.tsx", "Model"); err != nil {
			return fmt.Errorf("generating model docs: %w", err)
		}

	case "all":
		if err := generateDocs(packages["kronk"], outputDir, "DocsSDKKronk.tsx", "Kronk"); err != nil {
			return fmt.Errorf("generating kronk docs: %w", err)
		}

		if err := generateDocs(packages["model"], outputDir, "DocsSDKModel.tsx", "Model"); err != nil {
			return fmt.Errorf("generating model docs: %w", err)
		}

	default:
		return fmt.Errorf("unknown package: %s (use kronk, model, or all)", *pkg)
	}

	fmt.Println("Documentation generated successfully")

	return nil
}

func generateDocs(importPath, outputDir, filename, displayName string) error {
	cmd := exec.Command("go", "doc", "-all", importPath)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("running go doc: %w", err)
	}

	docs, err := parseGoDoc(string(output), importPath)
	if err != nil {
		return fmt.Errorf("parsing go doc: %w", err)
	}

	tsx := generateTSX(docs, displayName)

	outputPath := outputDir + "/" + filename
	if err := os.WriteFile(outputPath, []byte(tsx), 0644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	fmt.Printf("Generated %s\n", outputPath)
	return nil
}

func parseGoDoc(output, importPath string) (*packageDocs, error) {
	docs := &packageDocs{
		importPath: importPath,
	}

	lines := strings.Split(output, "\n")
	scanner := &lineScanner{lines: lines, pos: 0}

	for scanner.hasNext() {
		line := scanner.next()

		if strings.HasPrefix(line, "package ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				docs.name = parts[1]
			}

			for scanner.hasNext() {
				nextLine := scanner.peek()
				if nextLine == "" {
					scanner.next()
					continue
				}
				break
			}

			docs.description = parseDescription(scanner)
			continue
		}

		if line == "CONSTANTS" {
			docs.constants = parseConstants(scanner)
			continue
		}

		if line == "VARIABLES" {
			docs.variables = parseVariables(scanner)
			continue
		}

		if line == "FUNCTIONS" {
			docs.functions = parseFunctions(scanner)
			continue
		}

		if line == "TYPES" {
			docs.types = parseTypes(scanner)
			continue
		}
	}

	return docs, nil
}

type lineScanner struct {
	lines []string
	pos   int
}

func (s *lineScanner) hasNext() bool {
	return s.pos < len(s.lines)
}

func (s *lineScanner) next() string {
	if s.pos >= len(s.lines) {
		return ""
	}

	line := s.lines[s.pos]
	s.pos++

	return line
}

func (s *lineScanner) peek() string {
	if s.pos >= len(s.lines) {
		return ""
	}
	return s.lines[s.pos]
}

func (s *lineScanner) back() {
	if s.pos > 0 {
		s.pos--
	}
}

func parseDescription(scanner *lineScanner) string {
	var desc []string

	for scanner.hasNext() {
		line := scanner.next()
		if line == "" {
			break
		}

		if strings.HasPrefix(line, "CONSTANTS") || strings.HasPrefix(line, "VARIABLES") ||
			strings.HasPrefix(line, "FUNCTIONS") || strings.HasPrefix(line, "TYPES") {
			scanner.back()
			break
		}

		desc = append(desc, strings.TrimSpace(line))
	}

	return strings.Join(desc, " ")
}

func parseConstants(scanner *lineScanner) []constGroup {
	var groups []constGroup

	for scanner.hasNext() {
		line := scanner.peek()

		if line == "VARIABLES" || line == "FUNCTIONS" || line == "TYPES" {
			break
		}

		scanner.next()

		if strings.HasPrefix(line, "const ") || strings.HasPrefix(line, "const\t") {
			code, comment := parseConstBlock(line, scanner)
			name := extractConstName(code)
			groups = append(groups, constGroup{
				name:    name,
				code:    code,
				comment: comment,
			})
		}
	}

	return groups
}

func parseConstBlock(firstLine string, scanner *lineScanner) (string, string) {
	codeLines := []string{
		firstLine,
	}

	if strings.Contains(firstLine, "(") && !strings.Contains(firstLine, ")") {
		for scanner.hasNext() {
			line := scanner.next()
			codeLines = append(codeLines, line)

			if strings.HasPrefix(line, ")") {
				break
			}
		}
	}

	var commentLines []string

	for scanner.hasNext() {
		line := scanner.peek()
		if line == "" {
			scanner.next()
			continue
		}

		if !strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "\t") {
			break
		}

		scanner.next()

		commentLines = append(commentLines, strings.TrimSpace(line))
	}

	return strings.Join(codeLines, "\n"), strings.Join(commentLines, " ")
}

var reConstName1 = regexp.MustCompile(`(\w+)\s*=`)
var reConstName2 = regexp.MustCompile(`const\s+(\w+)`)

func extractConstName(code string) string {
	if strings.Contains(code, "(") {
		matches := reConstName1.FindStringSubmatch(code)
		if len(matches) > 1 {
			return matches[1]
		}

		return "Constants"
	}

	matches := reConstName2.FindStringSubmatch(code)
	if len(matches) > 1 {
		return matches[1]
	}

	return "Constant"
}

func parseVariables(scanner *lineScanner) []variable {
	var vars []variable

	for scanner.hasNext() {
		line := scanner.peek()

		if line == "CONSTANTS" || line == "FUNCTIONS" || line == "TYPES" {
			break
		}

		scanner.next()

		if strings.HasPrefix(line, "var ") || strings.HasPrefix(line, "var\t") {
			code, comment := parseVarBlock(line, scanner)
			name := extractVarName(code)
			vars = append(vars, variable{
				name:    name,
				code:    code,
				comment: comment,
			})
		}
	}

	return vars
}

func parseVarBlock(firstLine string, scanner *lineScanner) (string, string) {
	var codeLines []string
	codeLines = append(codeLines, firstLine)

	braceCount := strings.Count(firstLine, "{") - strings.Count(firstLine, "}")

	for braceCount > 0 && scanner.hasNext() {
		line := scanner.next()
		codeLines = append(codeLines, line)
		braceCount += strings.Count(line, "{") - strings.Count(line, "}")
	}

	var commentLines []string

	for scanner.hasNext() {
		line := scanner.peek()
		if line == "" {
			scanner.next()
			continue
		}

		if !strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "\t") {
			break
		}

		scanner.next()

		commentLines = append(commentLines, strings.TrimSpace(line))
	}

	return strings.Join(codeLines, "\n"), strings.Join(commentLines, " ")
}

var reVarName = regexp.MustCompile(`var\s+(\w+)`)

func extractVarName(code string) string {
	matches := reVarName.FindStringSubmatch(code)
	if len(matches) > 1 {
		return matches[1]
	}

	return "Variable"
}

func parseFunctions(scanner *lineScanner) []function {
	var funcs []function

	for scanner.hasNext() {
		line := scanner.peek()

		if line == "CONSTANTS" || line == "VARIABLES" || line == "TYPES" {
			break
		}

		scanner.next()

		if strings.HasPrefix(line, "func ") {
			fn := parseFunction(line, scanner)
			funcs = append(funcs, fn)
		}
	}

	return funcs
}

func parseFunction(signature string, scanner *lineScanner) function {
	fn := function{
		signature: strings.TrimSpace(signature),
	}

	fn.name = extractFuncName(signature)
	fn.receiver = extractReceiver(signature)

	var commentLines []string

	for scanner.hasNext() {
		line := scanner.peek()
		if line == "" {
			scanner.next()
			continue
		}

		if !strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "\t") {
			break
		}

		scanner.next()

		commentLines = append(commentLines, strings.TrimSpace(line))
	}

	fn.comment = strings.Join(commentLines, " ")

	return fn
}

var reFuncName = regexp.MustCompile(`func\s+(?:\([^)]+\)\s+)?(\w+)`)

func extractFuncName(signature string) string {
	matches := reFuncName.FindStringSubmatch(signature)
	if len(matches) > 1 {
		return matches[1]
	}

	return "Function"
}

var reReceiver = regexp.MustCompile(`func\s+\((\w+)\s+\*?(\w+)\)`)

func extractReceiver(signature string) string {
	matches := reReceiver.FindStringSubmatch(signature)
	if len(matches) > 2 {
		return matches[2]
	}

	return ""
}

func parseTypes(scanner *lineScanner) []typeDef {
	var types []typeDef

	for scanner.hasNext() {
		line := scanner.peek()

		if line == "CONSTANTS" || line == "VARIABLES" || line == "FUNCTIONS" {
			break
		}

		scanner.next()

		if strings.HasPrefix(line, "type ") {
			td := parseType(line, scanner)
			if td.isExported {
				types = append(types, td)
			}
		}

		if strings.HasPrefix(line, "func ") {
			fn := parseFunction(line, scanner)

			if fn.receiver != "" {
				for i := range types {
					if types[i].name == fn.receiver {
						types[i].methods = append(types[i].methods, fn)
						break
					}
				}
				continue
			}

			returnType := extractReturnType(fn.signature)
			if returnType != "" {
				for i := range types {
					if types[i].name == returnType {
						types[i].constructors = append(types[i].constructors, fn)
						break
					}
				}
			}
		}
	}

	return types
}

var reReturnType = regexp.MustCompile(`\)\s*\(\s*\*?(\w+)`)

func extractReturnType(signature string) string {
	matches := reReturnType.FindStringSubmatch(signature)
	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}

var reParseType = regexp.MustCompile(`type\s+(\w+)`)

func parseType(firstLine string, scanner *lineScanner) typeDef {
	td := typeDef{
		isExported: true,
	}

	matches := reParseType.FindStringSubmatch(firstLine)
	if len(matches) > 1 {
		td.name = matches[1]
		if len(td.name) > 0 && td.name[0] >= 'a' && td.name[0] <= 'z' {
			td.isExported = false
		}
	}

	codeLines := []string{
		firstLine,
	}

	if strings.Contains(firstLine, "{") && !strings.Contains(firstLine, "}") {
		for scanner.hasNext() {
			line := scanner.next()
			codeLines = append(codeLines, line)

			if strings.HasPrefix(line, "}") {
				break
			}
		}
	}

	td.code = strings.Join(codeLines, "\n")

	var commentLines []string

	for scanner.hasNext() {
		line := scanner.peek()
		if line == "" {
			scanner.next()
			continue
		}

		if !strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "\t") {
			break
		}

		scanner.next()

		commentLines = append(commentLines, strings.TrimSpace(line))
	}

	td.comment = strings.Join(commentLines, " ")

	return td
}

func generateTSX(docs *packageDocs, displayName string) string {
	var b strings.Builder

	var standaloneFuncs []function
	for _, f := range docs.functions {
		if f.receiver == "" {
			standaloneFuncs = append(standaloneFuncs, f)
		}
	}
	for _, t := range docs.types {
		standaloneFuncs = append(standaloneFuncs, t.constructors...)
	}

	var allMethods []function
	for _, t := range docs.types {
		allMethods = append(allMethods, t.methods...)
	}

	b.WriteString(fmt.Sprintf("export default function DocsSDK%s() {\n", displayName))
	b.WriteString("  return (\n")
	b.WriteString("    <div>\n")

	b.WriteString("      <div className=\"page-header\">\n")
	b.WriteString(fmt.Sprintf("        <h2>%s Package</h2>\n", displayName))
	b.WriteString(fmt.Sprintf("        <p>%s</p>\n", escapeJSX(docs.description)))
	b.WriteString("      </div>\n\n")

	b.WriteString("      <div className=\"doc-layout\">\n")
	b.WriteString("        <div className=\"doc-content\">\n")

	b.WriteString("          <div className=\"card\">\n")
	b.WriteString("            <h3>Import</h3>\n")
	b.WriteString("            <pre className=\"code-block\">\n")
	b.WriteString(fmt.Sprintf("              <code>import \"%s\"</code>\n", docs.importPath))
	b.WriteString("            </pre>\n")
	b.WriteString("          </div>\n")

	if len(standaloneFuncs) > 0 {
		b.WriteString("\n          <div className=\"card\" id=\"functions\">\n")
		b.WriteString("            <h3>Functions</h3>\n")
		for _, f := range standaloneFuncs {
			anchor := toAnchor("func-" + f.name)
			b.WriteString(fmt.Sprintf("\n            <div className=\"doc-section\" id=\"%s\">\n", anchor))
			b.WriteString(fmt.Sprintf("              <h4>%s</h4>\n", f.name))
			b.WriteString("              <pre className=\"code-block\">\n")
			b.WriteString(fmt.Sprintf("                <code>%s</code>\n", escapeJSX(f.signature)))
			b.WriteString("              </pre>\n")
			if f.comment != "" {
				b.WriteString(fmt.Sprintf("              <p className=\"doc-description\">%s</p>\n", escapeJSX(f.comment)))
			}
			b.WriteString("            </div>\n")
		}
		b.WriteString("          </div>\n")
	}

	if len(docs.types) > 0 {
		b.WriteString("\n          <div className=\"card\" id=\"types\">\n")
		b.WriteString("            <h3>Types</h3>\n")
		for _, t := range docs.types {
			anchor := toAnchor("type-" + t.name)
			b.WriteString(fmt.Sprintf("\n            <div className=\"doc-section\" id=\"%s\">\n", anchor))
			b.WriteString(fmt.Sprintf("              <h4>%s</h4>\n", t.name))
			b.WriteString("              <pre className=\"code-block\">\n")
			b.WriteString(fmt.Sprintf("                <code>{`%s`}</code>\n", escapeTemplateLiteral(t.code)))
			b.WriteString("              </pre>\n")
			if t.comment != "" {
				b.WriteString(fmt.Sprintf("              <p className=\"doc-description\">%s</p>\n", escapeJSX(t.comment)))
			}
			b.WriteString("            </div>\n")
		}
		b.WriteString("          </div>\n")
	}

	if len(allMethods) > 0 {
		b.WriteString("\n          <div className=\"card\" id=\"methods\">\n")
		b.WriteString("            <h3>Methods</h3>\n")
		for _, m := range allMethods {
			methodName := m.name
			if m.receiver != "" {
				methodName = m.receiver + "." + m.name
			}
			anchor := toAnchor("method-" + methodName)
			b.WriteString(fmt.Sprintf("\n            <div className=\"doc-section\" id=\"%s\">\n", anchor))
			b.WriteString(fmt.Sprintf("              <h4>%s</h4>\n", methodName))
			b.WriteString("              <pre className=\"code-block\">\n")
			b.WriteString(fmt.Sprintf("                <code>%s</code>\n", escapeJSX(m.signature)))
			b.WriteString("              </pre>\n")
			if m.comment != "" {
				b.WriteString(fmt.Sprintf("              <p className=\"doc-description\">%s</p>\n", escapeJSX(m.comment)))
			}
			b.WriteString("            </div>\n")
		}
		b.WriteString("          </div>\n")
	}

	if len(docs.constants) > 0 {
		b.WriteString("\n          <div className=\"card\" id=\"constants\">\n")
		b.WriteString("            <h3>Constants</h3>\n")
		for _, c := range docs.constants {
			anchor := toAnchor("const-" + c.name)
			b.WriteString(fmt.Sprintf("\n            <div className=\"doc-section\" id=\"%s\">\n", anchor))
			b.WriteString(fmt.Sprintf("              <h4>%s</h4>\n", c.name))
			b.WriteString("              <pre className=\"code-block\">\n")
			b.WriteString(fmt.Sprintf("                <code>{`%s`}</code>\n", escapeTemplateLiteral(c.code)))
			b.WriteString("              </pre>\n")
			if c.comment != "" {
				b.WriteString(fmt.Sprintf("              <p className=\"doc-description\">%s</p>\n", escapeJSX(c.comment)))
			}
			b.WriteString("            </div>\n")
		}
		b.WriteString("          </div>\n")
	}

	if len(docs.variables) > 0 {
		b.WriteString("\n          <div className=\"card\" id=\"variables\">\n")
		b.WriteString("            <h3>Variables</h3>\n")
		for _, v := range docs.variables {
			anchor := toAnchor("var-" + v.name)
			b.WriteString(fmt.Sprintf("\n            <div className=\"doc-section\" id=\"%s\">\n", anchor))
			b.WriteString(fmt.Sprintf("              <h4>%s</h4>\n", v.name))
			b.WriteString("              <pre className=\"code-block\">\n")
			b.WriteString(fmt.Sprintf("                <code>{`%s`}</code>\n", escapeTemplateLiteral(v.code)))
			b.WriteString("              </pre>\n")
			if v.comment != "" {
				b.WriteString(fmt.Sprintf("              <p className=\"doc-description\">%s</p>\n", escapeJSX(v.comment)))
			}
			b.WriteString("            </div>\n")
		}
		b.WriteString("          </div>\n")
	}

	b.WriteString("        </div>\n")

	b.WriteString("\n        <nav className=\"doc-sidebar\">\n")
	b.WriteString("          <div className=\"doc-sidebar-content\">\n")

	if len(standaloneFuncs) > 0 {
		b.WriteString("            <div className=\"doc-index-section\">\n")
		b.WriteString("              <a href=\"#functions\" className=\"doc-index-header\">Functions</a>\n")
		b.WriteString("              <ul>\n")
		for _, f := range standaloneFuncs {
			anchor := toAnchor("func-" + f.name)
			b.WriteString(fmt.Sprintf("                <li><a href=\"#%s\">%s</a></li>\n", anchor, f.name))
		}
		b.WriteString("              </ul>\n")
		b.WriteString("            </div>\n")
	}

	if len(docs.types) > 0 {
		b.WriteString("            <div className=\"doc-index-section\">\n")
		b.WriteString("              <a href=\"#types\" className=\"doc-index-header\">Types</a>\n")
		b.WriteString("              <ul>\n")
		for _, t := range docs.types {
			anchor := toAnchor("type-" + t.name)
			b.WriteString(fmt.Sprintf("                <li><a href=\"#%s\">%s</a></li>\n", anchor, t.name))
		}
		b.WriteString("              </ul>\n")
		b.WriteString("            </div>\n")
	}

	if len(allMethods) > 0 {
		b.WriteString("            <div className=\"doc-index-section\">\n")
		b.WriteString("              <a href=\"#methods\" className=\"doc-index-header\">Methods</a>\n")
		b.WriteString("              <ul>\n")
		for _, m := range allMethods {
			methodName := m.name
			if m.receiver != "" {
				methodName = m.receiver + "." + m.name
			}
			anchor := toAnchor("method-" + methodName)
			b.WriteString(fmt.Sprintf("                <li><a href=\"#%s\">%s</a></li>\n", anchor, methodName))
		}
		b.WriteString("              </ul>\n")
		b.WriteString("            </div>\n")
	}

	if len(docs.constants) > 0 {
		b.WriteString("            <div className=\"doc-index-section\">\n")
		b.WriteString("              <a href=\"#constants\" className=\"doc-index-header\">Constants</a>\n")
		b.WriteString("              <ul>\n")
		for _, c := range docs.constants {
			anchor := toAnchor("const-" + c.name)
			b.WriteString(fmt.Sprintf("                <li><a href=\"#%s\">%s</a></li>\n", anchor, c.name))
		}
		b.WriteString("              </ul>\n")
		b.WriteString("            </div>\n")
	}

	if len(docs.variables) > 0 {
		b.WriteString("            <div className=\"doc-index-section\">\n")
		b.WriteString("              <a href=\"#variables\" className=\"doc-index-header\">Variables</a>\n")
		b.WriteString("              <ul>\n")
		for _, v := range docs.variables {
			anchor := toAnchor("var-" + v.name)
			b.WriteString(fmt.Sprintf("                <li><a href=\"#%s\">%s</a></li>\n", anchor, v.name))
		}
		b.WriteString("              </ul>\n")
		b.WriteString("            </div>\n")
	}

	b.WriteString("          </div>\n")
	b.WriteString("        </nav>\n")

	b.WriteString("      </div>\n")
	b.WriteString("    </div>\n")
	b.WriteString("  );\n")
	b.WriteString("}\n")

	return b.String()
}

func toAnchor(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ReplaceAll(s, " ", "-")

	return s
}

func escapeJSX(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "{", "&#123;")
	s = strings.ReplaceAll(s, "}", "&#125;")

	return s
}

func escapeTemplateLiteral(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, "${", "\\${")

	return s
}
