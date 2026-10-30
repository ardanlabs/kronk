package tui

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/ardanlabs/kronk/cmd/kronk/config/questions"
	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
	kronkserver "github.com/ardanlabs/kronk/cmd/server/api/services/kronk"
)

func (a *app) newPickForm() *huh.Form {
	choices := a.pickChoices()
	rows := make([]pickRow, len(choices))
	var w pickWidths
	for i, c := range choices {
		rows[i] = newPickRow(c)
		w.id = max(w.id, len(rows[i].idText))
		w.source = max(w.source, len(rows[i].sourceText))
		w.kind = max(w.kind, len(rows[i].kindText))
		w.size = max(w.size, len(rows[i].sizeText))
	}

	opts := make([]huh.Option[string], 0, len(choices))
	for i, c := range choices {
		opts = append(opts, huh.NewOption(a.pickLabel(rows[i], w), c.ID))
	}

	// The question is in pickHeading. huh shows the search text where the
	// title goes, so the title is only seen when the search is closed. The
	// blank description puts an empty line under the search.
	sel := huh.NewSelect[string]().
		Title("Press / to search").
		Description(" ").
		Options(opts...).
		Height(16).
		Value(&a.picked)
	sel.WithTheme(a.searchTheme())

	// Open the search the way the / key does. Filtering(true) would turn it
	// on without the esc binding that closes it, so esc would leave the step.
	form := a.newForm(huh.NewGroup(sel))
	form.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	return form
}

// searchTheme is the form theme for the picker's list: a space after the
// search's / prompt, and its blank description is one line, without the
// usual blank line after it. A select keeps the first theme it gets, so the
// form's theme doesn't replace it.
func (a *app) searchTheme() huh.Theme {
	return huh.ThemeFunc(func(bool) *huh.Styles {
		st := *a.colors.form()
		st.Focused.TextInput.Prompt = st.Focused.TextInput.Prompt.PaddingRight(1)
		st.Focused.Description = st.Focused.Description.MarginBottom(0)
		st.Blurred.Description = st.Focused.Description
		return &st
	})
}

// clearSearch closes the picker's search and clears it with one esc. huh
// would keep the text as a filter after the first esc and clear it on a
// second one, which shows it as "/text" until then.
func (a *app) clearSearch(esc tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m, cmd := a.updateForm(esc)
	if a.step == stepPick && a.fieldUsesEsc() {
		_, clear := a.updateForm(esc)
		return a, tea.Batch(cmd, clear)
	}
	return m, cmd
}

// pickHeading is the picker's question. It is drawn above the form: while
// the list is searched, huh shows the search text where a field's title goes.
// It has the focused field's border, so it reads as part of the list.
func (a *app) pickHeading() string {
	st := a.theme.Theme(false)
	hint := "Type to search. Installed models first, then the catalog."
	return st.Focused.Base.Render(a.groupLine() + "\n" +
		st.Focused.Title.Render(a.pickQuestion()) + "\n" +
		st.Focused.Description.Render(hint))
}

// backKeyMap shows esc as the "back" key in the form help. Esc is handled by
// the app (see back); the binding itself stays on shift+tab, which still
// works but is not listed.
func backKeyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.Input.Prev.SetHelp("esc", "back")
	km.Select.Prev.SetHelp("esc", "back")
	km.Text.Prev.SetHelp("esc", "back")
	km.MultiSelect.Prev.SetHelp("esc", "back")
	km.MultiSelect.Toggle.SetHelp("space", "toggle")

	// The picker's search: esc closes and clears it at once (clearSearch).
	km.Select.Filter.SetHelp("/", "search")
	km.Select.SetFilter.SetHelp("esc", "clear search")
	return km
}

// newForm builds a form with the app's theme and esc listed as back in its
// key help. It gets the window size right away: without it the form asks for
// the size in Init and pads its groups only when the answer comes, so the
// first frame jumps.
func (a *app) newForm(groups ...*huh.Group) *huh.Form {
	f := huh.NewForm(groups...).WithTheme(a.theme).WithKeyMap(backKeyMap()).WithShowHelp(true)
	if a.width > 0 {
		f.Update(a.formSize())
	}
	return f
}

// formSize is the window size message the forms get: narrower when the
// answers column is on screen.
func (a *app) formSize() tea.WindowSizeMsg {
	return tea.WindowSizeMsg{Width: min(a.formWidth(), maxFormWidth), Height: a.height}
}

// maxFormWidth keeps the forms' text at a length that reads well on wide
// terminals.
const maxFormWidth = 100

// pickRow is one picker line split into columns.
type pickRow struct {
	idText, sourceText, kindText, sizeText string
	notes                                  []string
	notValidated                           bool
}

// pickWidths is the widest value of each picker column.
type pickWidths struct {
	id, source, kind, size int
}

func newPickRow(c source.Choice) pickRow {
	r := pickRow{idText: c.ID, sourceText: "catalog", kindText: string(c.Group()), sizeText: "-"}
	if c.Installed {
		r.sourceText = "installed"
		r.notValidated = !c.Validated
	}
	if c.Size > 0 {
		r.sizeText = source.FormatBytes(uint64(c.Size))
	}
	if c.InFile {
		r.notes = append(r.notes, "in config")
	}

	return r
}

// pickLabel pads each column before styling it, so ANSI codes don't break
// the alignment.
func (a *app) pickLabel(r pickRow, w pickWidths) string {
	label := fmt.Sprintf("%-*s  %s", w.id, r.idText, a.st.muted.Render(fmt.Sprintf("%-*s", w.source, r.sourceText)))

	// The kind of model only tells something in All models: a group's list
	// has one kind, named in the heading.
	if a.group == groupAll {
		label += "  " + a.st.muted.Render(fmt.Sprintf("%-*s", w.kind, r.kindText))
	}
	label += "  " + a.st.muted.Render(fmt.Sprintf("%*s", w.size, r.sizeText))
	if len(r.notes) > 0 {
		label += "  " + a.st.muted.Render(strings.Join(r.notes, " · "))
	}
	if r.notValidated {
		label += "  " + a.st.warn.Render("not validated")
	}

	return label
}

func (a *app) newQuestionsForm() *huh.Form {
	f, ans := a.facts, a.ans

	// a.qs is already in the order the questions are asked.
	var hg []*huh.Group
	for _, q := range a.qs {
		q := q.For(f)
		open := func() bool { return ans.Applies(f, q) }
		hg = append(hg, huh.NewGroup(withAnswersKey(a.field(q))).WithHideFunc(func() bool { return !open() }))

		if q.HasOther() {
			otherDesc := "Writes " + a.keyPath(q.Key)
			if n := a.facts.Analysis.Model.BlockCount; q.Key == "ngpu-layers" && n > 0 {
				otherDesc = fmt.Sprintf("The model has %d layers. ", n) + otherDesc
			}
			other := huh.NewInput().
				Key(q.Key + "/other").
				Title(upperFirst(q.Title)).
				Description(otherDesc).
				Placeholder("a value").
				Validate(q.Validate).
				Value(ans.Other[q.Key])

			hg = append(hg, huh.NewGroup(withAnswersKey(other)).WithHideFunc(func() bool {
				return !open() || *ans.Sel[q.Key] != questions.OtherValue
			}))
		}
	}

	return a.newForm(hg...)
}

// field builds the form field for one question.
func (a *app) field(q questions.Question) huh.Field {
	desc := a.describe(q)

	if q.Kind == questions.KindMulti {
		var opts []huh.Option[string]
		for _, o := range q.Options(a.facts) {
			opts = append(opts, huh.NewOption(o.Label, o.Value))
		}
		return huh.NewMultiSelect[string]().
			Key(q.Key).
			Title(upperFirst(q.Title)).
			Description(desc).
			Options(opts...).
			Value(a.ans.Multi[q.Key])
	}

	if q.Kind == questions.KindVars || q.Kind == questions.KindAdapters {
		return huh.NewText().
			Key(q.Key).
			Title(upperFirst(q.Title)).
			DescriptionFunc(func() string { return a.describe(q) }, a.ans.Sel["template"]).
			Placeholder(upperFirst(q.Def(a.facts)) + " (default)").
			Lines(4).
			CharLimit(4000).
			Validate(q.Validate).
			Value(a.ans.Sel[q.Key])
	}

	if q.Options == nil {
		// A secret's hint says whether the file has one, not a default. A
		// required input has no default.
		placeholder := q.Def(a.facts)
		if !q.Secret && !q.Required {
			placeholder += " (default)"
		}
		validate := q.Validate
		if q.Required {
			validate = func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("enter a value")
				}
				return q.Validate(s)
			}
		}

		in := huh.NewInput().
			Key(q.Key).
			Title(upperFirst(q.Title)).
			Description(desc).
			Placeholder(upperFirst(placeholder)).
			Validate(validate).
			Value(a.ans.Sel[q.Key])
		if q.Secret || q.Kind == questions.KindPassword {
			in = in.EchoMode(huh.EchoModePassword)
		}
		return in
	}

	// The first option keeps the key out of the file. When the default is
	// one of the options, that option's label names it.
	options := q.Options(a.facts)
	def := questions.DefaultLabel(q, a.facts)

	var opts []huh.Option[string]
	if !q.Required {
		opts = append(opts, huh.NewOption(upperFirst(def)+" (default)", ""))
	}
	for _, o := range options {
		if o.Value == q.Same {
			continue
		}
		opts = append(opts, huh.NewOption(upperFirst(o.Label), o.Value))
	}
	if q.HasOther() {
		label := "Other value…"
		if q.OtherLabel != "" {
			label = q.OtherLabel
		}
		opts = append(opts, huh.NewOption(label, questions.OtherValue))
	}

	return huh.NewSelect[string]().
		Key(q.Key).
		Title(upperFirst(q.Title)).
		Description(desc).
		Options(opts...).
		Value(a.ans.Sel[q.Key])
}

// upperFirst capitalizes the first letter of a title, label or hint. It is
// display only; the values written to the file keep their case.
func upperFirst(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}

func (a *app) describe(q questions.Question) string {
	// What the setting does, then what is known about this model.
	var about []string
	if q.About != "" {
		about = append(about, upperFirst(q.About))
	}
	if note := questions.ContextNote(a.facts); q.Key == "context-window" && note != "" {
		about = append(about, note)
	}
	if n := a.facts.Analysis.Model.BlockCount; q.Key == "moe.keep-experts-top-n" && n > 0 {
		about = append(about, fmt.Sprintf("The model has %d layers.", n))
	}
	if q.Kind == questions.KindAdapters && len(a.facts.Adapters) > 0 {
		about = append(about, "In the lora folder: "+strings.Join(a.facts.Adapters, ", "))
	}
	if v := a.facts.Template(a.ans.Value("template")).Custom(); q.Kind == questions.KindVars && len(v) > 0 {
		about = append(about, "The chat template also reads: "+strings.Join(v, ", "))
	}

	// Blocks are set apart by a blank line: about, tip, the keys written.
	var blocks []string
	if len(about) > 0 {
		blocks = append(blocks, a.st.value.Render(strings.Join(about, "\n")))
	}
	if q.Tip != "" {
		blocks = append(blocks, a.st.ok.Bold(true).Render("Tip:")+" "+a.st.value.Render(q.Tip))
	}

	// The default is shown once: in a list's "(default)" option, or
	// as an input's placeholder.
	keys := make([]string, 0, len(q.Keys()))
	for _, k := range q.Keys() {
		keys = append(keys, a.keyPath(k))
	}
	if q.Key == questions.KeyRopeScaling {
		keys = []string{"context-window, or rope-scaling-type, rope-freq-scale and yarn-orig-ctx"}
	}
	blocks = append(blocks, a.st.muted.Render("Writes:")+" "+a.st.value.Render(strings.Join(keys, ", ")))

	return strings.Join(blocks, "\n\n")
}

// keyPath is the key as it appears in the file: server settings sit under
// kms:, model settings under the model's entry.
func (a *app) keyPath(key string) string {
	if a.server {
		return "kms." + key
	}
	return key
}

func (a *app) newSectionsForm() *huh.Form {
	settings := kronkserver.Settings()

	var opts []huh.Option[string]
	for _, sec := range questions.ServerSections(settings) {
		var n, set int
		for _, st := range settings {
			if questions.ServerSection(st.Key) != sec {
				continue
			}
			n++
			if _, ok := a.existing[st.Key]; ok {
				set++
			}
		}

		noun := "settings"
		if n == 1 {
			noun = "setting"
		}
		label := questions.SectionLabel(sec) + a.st.muted.Render(fmt.Sprintf("  %d %s", n, noun))
		if set > 0 {
			label += a.st.muted.Render(fmt.Sprintf(" · %d in the file", set))
		}
		opts = append(opts, huh.NewOption(label, sec))
	}

	sel := huh.NewMultiSelect[string]().
		Title("Which server settings do you want to change?").
		Description("Space selects, enter continues. Sections with values in the file start selected.").
		Options(opts...).
		Value(&a.sections)

	return a.newForm(huh.NewGroup(sel))
}

func (a *app) newReviewForm() *huh.Form {
	// When the answers don't fit, the fixes come first, then keep: save
	// anyway.
	var opts []huh.Option[string]
	for i, x := range a.fixes {
		opts = append(opts, huh.NewOption(x.Label(), actFix+strconv.Itoa(i)))
	}

	if len(a.problems) > 0 {
		sel := huh.NewSelect[string]().
			Key(keyAction).
			Title("Fix the answers above before saving.").
			Description("Kronk would refuse them when it loads the model.").
			Options(huh.NewOption("Change one answer…", actOne),
				huh.NewOption("Quit without saving", actQuit)).
			Value(&a.action)
		return a.newForm(huh.NewGroup(withAnswersKey(sel)))
	}

	save := "Save to " + shortPath(a.path)
	title, desc := "Save this entry?", "Or change answers before saving."
	if a.server {
		title = "Save these settings?"
	}
	if len(a.fixes) > 0 || (a.check != nil && !a.check.Recommended.Fits) {
		save = "Keep it and save to " + shortPath(a.path)
		title, desc = "It doesn't fit in memory.", "Pick a change that fits, or keep the answers and save."
	}

	opts = append(opts,
		huh.NewOption(save, actSave),
		huh.NewOption("Save to another file…", actOther),
		huh.NewOption("Change one answer…", actOne),
		huh.NewOption("Quit without saving", actQuit),
	)

	sel := huh.NewSelect[string]().
		Key(keyAction).
		Title(title).
		Description(desc).
		Options(opts...).
		Value(&a.action)

	path := huh.NewInput().
		Title("File path").
		Description("For another machine: copy it there, or point KRONK_POOL_MODEL_CONFIG_FILE at it.").
		Placeholder("./model_config.yaml").
		Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("enter a path")
			}
			return nil
		}).
		Value(&a.otherPath)

	return a.newForm(
		huh.NewGroup(withAnswersKey(sel)),
		huh.NewGroup(path).WithHideFunc(func() bool { return a.action != actOther }),
	)
}
