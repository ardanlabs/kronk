package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	kronkserver "github.com/ardanlabs/kronk/cmd/server/api/services/kronk"
	"go.yaml.in/yaml/v3"
)

// Server settings are the kms: section of the config file. The questions are
// built from kronkserver.Settings, which reads the server's config struct, so
// a new server setting is asked without changes here.

// keyAdminPassword is the one setting asked differently: the user types the
// password and the file gets its SHA-256, which is what the server compares.
const keyAdminPassword = "web.admin.password-sha-256"

// sections groups the server settings by their first key part. Settings at
// the top level of kms: are in "runtime".
var sections = []struct {
	name, label string
}{
	{"web", "Web server: hosts, timeouts, CORS, admin UI"},
	{"auth", "External auth service"},
	{"authorization", "Authorization: which endpoints need a token"},
	{"mcp", "MCP service"},
	{"download", "Model downloads through the server"},
	{"tempo", "Tracing (Tempo)"},
	{"pool", "Model pool: memory budget, models kept loaded, idle unload"},
	{"runtime", "Runtime: paths, libraries, processor, tokens, logging"},
}

// skipServer are kms keys the tool doesn't ask. base-path: the server finds
// this file through the base path before it reads kms:, so a value here can't
// move it and only confuses.
var skipServer = map[string]bool{"base-path": true}

func sectionOf(key string) string {
	if first, _, ok := strings.Cut(key, "."); ok {
		return first
	}
	return "runtime"
}

func sectionLabel(name string) string {
	for _, s := range sections {
		if s.name == name {
			return s.label
		}
	}
	return name
}

// serverSections lists the sections that have settings, in sections order,
// then any section added to the server and not listed above.
func serverSections(settings []kronkserver.Setting) []string {
	var out []string
	for _, s := range sections {
		if slices.ContainsFunc(settings, func(st kronkserver.Setting) bool { return sectionOf(st.Key) == s.name }) {
			out = append(out, s.name)
		}
	}
	for _, st := range settings {
		if sec := sectionOf(st.Key); !slices.Contains(out, sec) {
			out = append(out, sec)
		}
	}
	return out
}

// serverQuestions builds one question per server setting. A question is
// asked when its section is in picked. existing is the flattened kms: block,
// used to tell whether a secret is already set.
func serverQuestions(settings []kronkserver.Setting, picked *[]string, existing map[string]any) []question {
	var qs []question
	for _, sec := range serverSections(settings) {
		for _, st := range settings {
			if sectionOf(st.Key) != sec || skipServer[st.Key] {
				continue
			}
			qs = append(qs, serverQuestion(st, sec, picked, existing))
		}
	}
	return qs
}

func serverQuestion(st kronkserver.Setting, sec string, picked *[]string, existing map[string]any) question {
	// The first sentence of the help text is the question, the rest its
	// description.
	title, about, _ := strings.Cut(st.Help, ". ")

	q := question{
		key:   st.Key,
		group: sec,
		title: strings.TrimSuffix(title, ".") + "?",
		about: about,
		show:  func(facts, *answers) bool { return slices.Contains(*picked, sec) },
	}

	def := st.Default
	switch {
	case def == "":
		def = "not set"
	case st.Kind == kronkserver.KindBool && def == "true":
		def = "yes"
	case st.Kind == kronkserver.KindBool:
		def = "no"
	}

	switch st.Kind {
	case kronkserver.KindBool:
		q.kind = kindBool
		q.options = fixed(option{"Yes", "true"}, option{"No", "false"})
		q.same = st.Default
	case kronkserver.KindChoice:
		q.kind = kindChoice
		var opts []option
		for _, v := range st.Values {
			opts = append(opts, option{v, v})
		}
		q.options = fixed(opts...)
		q.same = st.Default
	case kronkserver.KindInt:
		q.kind = kindInt
		q.min = -1 << 31
	case kronkserver.KindFloat:
		q.kind = kindFloat
		q.min = -1 << 31
	case kronkserver.KindDuration:
		q.kind = kindDuration
	case kronkserver.KindList:
		q.kind = kindList
		q.about = strings.TrimSpace(q.about + " Comma separated.")
	default:
		q.kind = kindText
	}

	if st.Secret {
		q.secret = true
		def = "not set"
		if _, ok := existing[st.Key]; ok {
			def = "set in the file, empty keeps it"
		}
	}
	if st.Key == keyAdminPassword {
		q.kind = kindPassword
		q.title = "Browser admin login password?"
		q.about = "The file stores its SHA-256, not the password."
	}

	q.def = func(facts) string { return def }

	return q
}

// hashPassword is what the server compares the admin login against.
func hashPassword(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// =============================================================================

// readKMS returns the flattened kms: section of the file, empty when the file
// or the section doesn't exist.
func readKMS(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}

	return readKMSData(data)
}

func readKMSData(data []byte) (map[string]any, error) {
	var doc struct {
		KMS map[string]any `yaml:"kms"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	out := map[string]any{}
	flatten("", doc.KMS, out)

	return out, nil
}
