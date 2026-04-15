// Package jinja implements a Jinja2-compatible template engine.
package jinja

import "fmt"

// segKind identifies the type of a scanned template segment.
type segKind int

const (
	segText    segKind = iota // Raw text between tags.
	segComment                // {# ... #}
	segExpr                   // {{ ... }}
	segStmt                   // {% ... %}
)

// segment represents a single scanned piece of a template.
type segment struct {
	kind      segKind
	text      string // Content between delimiters (tags) or raw text.
	trimLeft  bool   // {%- / {{- / {#- was used.
	trimRight bool   // -%} / -}} / -#} was used.
	line      int    // 1-based line number where the segment starts.
	col       int    // 1-based column number where the segment starts.
}

// scan splits source into a flat slice of segments representing the raw text,
// comments, expressions, and statements that make up a Jinja template.
func scan(source string) ([]segment, error) {
	var segs []segment

	// Position tracking.
	line := 1
	col := 1
	i := 0
	textStart := 0
	textLine := 1
	textCol := 1

	for i < len(source) {
		// Look for a two-character tag opener starting with '{'.
		if source[i] != '{' || i+1 >= len(source) {
			if source[i] == '\n' {
				line++
				col = 1
			} else {
				col++
			}
			i++
			continue
		}

		next := source[i+1]
		var kind segKind
		var closer string

		switch next {
		case '{':
			kind = segExpr
			closer = "}}"
		case '%':
			kind = segStmt
			closer = "%}"
		case '#':
			kind = segComment
			closer = "#}"
		default:
			if source[i] == '\n' {
				line++
				col = 1
			} else {
				col++
			}
			i++
			continue
		}

		// Emit any accumulated text before this tag.
		if i > textStart {
			segs = append(segs, segment{
				kind: segText,
				text: source[textStart:i],
				line: textLine,
				col:  textCol,
			})
		}

		tagLine := line
		tagCol := col

		// Skip past the two-character opener.
		i += 2
		col += 2

		// Check for the trim flag immediately after the opener.
		trimLeft := false
		if i < len(source) && source[i] == '-' {
			trimLeft = true
			i++
			col++
		}

		contentStart := i

		// Scan forward for the matching closer, respecting quoted strings.
		trimRight := false
		found := false

		for i < len(source) {
			ch := source[i]

			// Handle quoted strings so that closers inside them are ignored.
			if ch == '\'' || ch == '"' {
				quote := ch
				if ch == '\n' {
					line++
					col = 1
				} else {
					col++
				}
				i++
				for i < len(source) && source[i] != quote {
					if source[i] == '\\' && i+1 < len(source) {
						// Skip escaped character.
						if source[i+1] == '\n' {
							line++
							col = 1
						} else {
							col++
						}
						i++
					}
					if source[i] == '\n' {
						line++
						col = 1
					} else {
						col++
					}
					i++
				}
				if i < len(source) {
					// Skip closing quote.
					col++
					i++
				}
				continue
			}

			// Check for trim-right flag followed by the closer.
			if ch == '-' && i+2 < len(source) && source[i+1] == closer[0] && source[i+2] == closer[1] {
				trimRight = true
				contentEnd := i
				i += 3
				col += 3

				segs = append(segs, segment{
					kind:      kind,
					text:      source[contentStart:contentEnd],
					trimLeft:  trimLeft,
					trimRight: trimRight,
					line:      tagLine,
					col:       tagCol,
				})
				found = true
				break
			}

			// Check for the two-character closer.
			if i+1 < len(source) && ch == closer[0] && source[i+1] == closer[1] {
				contentEnd := i
				i += 2
				col += 2

				segs = append(segs, segment{
					kind:      kind,
					text:      source[contentStart:contentEnd],
					trimLeft:  trimLeft,
					trimRight: trimRight,
					line:      tagLine,
					col:       tagCol,
				})
				found = true
				break
			}

			if ch == '\n' {
				line++
				col = 1
			} else {
				col++
			}
			i++
		}

		if !found {
			tagKind := "expression"
			switch kind {
			case segStmt:
				tagKind = "statement"
			case segComment:
				tagKind = "comment"
			}
			return nil, fmt.Errorf("unclosed %s tag at line %d, col %d", tagKind, tagLine, tagCol)
		}

		// Next text segment starts here.
		textStart = i
		textLine = line
		textCol = col
	}

	// Emit any trailing text.
	if textStart < len(source) {
		segs = append(segs, segment{
			kind: segText,
			text: source[textStart:],
			line: textLine,
			col:  textCol,
		})
	}

	return segs, nil
}
