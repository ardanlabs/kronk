package questions

import (
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
)

// Answers holds the form values. A select stores its value in sel; when the
// user picks "Other value…" the typed value is in other.
type Answers struct {
	ByKey map[string]Question
	Sel   map[string]*string
	Other map[string]*string
	Multi map[string]*[]string // kindMulti answers
}

// NewAnswers returns empty answers for the questions qs.
func NewAnswers(qs []Question) *Answers {
	a := Answers{
		ByKey: make(map[string]Question, len(qs)),
		Sel:   make(map[string]*string, len(qs)),
		Other: make(map[string]*string, len(qs)),
		Multi: make(map[string]*[]string),
	}

	for _, q := range qs {
		a.ByKey[q.Key] = q
		a.Sel[q.Key] = new(string)
		a.Other[q.Key] = new(string)
		a.Multi[q.Key] = new([]string)
	}

	return &a
}

// Value is the final answer for key. Empty means keep Kronk's default.
func (a *Answers) Value(key string) string {
	sel, ok := a.Sel[key]
	if !ok {
		return ""
	}
	if a.ByKey[key].Kind == KindMulti {
		return strings.Join(*a.Multi[key], ",")
	}
	if *sel == OtherValue {
		return strings.TrimSpace(*a.Other[key])
	}
	return strings.TrimSpace(*sel)
}

// Prefill sets the answers from an existing entry, so editing starts from
// what is in the file.
func (a *Answers) Prefill(f source.Facts, existing map[string]any) {
	for key, q := range a.ByKey {
		if q.Kind == KindVars {
			*a.Sel[key] = formatVars(existingVars(existing))
			continue
		}

		v, ok := existing[key]
		if !ok || q.Secret {
			continue
		}
		if q.sameAs != "" && sameValue(existing[q.sameAs], v) {
			continue
		}

		switch {
		case q.Kind == KindAdapters:
			*a.Sel[key] = formatAdapters(v)
		case q.Kind == KindMulti:
			*a.Multi[key] = anyStrings(v)
		case q.Kind == kindFloatList:
			*a.Sel[key] = strings.Join(anyStrings(v), ", ")
		case q.isSame(v):
		default:
			s := fmt.Sprint(v)
			if _, ok := v.([]any); ok {
				s = strings.Join(anyStrings(v), ",")
			}
			a.set(f, key, s)
		}
	}
}

// set answers key with value: the matching option when there is one,
// otherwise "Other value…" with the value typed in.
func (a *Answers) set(f source.Facts, key string, value string) {
	q := a.ByKey[key]
	if _, ok := q.Option(f, value); ok || q.Options == nil {
		*a.Sel[key] = value
		return
	}
	*a.Sel[key] = OtherValue
	*a.Other[key] = value
}

// multiValues is the answer to a kindMulti question, nil when key is not one
// of the questions.
func (a *Answers) multiValues(key string) []string {
	if p, ok := a.Multi[key]; ok {
		return *p
	}
	return nil
}

// Applies reports whether q is asked for this model and these answers.
func (a *Answers) Applies(f source.Facts, q Question) bool {
	if q.generation && !f.Generates() {
		return false
	}
	return q.show(f, a)
}
