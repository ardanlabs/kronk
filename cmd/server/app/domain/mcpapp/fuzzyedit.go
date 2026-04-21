package mcpapp

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FuzzyEditInput defines the input parameters for the fuzzy_edit tool.
type FuzzyEditInput struct {
	FilePath  string `json:"file_path" jsonschema:"Absolute path to the file to edit"`
	OldString string `json:"old_string" jsonschema:"The text to find in the file (fuzzy whitespace matching is applied)"`
	NewString string `json:"new_string" jsonschema:"The replacement text"`
}

func (a *App) fuzzyEdit(ctx context.Context, req *mcp.CallToolRequest, input FuzzyEditInput) (*mcp.CallToolResult, any, error) {
	a.log.Info(ctx, "fuzzy_edit", "status", "request", "file", input.FilePath)

	if input.FilePath == "" {
		return errorResult("file_path is required"), nil, nil
	}

	if input.OldString == "" {
		return errorResult("old_string is required"), nil, nil
	}

	content, err := os.ReadFile(input.FilePath)
	if err != nil {
		a.log.Error(ctx, "fuzzy_edit", "status", "read error", "err", err)
		return errorResult(fmt.Sprintf("failed to read file: %v", err)), nil, nil
	}

	fileStr := string(content)

	replaced, ok := tieredReplace(fileStr, input.OldString, input.NewString)
	if !ok {
		a.log.Info(ctx, "fuzzy_edit", "status", "no match", "file", input.FilePath)
		return errorResult("old_string not found in file (even with fuzzy matching)"), nil, nil
	}

	if err := os.WriteFile(input.FilePath, []byte(replaced), 0644); err != nil {
		a.log.Error(ctx, "fuzzy_edit", "status", "write error", "err", err)
		return errorResult(fmt.Sprintf("failed to write file: %v", err)), nil, nil
	}

	a.log.Info(ctx, "fuzzy_edit", "status", "completed", "file", input.FilePath)

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("Successfully edited %s", input.FilePath)},
		},
	}, nil, nil
}

// =============================================================================
// Tiered Replace
// =============================================================================

// tieredReplace attempts to replace oldStr in content using progressively
// fuzzier matching strategies. It returns the modified content and true if
// exactly one match was found and replaced.
func tieredReplace(content, oldStr, newStr string) (string, bool) {

	// Tier 1: Exact byte match.
	if result, ok := exactReplace(content, oldStr, newStr); ok {
		return result, true
	}

	// Tier 2: Normalize line endings then exact match.
	if result, ok := lineEndingReplace(content, oldStr, newStr); ok {
		return result, true
	}

	// Tier 3: Indentation-insensitive (ignore leading whitespace per line).
	if result, ok := indentInsensitiveReplace(content, oldStr, newStr); ok {
		return result, true
	}

	return "", false
}

// exactReplace performs a standard strings.Replace, succeeding only when
// there is exactly one occurrence.
func exactReplace(content, oldStr, newStr string) (string, bool) {
	count := strings.Count(content, oldStr)
	if count != 1 {
		return "", false
	}

	return strings.Replace(content, oldStr, newStr, 1), true
}

// lineEndingReplace normalizes \r\n → \n in both content and oldStr, then
// attempts an exact match. If successful, it applies the replacement to the
// original content preserving its original line endings.
func lineEndingReplace(content, oldStr, newStr string) (string, bool) {
	normContent := strings.ReplaceAll(content, "\r\n", "\n")
	normOld := strings.ReplaceAll(oldStr, "\r\n", "\n")

	count := strings.Count(normContent, normOld)
	if count != 1 {
		return "", false
	}

	// Find where the match starts in normalized content, then map back to
	// the original content by replacing in the normalized version and
	// restoring original line endings if the file used \r\n.
	result := strings.Replace(normContent, normOld, newStr, 1)

	// If the original had \r\n, the caller's file was CRLF. Preserve that.
	if strings.Contains(content, "\r\n") && !strings.Contains(newStr, "\r\n") {
		result = strings.ReplaceAll(result, "\n", "\r\n")
	}

	return result, true
}

// indentInsensitiveReplace strips leading whitespace from each line when
// comparing, but preserves the file's original indentation in non-matched
// regions. When a match is found, the replacement text is inserted as-is.
func indentInsensitiveReplace(content, oldStr, newStr string) (string, bool) {
	contentLines := strings.Split(content, "\n")
	oldLines := strings.Split(oldStr, "\n")

	// Remove trailing empty line from oldLines if it exists (artifact of
	// trailing newline in the search string).
	if len(oldLines) > 0 && strings.TrimSpace(oldLines[len(oldLines)-1]) == "" {
		oldLines = oldLines[:len(oldLines)-1]
	}

	if len(oldLines) == 0 {
		return "", false
	}

	// Build trimmed versions of old lines for comparison.
	trimmedOld := make([]string, len(oldLines))
	for i, line := range oldLines {
		trimmedOld[i] = strings.TrimSpace(line)
	}

	// Slide over content lines looking for a match.
	var matchIndices []int
	for i := 0; i <= len(contentLines)-len(oldLines); i++ {
		if matchesAt(contentLines, i, trimmedOld) {
			matchIndices = append(matchIndices, i)
		}
	}

	// Require exactly one match.
	if len(matchIndices) != 1 {
		return "", false
	}

	idx := matchIndices[0]

	// Build the result: lines before match + newStr + lines after match.
	var b strings.Builder

	for _, line := range contentLines[:idx] {
		b.WriteString(line)
		b.WriteByte('\n')
	}

	b.WriteString(newStr)

	// Ensure newStr ends with a newline before appending remaining lines.
	if !strings.HasSuffix(newStr, "\n") {
		b.WriteByte('\n')
	}

	remaining := contentLines[idx+len(oldLines):]
	for i, line := range remaining {
		b.WriteString(line)
		if i < len(remaining)-1 {
			b.WriteByte('\n')
		}
	}

	return b.String(), true
}

// matchesAt reports whether trimmedOld matches contentLines starting at
// position start, comparing with leading/trailing whitespace stripped.
func matchesAt(contentLines []string, start int, trimmedOld []string) bool {
	for j, trimmed := range trimmedOld {
		if strings.TrimSpace(contentLines[start+j]) != trimmed {
			return false
		}
	}

	return true
}

// =============================================================================

func errorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: fmt.Sprintf("error: %s", msg)},
		},
		IsError: true,
	}
}
