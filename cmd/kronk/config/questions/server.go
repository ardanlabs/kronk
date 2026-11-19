package questions

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
	kronkserver "github.com/ardanlabs/kronk/cmd/server/api/services/kronk"
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

// ServerSection is the section of a server setting key.
func ServerSection(key string) string {
	if first, _, ok := strings.Cut(key, "."); ok {
		return first
	}
	return "runtime"
}

// SectionLabel describes a server settings section.
func SectionLabel(name string) string {
	for _, s := range sections {
		if s.name == name {
			return s.label
		}
	}
	return name
}

// ServerSections lists the sections that have settings, in sections order,
// then any section added to the server and not listed above.
func ServerSections(settings []kronkserver.Setting) []string {
	var out []string
	for _, s := range sections {
		if slices.ContainsFunc(settings, func(st kronkserver.Setting) bool { return ServerSection(st.Key) == s.name }) {
			out = append(out, s.name)
		}
	}
	for _, st := range settings {
		if sec := ServerSection(st.Key); !slices.Contains(out, sec) {
			out = append(out, sec)
		}
	}
	return out
}

// Server builds one question per server setting. A question is
// asked when its section is in picked. existing is the flattened kms: block,
// used to tell whether a secret is already set.
func Server(settings []kronkserver.Setting, picked *[]string, existing map[string]any) []Question {
	var qs []Question
	for _, sec := range ServerSections(settings) {
		for _, st := range settings {
			if ServerSection(st.Key) != sec || skipServer[st.Key] {
				continue
			}
			qs = append(qs, serverQuestion(st, sec, picked, existing))
		}
	}
	return qs
}

func serverQuestion(st kronkserver.Setting, sec string, picked *[]string, existing map[string]any) Question {
	// The first sentence of the help text is the question, the rest its
	// description.
	title, about, _ := strings.Cut(st.Help, ". ")

	q := Question{
		Key:   st.Key,
		Group: sec,
		Title: strings.TrimSuffix(title, ".") + "?",
		About: about,
		show:  func(source.Facts, *Answers) bool { return slices.Contains(*picked, sec) },
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
		q.Kind = kindBool
		q.Options = yesNo
		q.Same = st.Default
	case kronkserver.KindChoice:
		q.Kind = kindChoice
		var opts []option
		for _, v := range st.Values {
			opts = append(opts, option{v, v})
		}
		q.Options = fixed(opts...)
		q.Same = st.Default
	case kronkserver.KindInt:
		q.Kind = kindInt
		q.min = -1 << 31
	case kronkserver.KindFloat:
		q.Kind = kindFloat
		q.min = -1 << 31
	case kronkserver.KindDuration:
		q.Kind = kindDuration
	case kronkserver.KindList:
		q.Kind = kindList
		q.About = strings.TrimSpace(q.About + " Comma separated.")
	default:
		q.Kind = kindText
	}

	if st.Secret {
		q.Secret = true
		def = "not set"
		if _, ok := existing[st.Key]; ok {
			def = "set in the file, empty keeps it"
		}
	}
	if st.Key == keyAdminPassword {
		q.Kind = KindPassword
		q.Title = "Browser admin login password?"
		q.About = "The file stores its SHA-256, not the password."
	}

	q.Def = func(source.Facts) string { return def }

	return q
}

// hashPassword is what the server compares the admin login against.
func hashPassword(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
