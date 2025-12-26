package api

import (
	"os/exec"
	"regexp"
	"strings"
)

type paramField struct {
	Name        string
	JSONName    string
	Type        string
	Description string
}

func parseParams() ([]paramField, error) {
	cmd := exec.Command("go", "doc", "github.com/ardanlabs/kronk/sdk/kronk/model.Params")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return parseParamsOutput(string(output)), nil
}

func parseParamsOutput(output string) []paramField {
	var fields []paramField

	lines := strings.Split(output, "\n")

	reField := regexp.MustCompile(`^\s+(\w+)\s+(\S+)\s+` + "`" + `json:"([^"]+)"` + "`")

	var structLines []string
	inStruct := false
	for _, line := range lines {
		if strings.Contains(line, "type Params struct") {
			inStruct = true
			continue
		}
		if inStruct {
			if strings.HasPrefix(line, "}") {
				break
			}
			structLines = append(structLines, line)
		}
	}

	for _, line := range structLines {
		matches := reField.FindStringSubmatch(line)
		if len(matches) == 4 {
			fields = append(fields, paramField{
				Name:     matches[1],
				Type:     matches[2],
				JSONName: matches[3],
			})
		}
	}

	docText := extractDocText(output)

	for i := range fields {
		fields[i].Description = extractFieldDescription(fields[i].Name, docText)
	}

	return fields
}

func extractDocText(output string) string {
	lines := strings.Split(output, "\n")

	var docLines []string
	inDoc := false
	for _, line := range lines {
		if strings.HasPrefix(line, "}") {
			inDoc = true
			continue
		}
		if inDoc {
			docLines = append(docLines, line)
		}
	}

	return strings.Join(docLines, "\n")
}

func extractFieldDescription(fieldName string, docText string) string {
	fieldDescriptions := map[string]string{
		"Temperature":     "Controls randomness of output by rescaling probability distribution",
		"TopK":            "Limits token pool to K most probable tokens",
		"TopP":            "Nucleus sampling - selects tokens whose cumulative probability exceeds threshold",
		"MinP":            "Dynamic sampling threshold balancing coherence and diversity",
		"MaxTokens":       "Maximum output tokens to generate",
		"Thinking":        "Enable model thinking/reasoning for non-GPT models",
		"ReasoningEffort": "Reasoning level for GPT models: none, minimal, low, medium, high",
	}

	patterns := map[string]*regexp.Regexp{
		"Temperature":     regexp.MustCompile(`Temperature[^.]+\.[^.]*default[^.]*(\d+\.?\d*)`),
		"TopK":            regexp.MustCompile(`Top-?K[^.]+\.[^.]*default[^.]*(\d+)`),
		"TopP":            regexp.MustCompile(`Top-?P[^.]+\.[^.]*default[^.]*(\d+\.?\d*)`),
		"MinP":            regexp.MustCompile(`Min-?P[^.]+\.[^.]*default[^.]*(\d+\.?\d*)`),
		"MaxTokens":       regexp.MustCompile(`MaxTokens[^.]+\.[^.]*default[^.]*(\d+)`),
		"Thinking":        regexp.MustCompile(`EnableThinking[^.]+\.[^.]*default[^.]*"([^"]+)"`),
		"ReasoningEffort": nil,
	}

	desc := fieldDescriptions[fieldName]

	if pattern, ok := patterns[fieldName]; ok && pattern != nil {
		matches := pattern.FindStringSubmatch(docText)
		if len(matches) > 1 {
			desc += " (default: " + matches[1] + ")"
		}
	}

	return desc
}

func paramsToFields() []field {
	params, err := parseParams()
	if err != nil {
		return defaultParamFields()
	}

	var fields []field
	for _, p := range params {
		fields = append(fields, field{
			Name:        p.JSONName,
			Type:        p.Type,
			Required:    false,
			Description: p.Description,
		})
	}

	return fields
}

func defaultParamFields() []field {
	return []field{
		{Name: "temperature", Type: "float32", Required: false, Description: "Controls randomness of output (default: 0.8)"},
		{Name: "top_k", Type: "int32", Required: false, Description: "Limits token pool to K most probable tokens (default: 40)"},
		{Name: "top_p", Type: "float32", Required: false, Description: "Nucleus sampling threshold (default: 0.9)"},
		{Name: "min_p", Type: "float32", Required: false, Description: "Dynamic sampling threshold (default: 0.0)"},
		{Name: "max_tokens", Type: "int", Required: false, Description: "Maximum output tokens (default: 1024)"},
		{Name: "enable_thinking", Type: "boolean", Required: false, Description: "Enable model thinking for non-GPT models (default: true)"},
		{Name: "reasoning_effort", Type: "string", Required: false, Description: "Reasoning level for GPT models (default: medium)"},
	}
}
