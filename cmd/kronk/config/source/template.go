package source

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/ardanlabs/kronk/sdk/tools/defaults"
)

// The thinking and reasoning questions are asked only when the chat template
// the server would use reads enable_thinking or reasoning_effort. Which
// template that is follows the template question's answer, so it is resolved
// for each answer, the way the server resolves it (findTemplateFile),
// and kept per file.

// kronkVars are the template variables Kronk or the request fill in. The
// custom variables question lists the template's other ones.
var kronkVars = map[string]bool{
	"messages": true, "tools": true, "tool_choice": true, "documents": true,
	"add_generation_prompt": true, "bos_token": true, "eos_token": true,
	"enable_thinking": true, "reasoning_effort": true,
}

// ChatTemplate is what the questions need to know about a chat template.
type ChatTemplate struct {
	// known is false when the template can't be read here: a template file
	// missing on this machine, or a GGUF without a template. Every variable
	// counts as read then, so the questions are still asked.
	known bool
	vars  []string
}

// Reads reports whether the template reads the variable name, or may.
func (t ChatTemplate) Reads(name string) bool {
	return !t.known || slices.Contains(t.vars, name)
}

// Custom lists the variables the template reads that Kronk and the request
// don't fill in, such as preserve_thinking.
func (t ChatTemplate) Custom() []string {
	return slices.DeleteFunc(slices.Clone(t.vars), func(v string) bool { return kronkVars[v] })
}

// templates resolves the chat template of one model for a template answer.
type templates struct {
	modelID  string // the model file name, which the jinja directory is searched by
	basePath string
	local    bool   // the config is for this machine, whose jinja directory applies
	gguf     string // the template in the GGUF, empty when it has none
	cache    map[string]ChatTemplate
}

// NewTemplates returns the template resolver for the model c. local is
// set when the config is for this machine.
func NewTemplates(c Choice, basePath string, local bool, gguf string) *templates {
	return &templates{
		modelID:  path.Base(c.ID), // the server searches by the file name, without the owner
		basePath: basePath,
		local:    local,
		gguf:     gguf,
		cache:    map[string]ChatTemplate{},
	}
}

// forFile returns the template the server uses when the entry's template:
// is file, or has none when file is empty.
func (t *templates) forFile(file string) ChatTemplate {
	if ct, ok := t.cache[file]; ok {
		return ct
	}

	ct := t.resolve(file)
	t.cache[file] = ct
	return ct
}

func (t *templates) resolve(file string) ChatTemplate {
	// Another machine's jinja directory is not this one: only a template
	// file that can be read here is looked at.
	if file != "" || t.local {
		script, found, err := findTemplateFile(file, t.modelID, t.basePath)
		switch {
		case err != nil:
			return ChatTemplate{}
		case found:
			return ChatTemplate{known: true, vars: templateVariables(script)}
		}
	}

	if t.gguf == "" {
		return ChatTemplate{}
	}
	return ChatTemplate{known: true, vars: templateVariables(t.gguf)}
}

// findTemplateFile finds the chat template file the way a model load does
// before it falls back to the GGUF's template (retrieveTemplate in
// sdk/kronk/model/model.go): jinjaFile, the entry's template:, when set;
// otherwise <jinjaDir>/<modelID>.jinja, then the model id without its quant
// suffix. found is false when no file applies. An unreadable or empty
// jinjaFile is an error, as it is when the model loads.
//
// The SDK only resolves templates for a loaded model, so this repeats its
// steps.
func findTemplateFile(jinjaFile string, modelID string, basePath string) (script string, found bool, err error) {
	if jinjaFile != "" {
		data, err := os.ReadFile(jinjaFile)
		switch {
		case err != nil:
			return "", false, err
		case len(data) == 0:
			return "", false, errors.New("jinja template is empty")
		}
		return string(data), true, nil
	}

	if modelID == "" {
		return "", false, nil
	}

	names := []string{modelID}
	if stripped := quantSuffix.ReplaceAllString(modelID, ""); stripped != modelID && stripped != "" {
		names = append(names, stripped)
	}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(defaults.JinjaDir(basePath), name+".jinja"))
		if err == nil && len(data) > 0 {
			return string(data), true, nil
		}
	}
	return "", false, nil
}

// quantSuffix is a trailing quantization tag such as .Q8_0 or -UD-Q4_K_XL.
// It mirrors quantSuffixPattern in sdk/kronk/model/models.go, which is not
// exported; keep the two in step.
var quantSuffix = regexp.MustCompile(`(?i)[.\-](UD-)?(Q\d+(_[0-9A-Z]+)*|IQ\d+(_[0-9A-Z]+)*|bf16|f16|f32)$`)
