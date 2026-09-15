package prepare

import (
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxTargetRunes = 800

var codeLineNumberPattern = regexp.MustCompile(`(?:^|[[:space:]])([0-9]{1,3})[[:space:]]`)
var leadingCodeLineNumberPattern = regexp.MustCompile(`^[0-9]{1,3}[[:space:]]+`)
var codeAssignmentPattern = regexp.MustCompile(`\b(?:(?:const|var)[[:space:]]+)?[[:alpha:]_][[:alnum:]_]*[[:space:]]*(?::=|=[[:space:]]+)[[:space:]]*`)
var labeledMetricPattern = regexp.MustCompile(`(?i)\b(?:average|fastest|slowest|requests/sec|total):[[:space:]]*[0-9]+(?:\.[0-9]+)?(?:[[:space:]]*[[:alpha:]/]+)?`)
var percentagePattern = regexp.MustCompile(`(?i)\b[0-9]+(?:\.[0-9]+)?(?:%|[[:space:]]+percent\b)`)
var booleanValuePattern = regexp.MustCompile(`(?i)\b(?:bool|boolean)[[:space:]]+(true|false)\b`)
var stringLiteralPattern = regexp.MustCompile(`"(?:\\.|[^"\\])*"`)
var presentationAnnotationPattern = regexp.MustCompile(`[[:space:]]*<--[^\r\n]*$`)
var proseListPrefixPattern = regexp.MustCompile(`^(?:(?:[-*+•◦▪▫‣⁃·\x{F000}-\x{F8FF}])[[:space:]]+|(?:\([0-9]{1,3}\)|[0-9]{1,3}[.)])[[:space:]]+)+`)
var functionDeclarationPattern = regexp.MustCompile(`\bfunc[[:space:]]+(?:\([^)]*\)[[:space:]]*)?([[:alpha:]_][[:alnum:]_]*)[[:space:]]*\(`)
var typeDeclarationPattern = regexp.MustCompile(`\btype[[:space:]]+([[:alpha:]_][[:alnum:]_]*)\b`)
var packageDeclarationPattern = regexp.MustCompile(`\bpackage[[:space:]]+([[:alpha:]_][[:alnum:]_]*)\b`)
var leadingSourceLinePattern = regexp.MustCompile(`^[0-9]{1,3}[[:space:]]+(?:const|var|func|package|type)[[:space:]]`)
var commandFlagPattern = regexp.MustCompile(`--[[:alnum:]][[:alnum:]-]*`)
var settingPattern = regexp.MustCompile(`\b[[:alpha:]_][[:alnum:]_]*=[^[:space:],;]+`)
var signalNamePattern = regexp.MustCompile(`\bSIG[A-Z0-9]+\b`)
var numericAnswerPattern = regexp.MustCompile(`(?i)(?:~?[0-9]+(?:\.[0-9]+)?[-[:space:]]*(?:bytes?|milliseconds?|seconds?|ms|kb|mb|meg|megabytes?|times?)\b|\bwhich is[[:space:]]+(~?[0-9]+(?:\.[0-9]+)?))`)
var namedAnswerPattern = regexp.MustCompile(`(?i)\b(?:(?:tool|variable|function|method|type|package) named|output is|need(?: then)? is an?)[[:space:]]+([[:alpha:]_][[:alnum:]_./-]*)\b`)
var leadingOutputPattern = regexp.MustCompile(`^([[:alpha:]_][[:alnum:]_.-]*)[[:space:]]+is the output\b`)
var inaccessibleNamePattern = regexp.MustCompile(`(?i)\bbut[[:space:]]+([[:alpha:]_][[:alnum:]_]*)[[:space:]]+is not\b`)
var fieldReferencePattern = regexp.MustCompile(`(?i)\b([[:alpha:]_][[:alnum:]_]*)[[:space:]]+field\b`)

func quoteCandidates(passage string) []string {
	var quotes []string
	seen := make(map[string]struct{})
	appendQuote := func(quote string) {
		quote = cleanQuote(quote)
		length := utf8.RuneCountInString(quote)
		if length < 1 || length > maxTargetRunes || isIncompleteCodeCandidate(quote) {
			return
		}
		if _, duplicate := seen[quote]; duplicate {
			return
		}
		seen[quote] = struct{}{}
		quotes = append(quotes, quote)
	}

	for index, section := range strings.Split(passage, "```") {
		if index%2 == 1 {
			for _, quote := range codeQuoteCandidates(section) {
				appendQuote(quote)
			}
			continue
		}
		for _, block := range proseBlocks(section) {
			if cells, ok := tableQuoteCandidates(block); ok {
				for _, quote := range cells {
					appendQuote(quote)
				}
				continue
			}
			for _, quote := range proseQuoteCandidates(block) {
				appendQuote(quote)
				for _, proposition := range prosePropositionCandidates(quote) {
					appendQuote(proposition)
				}
				for _, atomic := range proseAtomicCandidates(quote) {
					appendQuote(atomic)
				}
			}
		}
	}

	return quotes
}

func isIncompleteCodeCandidate(candidate string) bool {
	return strings.HasSuffix(strings.TrimSpace(candidate), "{")
}

func proseBlocks(section string) []string {
	lines := strings.Split(section, "\n")
	blocks := make([]string, 0, len(lines))
	var prose strings.Builder
	var table strings.Builder
	flushProse := func() {
		if block := strings.TrimSpace(prose.String()); block != "" {
			blocks = append(blocks, block)
		}
		prose.Reset()
	}
	flushTable := func() {
		if block := strings.TrimSpace(table.String()); block != "" {
			blocks = append(blocks, block)
		}
		table.Reset()
	}
	for _, line := range lines {
		line = strings.TrimSpace(normalizeSourceText(line))
		if line == "" {
			flushProse()
			flushTable()
			continue
		}
		listItem := proseListPrefixPattern.MatchString(line)
		line = cleanProse(line)
		if line == "" {
			continue
		}
		if strings.Count(line, "|") >= 2 {
			flushProse()
			if table.Len() > 0 {
				table.WriteByte('\n')
			}
			table.WriteString(line)
			continue
		}
		flushTable()
		if listItem {
			flushProse()
		}
		if prose.Len() > 0 {
			prose.WriteByte(' ')
		}
		prose.WriteString(line)
	}
	flushProse()
	flushTable()
	return blocks
}

func tableQuoteCandidates(table string) ([]string, bool) {
	if strings.Count(table, "|") < 2 {
		return nil, false
	}
	var rows [][]string
	for line := range strings.SplitSeq(table, "\n") {
		var cells []string
		separator := true
		for cell := range strings.SplitSeq(strings.TrimSpace(line), "|") {
			cell = cleanProse(cell)
			if cell == "" {
				continue
			}
			if !isTableSeparator(cell) {
				separator = false
			}
			cells = append(cells, cell)
		}
		if !separator && len(cells) > 0 {
			rows = append(rows, cells)
		}
	}
	if len(rows) == 0 {
		return nil, true
	}

	var quotes []string
	headings := rows[0]
	quotes = append(quotes, headings...)
	for _, row := range rows[1:] {
		quotes = append(quotes, row...)
		if len(row) == len(headings) {
			fields := make([]string, len(row))
			for index := range row {
				fields[index] = headings[index] + ": " + row[index]
			}
			quotes = append(quotes, strings.Join(fields, "; "))
		} else {
			quotes = append(quotes, strings.Join(row, " — "))
		}
	}
	return quotes, true
}

func isTableSeparator(cell string) bool {
	cell = strings.TrimSpace(cell)
	if cell == "" {
		return true
	}
	for _, char := range cell {
		if char != '-' && char != ':' && !unicode.IsSpace(char) {
			return false
		}
	}
	return true
}

func proseQuoteCandidates(passage string) []string {
	runes := []rune(passage)
	quotes := make([]string, 0, 4)
	start := 0

	appendQuote := func(end int) {
		quote := cleanProse(string(runes[start:end]))
		start = end
		if strings.HasSuffix(quote, ",") || strings.HasSuffix(quote, ":") || strings.HasSuffix(quote, ";") {
			return
		}
		if !hasBalancedOpeningQuotation(quote) {
			return
		}
		length := utf8.RuneCountInString(quote)
		if length >= 1 && length <= maxTargetRunes {
			quotes = append(quotes, quote)
		}
	}

	insideStraightQuote := false
	insideCurlyDoubleQuote := false
	insideCurlySingleQuote := false
	for index, char := range runes {
		switch char {
		case '"':
			insideStraightQuote = !insideStraightQuote
		case '“':
			insideCurlyDoubleQuote = true
		case '”':
			insideCurlyDoubleQuote = false
		case '‘':
			insideCurlySingleQuote = true
		case '’':
			insideCurlySingleQuote = false
		}
		if char != '.' && char != '?' && char != '!' {
			continue
		}
		if insideStraightQuote || insideCurlyDoubleQuote || insideCurlySingleQuote {
			continue
		}
		next := index + 1
		if (next == len(runes) || unicode.IsSpace(runes[next])) && !continuesAfterPeriod(runes, index) {
			appendQuote(next)
		}
	}
	if start < len(runes) {
		appendQuote(len(runes))
	}

	return quotes
}

func prosePropositionCandidates(sentence string) []string {
	parts := strings.Split(sentence, ";")
	if len(parts) < 2 {
		return nil
	}
	candidates := make([]string, 0, len(parts))
	for _, part := range parts {
		part = cleanProse(part)
		if len(strings.Fields(part)) >= 4 && hasBalancedOpeningQuotation(part) {
			candidates = append(candidates, part)
		}
	}
	return candidates
}

func hasBalancedOpeningQuotation(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return true
	}

	switch []rune(value)[0] {
	case '"':
		return strings.Count(value, `"`)%2 == 0
	case '“':
		return strings.Count(value, "“") == strings.Count(value, "”")
	case '‘':
		return strings.Count(value, "‘") == strings.Count(value, "’")
	}
	return true
}

func codeQuoteCandidates(code string) []string {
	var quotes []string
	for line := range strings.SplitSeq(strings.TrimSpace(code), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, quote := range splitNumberedCodeLine(line) {
			quote = strings.TrimSpace(leadingCodeLineNumberPattern.ReplaceAllString(quote, ""))
			quote = strings.TrimSpace(presentationAnnotationPattern.ReplaceAllString(quote, ""))
			if quote == "" || onlyCodePunctuation(quote) {
				continue
			}
			quotes = append(quotes, quote)
			quotes = append(quotes, codeAtomicCandidates(quote)...)
			quotes = append(quotes, textAtomicCandidates(quote)...)
		}
	}
	return quotes
}

func textAtomicCandidates(text string) []string {
	candidates := percentagePattern.FindAllString(text, -1)
	for _, match := range booleanValuePattern.FindAllStringSubmatch(text, -1) {
		candidates = append(candidates, match[1])
	}
	return candidates
}

func proseAtomicCandidates(text string) []string {
	candidates := textAtomicCandidates(text)
	candidates = append(candidates, commandFlagPattern.FindAllString(text, -1)...)
	candidates = append(candidates, settingPattern.FindAllString(text, -1)...)
	candidates = append(candidates, signalNamePattern.FindAllString(text, -1)...)
	for _, match := range numericAnswerPattern.FindAllStringSubmatch(text, -1) {
		candidate := strings.TrimSpace(match[0])
		if match[1] != "" {
			candidate = match[1]
		}
		candidates = append(candidates, candidate)
	}
	for _, pattern := range []*regexp.Regexp{namedAnswerPattern, leadingOutputPattern, inaccessibleNamePattern} {
		for _, match := range pattern.FindAllStringSubmatch(text, -1) {
			candidates = append(candidates, match[1])
		}
	}
	return candidates
}

func codeAtomicCandidates(line string) []string {
	candidates := stringLiteralPattern.FindAllString(line, -1)
	for _, match := range functionDeclarationPattern.FindAllStringSubmatch(line, -1) {
		candidates = append(candidates, match[1])
	}
	for _, match := range typeDeclarationPattern.FindAllStringSubmatch(line, -1) {
		candidates = append(candidates, match[1])
	}
	for _, match := range packageDeclarationPattern.FindAllStringSubmatch(line, -1) {
		candidates = append(candidates, match[1])
	}
	matches := codeAssignmentPattern.FindAllStringIndex(line, -1)
	for index, match := range matches {
		end := len(line)
		if index+1 < len(matches) {
			end = matches[index+1][0]
		}
		if candidate := trimCollapsedCodeStatement(line[match[0]:end]); candidate != "" {
			candidates = append(candidates, candidate)
		}
		if candidate := trimAssignedValue(line[match[1]:end]); candidate != "" {
			candidates = append(candidates, candidate)
		}
	}

	for offset := 0; offset < len(line); {
		command := strings.Index(line[offset:], "$ ")
		if command < 0 {
			break
		}
		start := offset + command
		end := len(line)
		for _, marker := range []string{" $ ", " Sample output:", " Output:"} {
			if position := strings.Index(line[start+2:], marker); position >= 0 && start+2+position < end {
				end = start + 2 + position
			}
		}
		if candidate := strings.TrimSpace(line[start:end]); candidate != "$" {
			candidates = append(candidates, candidate)
		}
		offset = end
	}

	for _, candidate := range labeledMetricPattern.FindAllString(line, -1) {
		candidates = append(candidates, strings.TrimSpace(candidate))
	}
	return candidates
}

func trimCollapsedCodeStatement(statement string) string {
	end := len(statement)
	for _, marker := range []string{" . . .", " defer ", " if ", " for ", " switch ", " return ", " go func", " func ", " Sample output:", " Output:"} {
		if position := strings.Index(statement, marker); position >= 0 && position < end {
			end = position
		}
	}
	return strings.TrimSpace(strings.TrimRight(statement[:end], ";{}"))
}

func trimAssignedValue(value string) string {
	if comment := strings.Index(value, " //"); comment >= 0 {
		value = value[:comment]
	}
	return strings.TrimSpace(strings.TrimRight(value, ",;{}"))
}

func onlyCodePunctuation(value string) bool {
	for _, char := range value {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			return false
		}
	}
	return true
}

func splitNumberedCodeLine(line string) []string {
	matches := codeLineNumberPattern.FindAllStringSubmatchIndex(line, -1)
	if len(matches) < 3 || matches[0][2] != 0 {
		return []string{line}
	}

	starts := make([]int, 0, len(matches))
	previous := -1
	for _, match := range matches {
		number, err := strconv.Atoi(line[match[2]:match[3]])
		if err != nil {
			continue
		}
		if previous >= 0 && (number < previous || number > previous+2) {
			continue
		}
		starts = append(starts, match[2])
		previous = number
	}
	if len(starts) < 3 {
		return []string{line}
	}

	quotes := make([]string, 0, len(starts))
	for index, start := range starts {
		end := len(line)
		if index+1 < len(starts) {
			end = starts[index+1]
		}
		quotes = append(quotes, strings.TrimSpace(line[start:end]))
	}
	return quotes
}

func continuesAfterPeriod(runes []rune, period int) bool {
	if runes[period] != '.' {
		return false
	}
	if period > 0 && period+1 < len(runes) && unicode.IsDigit(runes[period-1]) && unicode.IsDigit(runes[period+1]) {
		return true
	}

	start := period
	for start > 0 && unicode.IsLetter(runes[start-1]) {
		start--
	}
	word := strings.ToLower(string(runes[start:period]))
	if utf8.RuneCountInString(word) == 1 {
		return true
	}

	switch word {
	case "mr", "mrs", "ms", "dr", "prof", "st", "mt", "sen", "rep", "gov", "gen", "lt", "col", "sgt", "capt", "rev", "fr":
		return true
	}

	return false
}

func cleanQuote(quote string) string {
	quote = strings.TrimSpace(normalizeSourceText(quote))
	for strings.HasPrefix(quote, "#") {
		quote = strings.TrimSpace(strings.TrimPrefix(quote, "#"))
	}
	quote = strings.TrimPrefix(quote, "* ")
	quote = strings.TrimPrefix(quote, "- ")
	return strings.TrimSpace(quote)
}

func cleanProse(value string) string {
	value = cleanQuote(value)
	value = proseListPrefixPattern.ReplaceAllString(value, "")
	return strings.Join(strings.Fields(value), " ")
}

func normalizeSourceText(value string) string {
	return html.UnescapeString(value)
}
