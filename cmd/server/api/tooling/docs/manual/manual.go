// Package manual provides a documentation generator that converts MANUAL.md to React.
package manual

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

type chapter struct {
	id       string
	title    string
	sections []section
}

type section struct {
	id    string
	title string
	level int
}

func Run() error {
	manualPath := "MANUAL.md"
	outputDir := "cmd/server/api/frontends/bui/src/components"

	content, err := os.ReadFile(manualPath)
	if err != nil {
		return fmt.Errorf("reading MANUAL.md: %w", err)
	}

	tsx := generateManualTSX(string(content))

	outputPath := outputDir + "/DocsManual.tsx"
	if err := os.WriteFile(outputPath, []byte(tsx), 0644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	fmt.Printf("Generated %s\n", outputPath)
	return nil
}

func generateManualTSX(markdown string) string {
	var b strings.Builder

	chapters := parseChapters(markdown)
	htmlContent := markdownToJSX(markdown)

	b.WriteString(`import { useState, useEffect } from 'react';
import { useLocation } from 'react-router-dom';

export default function DocsManual() {
  const [activeSection, setActiveSection] = useState('');
  const location = useLocation();

  useEffect(() => {
    if (location.hash) {
      const id = location.hash.slice(1);
      const element = document.getElementById(id);
      if (element) {
        setTimeout(() => {
          element.scrollIntoView({ behavior: 'smooth', block: 'start' });
        }, 100);
      }
    }
  }, [location.hash]);

  useEffect(() => {
    const handleScroll = () => {
      const sections = document.querySelectorAll('.manual-content h2, .manual-content h3');
      let current = '';
      sections.forEach((section) => {
        const rect = section.getBoundingClientRect();
        if (rect.top <= 100) {
          current = section.id;
        }
      });
      setActiveSection(current);
    };

    window.addEventListener('scroll', handleScroll);
    return () => window.removeEventListener('scroll', handleScroll);
  }, []);

  return (
    <div>
      <div className="page-header">
        <h2>Kronk Manual</h2>
        <p>Complete documentation for the Kronk Model Server</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content manual-content">
`)

	b.WriteString(htmlContent)

	b.WriteString(`
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
`)

	for _, ch := range chapters {
		b.WriteString("            <div className=\"doc-index-section\">\n")
		fmt.Fprintf(&b, "              <a href=\"#%s\" className={`doc-index-header ${activeSection === '%s' ? 'active' : ''}`}>%s</a>\n",
			ch.id, ch.id, escapeJSX(ch.title))

		if len(ch.sections) > 0 {
			b.WriteString("              <ul>\n")
			for _, sec := range ch.sections {
				fmt.Fprintf(&b, "                <li><a href=\"#%s\" className={activeSection === '%s' ? 'active' : ''}>%s</a></li>\n",
					sec.id, sec.id, escapeJSX(sec.title))
			}
			b.WriteString("              </ul>\n")
		}

		b.WriteString("            </div>\n")
	}

	b.WriteString(`          </div>
        </nav>
      </div>
    </div>
  );
}
`)

	return b.String()
}

func parseChapters(markdown string) []chapter {
	var chapters []chapter
	lines := strings.Split(markdown, "\n")

	var currentChapter *chapter

	reH2 := regexp.MustCompile(`^## (.+)`)
	reH3 := regexp.MustCompile(`^### (.+)`)

	for _, line := range lines {
		switch {
		case reH2.MatchString(line):
			if currentChapter != nil {
				chapters = append(chapters, *currentChapter)
			}
			title := reH2.FindStringSubmatch(line)[1]
			currentChapter = &chapter{
				id:    toAnchor(title),
				title: title,
			}
		case reH3.MatchString(line) && currentChapter != nil:
			title := reH3.FindStringSubmatch(line)[1]
			currentChapter.sections = append(currentChapter.sections, section{
				id:    toAnchor(title),
				title: title,
				level: 3,
			})
		}
	}

	if currentChapter != nil {
		chapters = append(chapters, *currentChapter)
	}

	return chapters
}

var reOrderedList = regexp.MustCompile(`^\d+\. (.+)`)

func markdownToJSX(markdown string) string {
	lines := strings.Split(markdown, "\n")
	var result []string
	inCodeBlock := false
	codeBlockLang := ""
	var codeLines []string
	inTable := false
	var tableLines []string
	inUL := false
	var ulItems []string
	inOL := false
	var olItems []string
	var olSubItems [][]string // Sub-items (nested UL) per OL item
	var paraLines []string

	flushParagraph := func() {
		if len(paraLines) > 0 {
			merged := strings.Join(paraLines, " ")
			result = append(result, fmt.Sprintf("          <p>%s</p>", convertInlineMarkdown(merged)))
			paraLines = nil
		}
	}

	flushUL := func() {
		flushParagraph()
		if len(ulItems) > 0 {
			result = append(result, "          <ul>")
			for _, item := range ulItems {
				result = append(result, fmt.Sprintf("            <li>%s</li>", item))
			}
			result = append(result, "          </ul>")
			ulItems = nil
		}
		inUL = false
	}

	flushOL := func() {
		flushParagraph()
		if len(olItems) > 0 {
			result = append(result, "          <ol>")
			for j, item := range olItems {
				switch {
				case j < len(olSubItems) && len(olSubItems[j]) > 0:
					result = append(result, fmt.Sprintf("            <li>%s", item))
					result = append(result, "              <ul>")
					for _, sub := range olSubItems[j] {
						result = append(result, fmt.Sprintf("                <li>%s</li>", sub))
					}
					result = append(result, "              </ul>")
					result = append(result, "            </li>")
				default:
					result = append(result, fmt.Sprintf("            <li>%s</li>", item))
				}
			}
			result = append(result, "          </ol>")
			olItems = nil
			olSubItems = nil
		}
		inOL = false
	}

	for i := range lines {
		line := lines[i]

		if strings.HasPrefix(line, "```") {
			flushParagraph()
			flushUL()
			flushOL()

			switch {
			case !inCodeBlock:
				inCodeBlock = true
				codeBlockLang = strings.TrimPrefix(line, "```")
				codeLines = nil
			default:
				code := escapeForTemplateLiteral(strings.Join(codeLines, "\n"))

				switch {
				case codeBlockLang != "":
					result = append(result, fmt.Sprintf("          <pre className=\"code-block\"><code className=\"language-%s\">{`%s`}</code></pre>", codeBlockLang, code))
				default:
					result = append(result, fmt.Sprintf("          <pre className=\"code-block\"><code>{`%s`}</code></pre>", code))
				}

				inCodeBlock = false
				codeBlockLang = ""
			}
			continue
		}

		if inCodeBlock {
			codeLines = append(codeLines, line)
			continue
		}

		switch {
		case strings.HasPrefix(line, "|") && strings.Contains(line, "|"):
			flushParagraph()
			flushUL()
			flushOL()
			if !inTable {
				inTable = true
				tableLines = nil
			}
			tableLines = append(tableLines, line)
			continue
		case inTable:
			result = append(result, convertTable(tableLines))
			inTable = false
			tableLines = nil
		}

		if inOL {
			trimmed := strings.TrimLeft(line, " \t")
			if item, ok := strings.CutPrefix(trimmed, "- "); ok && line != trimmed {
				if len(olSubItems) == 0 {
					olSubItems = make([][]string, len(olItems))
				}
				for len(olSubItems) < len(olItems) {
					olSubItems = append(olSubItems, nil)
				}
				olSubItems[len(olItems)-1] = append(olSubItems[len(olItems)-1], convertInlineMarkdown(item))
				continue
			}
		}

		switch {
		case strings.HasPrefix(line, "- "):
			flushOL()
			inUL = true
			ulItems = append(ulItems, convertInlineMarkdown(strings.TrimPrefix(line, "- ")))
			continue
		case strings.HasPrefix(line, "* "):
			flushOL()
			inUL = true
			ulItems = append(ulItems, convertInlineMarkdown(strings.TrimPrefix(line, "* ")))
			continue
		default:
			if inUL {
				flushUL()
			}
		}

		switch matches := reOrderedList.FindStringSubmatch(line); {
		case len(matches) > 1:
			flushUL()
			inOL = true
			olItems = append(olItems, convertInlineMarkdown(matches[1]))
			continue
		default:
			if inOL {
				flushOL()
			}
		}

		switch {
		case strings.HasPrefix(line, "# "):
			flushParagraph()
			title := strings.TrimPrefix(line, "# ")
			result = append(result, fmt.Sprintf("          <h1 id=\"%s\">%s</h1>", toAnchor(title), escapeJSX(title)))
		case strings.HasPrefix(line, "## "):
			flushParagraph()
			title := strings.TrimPrefix(line, "## ")
			result = append(result, fmt.Sprintf("          <h2 id=\"%s\">%s</h2>", toAnchor(title), escapeJSX(title)))
		case strings.HasPrefix(line, "### "):
			flushParagraph()
			title := strings.TrimPrefix(line, "### ")
			result = append(result, fmt.Sprintf("          <h3 id=\"%s\">%s</h3>", toAnchor(title), escapeJSX(title)))
		case strings.HasPrefix(line, "#### "):
			flushParagraph()
			title := strings.TrimPrefix(line, "#### ")
			result = append(result, fmt.Sprintf("          <h4 id=\"%s\">%s</h4>", toAnchor(title), escapeJSX(title)))
		case strings.HasPrefix(line, "##### "):
			flushParagraph()
			title := strings.TrimPrefix(line, "##### ")
			result = append(result, fmt.Sprintf("          <h5>%s</h5>", escapeJSX(title)))
		case strings.HasPrefix(line, "> "):
			flushParagraph()
			quote := strings.TrimPrefix(line, "> ")
			result = append(result, fmt.Sprintf("          <blockquote>%s</blockquote>", convertInlineMarkdown(quote)))
		case line == "---":
			flushParagraph()
			result = append(result, "          <hr />")
		case strings.TrimSpace(line) != "":
			paraLines = append(paraLines, strings.TrimSpace(line))
		default:
			flushParagraph()
		}
	}

	flushParagraph()
	flushUL()
	flushOL()

	if inTable {
		result = append(result, convertTable(tableLines))
	}

	return strings.Join(result, "\n")
}

func convertTable(lines []string) string {
	if len(lines) < 2 {
		return ""
	}

	var b strings.Builder
	b.WriteString("          <table className=\"flags-table\">\n")

	headerCells := parseTableRow(lines[0])
	b.WriteString("            <thead>\n              <tr>\n")
	for _, cell := range headerCells {
		fmt.Fprintf(&b, "                <th>%s</th>\n", convertInlineMarkdown(cell))
	}
	b.WriteString("              </tr>\n            </thead>\n")

	b.WriteString("            <tbody>\n")
	for i := 2; i < len(lines); i++ {
		cells := parseTableRow(lines[i])
		b.WriteString("              <tr>\n")
		for _, cell := range cells {
			fmt.Fprintf(&b, "                <td>%s</td>\n", convertInlineMarkdown(cell))
		}
		b.WriteString("              </tr>\n")
	}
	b.WriteString("            </tbody>\n")
	b.WriteString("          </table>")

	return b.String()
}

func parseTableRow(line string) []string {
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	var cells []string
	for _, p := range parts {
		cells = append(cells, strings.TrimSpace(p))
	}
	return cells
}

func convertInlineMarkdown(text string) string {
	reImage := regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
	text = reImage.ReplaceAllStringFunc(text, func(match string) string {
		parts := reImage.FindStringSubmatch(match)
		if len(parts) == 3 {
			alt := escapeJSXContent(parts[1])
			return fmt.Sprintf(`<img src="%s" alt="%s" />`, parts[2], alt)
		}
		return match
	})

	reLink := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	text = reLink.ReplaceAllStringFunc(text, func(match string) string {
		parts := reLink.FindStringSubmatch(match)
		if len(parts) == 3 {
			linkText := escapeJSXContent(parts[1])
			return fmt.Sprintf(`<a href="%s">%s</a>`, parts[2], linkText)
		}
		return match
	})

	reCode := regexp.MustCompile("`([^`]+)`")
	text = reCode.ReplaceAllStringFunc(text, func(match string) string {
		parts := reCode.FindStringSubmatch(match)
		if len(parts) == 2 {
			codeText := escapeJSXContent(parts[1])
			return fmt.Sprintf(`<code>%s</code>`, codeText)
		}
		return match
	})

	reBold := regexp.MustCompile(`\*\*([^*]+)\*\*`)
	text = reBold.ReplaceAllStringFunc(text, func(match string) string {
		parts := reBold.FindStringSubmatch(match)
		if len(parts) == 2 {
			return fmt.Sprintf(`<strong>%s</strong>`, escapeJSXContent(parts[1]))
		}
		return match
	})

	reItalic := regexp.MustCompile(`(?:^|[^*])\*([^*]+)\*(?:[^*]|$)`)
	text = reItalic.ReplaceAllStringFunc(text, func(match string) string {
		parts := reItalic.FindStringSubmatch(match)
		if len(parts) == 2 {
			prefix := ""
			suffix := ""
			if len(match) > 0 && match[0] != '*' {
				prefix = string(match[0])
			}
			if len(match) > 0 && match[len(match)-1] != '*' {
				suffix = string(match[len(match)-1])
			}
			return fmt.Sprintf(`%s<em>%s</em>%s`, prefix, escapeJSXContent(parts[1]), suffix)
		}
		return match
	})

	reUnderscoreItalic := regexp.MustCompile(`(?:^|[^_\w])_([^_]+)_(?:[^_\w]|$)`)
	text = reUnderscoreItalic.ReplaceAllStringFunc(text, func(match string) string {
		parts := reUnderscoreItalic.FindStringSubmatch(match)
		if len(parts) == 2 {
			prefix := ""
			suffix := ""
			if len(match) > 0 && match[0] != '_' {
				prefix = string(match[0])
			}
			if len(match) > 0 && match[len(match)-1] != '_' {
				suffix = string(match[len(match)-1])
			}
			return fmt.Sprintf(`%s<em>%s</em>%s`, prefix, escapeJSXContent(parts[1]), suffix)
		}
		return match
	})

	remaining := regexp.MustCompile(`([^<>&{}]+)`)
	text = remaining.ReplaceAllStringFunc(text, func(match string) string {
		if strings.Contains(match, "<") || strings.Contains(match, ">") ||
			strings.Contains(match, "{") || strings.Contains(match, "}") {
			return escapeJSXContent(match)
		}
		return match
	})

	text = escapeRemainingJSX(text)

	return text
}

func escapeJSXContent(s string) string {
	s = strings.ReplaceAll(s, "{", "&#123;")
	s = strings.ReplaceAll(s, "}", "&#125;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func escapeRemainingJSX(text string) string {
	var result strings.Builder
	inTag := false

	for i := 0; i < len(text); i++ {
		c := text[i]

		switch {
		case c == '<' && i+1 < len(text) && (text[i+1] == '/' || (text[i+1] >= 'a' && text[i+1] <= 'z') || (text[i+1] >= 'A' && text[i+1] <= 'Z')):
			inTag = true
			result.WriteByte(c)
		case c == '<':
			result.WriteString("&lt;")
		case c == '>' && inTag:
			inTag = false
			result.WriteByte(c)
		case c == '>':
			result.WriteString("&gt;")
		case c == '{' && !inTag:
			result.WriteString("&#123;")
		case c == '}' && !inTag:
			result.WriteString("&#125;")
		default:
			result.WriteByte(c)
		}
	}

	return result.String()
}

func escapeJSX(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "{", "&#123;")
	s = strings.ReplaceAll(s, "}", "&#125;")
	return s
}

func escapeForTemplateLiteral(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, "${", "\\${")
	return s
}

func toAnchor(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "'", "")
	s = strings.ReplaceAll(s, "\"", "")
	s = strings.ReplaceAll(s, "(", "")
	s = strings.ReplaceAll(s, ")", "")
	s = strings.ReplaceAll(s, "&", "")
	s = strings.ReplaceAll(s, "/", "")
	re := regexp.MustCompile(`-+`)
	s = re.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}
