// Package questions holds what the config tool asks and what the answers
// write: the chat, audio, image and server settings questions, the answers,
// the changes they make to an entry, and the checks run before saving
// (problems the SDK refuses, the memory check's fixes, YaRN).
package questions

import (
	"cmp"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
)

// DefaultLabel names the default of q: the label of the option that behaves
// like leaving the key out, or the default text.
func DefaultLabel(q Question, f source.Facts) string {
	if q.Same != "" {
		if o, ok := q.Option(f, q.Same); ok {
			return o.Label
		}
	}
	return q.Def(f)
}

// kind is the type of value a question writes.
type kind int

const (
	kindChoice kind = iota
	kindBool
	kindInt
	kindFloat
	kindDuration
	kindText
	kindList      // comma separated, written as a YAML list
	KindPassword  // typed in clear, written as its SHA-256
	KindVars      // name=value lines, written as a map; see vars.go
	KindMulti     // several options at once, written as a list
	kindFloatList // comma separated numbers, written as a list
	KindAdapters  // adapter lines, written as a list of maps; see adapters.go
)

// OtherValue is the select value that opens a free-form input.
const OtherValue = "\x00other"

type option struct {
	Label string
	Value string
}

func fixed(opts ...option) func(source.Facts) []option {
	return func(source.Facts) []option { return opts }
}

// yesNo are the options of a yes or no question, yes first.
var yesNo = fixed(option{"Yes", "true"}, option{"No", "false"})

// gpuOptions lists the machine's GPUs by device name.
func gpuOptions(f source.Facts) []option {
	var opts []option
	for _, name := range f.GPUNames() {
		opts = append(opts, option{name, name})
	}
	return opts
}

// text is a default that doesn't depend on the model or the machine.
func text(s string) func(source.Facts) string {
	return func(source.Facts) string { return s }
}

// Question is one setting in the model entry. key is the YAML path inside
// the entry (dot separated). TestQuestionKeys checks every key and value
// against models.ModelConfig, so a renamed setting fails the build's tests.
type Question struct {
	Key     string
	also    []string
	Group   string
	Title   string
	About   string // what the setting does
	Tip     string // when to change it, shown as a recommendation
	Kind    kind
	Options func(source.Facts) []option
	min     float64
	max     float64
	Def     func(source.Facts) string
	show    func(source.Facts, *Answers) bool

	// Same is the option value that behaves like leaving the key out. It is
	// not listed, since the "(default)" option already covers it, and a file
	// that already has it is left as it is.
	Same string

	// Secret is never shown or prefilled. An empty answer keeps the value
	// in the file instead of removing it.
	Secret bool

	// sameAs names the question whose answer this one follows when left
	// empty, e.g. value precision follows key precision. An empty answer
	// leaves the key to that question.
	sameAs string

	// Required lists no "(default)" option: the setting needs a value when
	// the question is asked.
	Required bool

	// OtherLabel replaces "Other value…" for the free-form option.
	OtherLabel string

	// generation marks a setting that only applies when the model
	// generates text (source.Facts.Generates): sampling, thinking, the chat
	// template, speculation and the conversation cache.
	generation bool

	// pooled rewords the question for a model that doesn't generate text,
	// which has inputs, not conversations. Empty parts keep the question's
	// own wording.
	pooled wording
}

// wording is the text of a question.
type wording struct {
	title, about, tip string
}

// For returns q worded for the model f: the pooled wording for a model that
// doesn't generate text, and {inputs} replaced by what the model's
// projector takes, such as "images and audio".
func (q Question) For(f source.Facts) Question {
	if !f.Generates() {
		q.Title = cmp.Or(q.pooled.title, q.Title)
		q.About = cmp.Or(q.pooled.about, q.About)
		q.Tip = cmp.Or(q.pooled.tip, q.Tip)
	}

	inputs := f.InputsText()
	q.Title = strings.ReplaceAll(q.Title, "{inputs}", inputs)
	q.About = strings.ReplaceAll(q.About, "{inputs}", inputs)
	q.Tip = strings.ReplaceAll(q.Tip, "{inputs}", inputs)
	return q
}

func always(source.Facts, *Answers) bool { return true }

// Option returns the listed option of q with value.
func (q Question) Option(f source.Facts, value string) (option, bool) {
	if q.Options == nil {
		return option{}, false
	}
	for _, o := range q.Options(f) {
		if o.Value == value {
			return o, true
		}
	}
	return option{}, false
}

// HasOther reports whether q's list ends with "Other value…", which opens a
// free-form input.
func (q Question) HasOther() bool {
	return q.Options != nil && q.Kind != kindChoice && q.Kind != kindBool && q.Kind != KindMulti
}

// isSame reports whether v is the option value that behaves like leaving the
// key out. Option values are answer text, so v is compared as text.
func (q Question) isSame(v any) bool {
	return q.Same != "" && fmt.Sprint(v) == q.Same
}

// Keys are the keys the question writes.
func (q Question) Keys() []string {
	return append([]string{q.Key}, q.also...)
}

// parse converts an answer to the typed value written to YAML.
func (q Question) parse(s string) (any, error) {
	switch q.Kind {
	case kindBool:
		return strconv.ParseBool(s)

	case kindInt:
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, errors.New("enter a whole number")
		}
		if err := q.inRange(float64(n)); err != nil {
			return nil, err
		}
		return n, nil

	case kindFloat:
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, errors.New("enter a number")
		}
		if err := q.inRange(n); err != nil {
			return nil, err
		}
		return n, nil

	case kindDuration:
		d, err := time.ParseDuration(s)
		if err != nil {
			return nil, errors.New("enter a duration like 30s, 3m or 1h")
		}
		if d < 0 || (d == 0 && q.min > 0) {
			return nil, errors.New("must be greater than zero")
		}
		return s, nil

	case kindList, KindMulti:
		return SplitList(s, ","), nil

	case KindPassword:
		return hashPassword(s), nil

	case KindVars:
		return parseVars(s)

	case kindFloatList:
		var out []float64
		for part := range strings.SplitSeq(s, ",") {
			part = strings.TrimSpace(part)
			n, err := strconv.ParseFloat(part, 64)
			if err != nil || n < 0 {
				return nil, fmt.Errorf("%q: enter numbers of 0 or more, comma separated", part)
			}
			out = append(out, n)
		}
		return out, nil

	case KindAdapters:
		return parseAdapters(s)
	}

	return s, nil
}

func (q Question) inRange(n float64) error {
	if n < q.min {
		return fmt.Errorf("must be %s or more", trimFloat(q.min))
	}
	if q.max > 0 && n > q.max {
		return fmt.Errorf("must be %s or less", trimFloat(q.max))
	}
	return nil
}

// Validate is used by the free-form inputs. Empty keeps the default.
func (q Question) Validate(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	_, err := q.parse(s)
	return err
}

// SplitList splits s at sep, trims the parts and drops the empty ones.
func SplitList(s string, sep string) []string {
	var out []string
	for part := range strings.SplitSeq(s, sep) {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// anyStrings is a YAML list as text, nil when v is not a list.
func anyStrings(v any) []string {
	list, _ := v.([]any)
	if list == nil {
		return nil
	}
	out := make([]string, len(list))
	for i, item := range list {
		out[i] = fmt.Sprint(item)
	}
	return out
}

func trimFloat(n float64) string {
	return strconv.FormatFloat(n, 'f', -1, 64)
}
