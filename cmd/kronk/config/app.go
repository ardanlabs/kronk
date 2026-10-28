package config

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	kronkserver "github.com/ardanlabs/kronk/cmd/server/api/services/kronk"
	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

type step int

const (
	stepLoading step = iota
	stepStart
	stepSections
	stepPick
	stepFacts
	stepQuestions
	stepCheck
	stepReview
	stepSaving
	stepDone
	stepFailed
)

// Review actions.
const (
	actSave  = "save"
	actOther = "other"
	actEdit  = "edit"
	actQuit  = "quit"
)

type startMsg struct {
	serverFile string
	files      []string
	err        error
}

type loadedMsg struct {
	path    string
	doc     modelconfig.Document
	choices []choice
	err     error
}

type factsMsg struct {
	facts facts
	err   error
}

type checkMsg struct {
	analysis models.Analysis
	err      error
}

type savedMsg struct {
	path   string
	backup string
	err    error
}

// app is the root model. Each step hosts one huh form; the app moves to the
// next step when that form completes.
type app struct {
	ctx    context.Context
	src    *source
	st     styles
	theme  huh.Theme
	spin   spinner.Model
	width  int
	height int

	step   step
	status string
	err    error

	serverFile string
	files      []string
	start      *start
	startForm  *huh.Form

	path    string
	doc     modelconfig.Document
	choices []choice

	// server is set when the user configures the server settings (kms:)
	// instead of a model. sections holds the sections to change.
	server       bool
	sections     []string
	sectionsForm *huh.Form

	pickForm *huh.Form
	pickErr  error
	picked   string
	choice   choice

	facts    facts
	qs       []question
	ans      *answers
	existing map[string]any
	form     *huh.Form

	changes  []change
	check    *models.Analysis
	checkErr error

	reviewForm *huh.Form
	action     string
	otherPath  string

	savedPath string
	backup    string
}

func newApp(ctx context.Context, src *source, isDark bool) *app {
	st := newStyles(isDark)

	// The forms never receive tea.BackgroundColorMsg, so huh would always
	// pass false to the theme. Use the background detected at startup instead.
	theme := huh.ThemeFunc(func(bool) *huh.Styles { return formTheme(isDark) })

	return &app{
		ctx:    ctx,
		src:    src,
		st:     st,
		theme:  theme,
		spin:   spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(st.heading)),
		status: "Looking for config files",
	}
}

func (a *app) Init() tea.Cmd {
	return tea.Batch(a.spin.Tick, a.loadStart)
}

// =============================================================================
// Commands

func (a *app) loadStart() tea.Msg {
	path, err := a.src.configFile()
	if err != nil {
		return startMsg{err: err}
	}

	return startMsg{serverFile: path, files: configFiles(path)}
}

func (a *app) load(path string) tea.Cmd {
	return func() tea.Msg {
		doc, err := a.src.loadDoc(path)
		if err != nil {
			return loadedMsg{err: fmt.Errorf("read %s: %w", path, err)}
		}

		choices, err := a.src.choices(doc)
		if err != nil {
			return loadedMsg{err: err}
		}

		return loadedMsg{path: path, doc: doc, choices: choices}
	}
}

func (a *app) loadFacts(c choice) tea.Cmd {
	var other *machine
	if m, ok := a.start.machine(); ok {
		other = &m
	}

	return func() tea.Msg {
		f, err := a.src.facts(a.ctx, c, other)
		return factsMsg{facts: f, err: err}
	}
}

func (a *app) runCheck(cfg models.ModelConfig) tea.Cmd {
	f := a.facts
	return func() tea.Msg {
		an, err := a.src.check(f, cfg)
		return checkMsg{analysis: an, err: err}
	}
}

func (a *app) save(path string) tea.Cmd {
	id, changes, server := a.choice.ID, a.changes, a.server
	return func() tea.Msg {
		var backup string
		var err error
		if server {
			backup, err = saveKMS(path, changes)
		} else {
			backup, err = saveEntry(path, id, changes)
		}
		return savedMsg{path: path, backup: backup, err: err}
	}
}

// =============================================================================
// Update

func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height

	case tea.KeyPressMsg:
		switch a.step {
		case stepDone, stepFailed:
			return a, tea.Quit
		case stepLoading, stepFacts, stepCheck, stepSaving:
			if msg.String() == "ctrl+c" {
				return a, tea.Quit
			}
		case stepStart, stepSections, stepPick, stepQuestions, stepReview:
			if msg.String() == "esc" && !a.fieldUsesEsc() {
				return a.back()
			}
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spin, cmd = a.spin.Update(msg)
		return a, cmd

	case startMsg:
		if msg.err != nil {
			return a.fail(msg.err)
		}
		a.serverFile, a.files = msg.serverFile, msg.files
		a.start = newStart(msg.serverFile)
		a.step = stepStart
		a.startForm = a.newStartForm()
		return a, a.startForm.Init()

	case loadedMsg:
		if msg.err != nil {
			return a.fail(msg.err)
		}
		a.path, a.doc, a.choices = msg.path, msg.doc, msg.choices
		if len(a.choices) == 0 {
			return a.fail(fmt.Errorf("no models installed and the catalog is empty, pull a model first: kronk model pull --local <id>"))
		}
		a.step = stepPick
		a.pickForm = a.newPickForm()
		return a, a.pickForm.Init()

	case factsMsg:
		if msg.err != nil {
			if a.choice.Installed {
				return a.backToPick(msg.err)
			}
			return a.fail(msg.err)
		}
		return a.startQuestions(msg.facts)

	case checkMsg:
		if msg.err != nil {
			a.checkErr = msg.err
		} else {
			a.check = &msg.analysis
		}
		return a.startReview()

	case savedMsg:
		if msg.err != nil {
			return a.fail(msg.err)
		}
		a.step = stepDone
		a.savedPath, a.backup = msg.path, msg.backup
		return a, nil
	}

	return a.updateForm(msg)
}

// updateForm passes the message to the form of the current step and moves
// on when it completes.
func (a *app) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	form := a.currentForm()
	if form == nil {
		return a, nil
	}

	m, cmd := form.Update(msg)
	if f, ok := m.(*huh.Form); ok {
		form = f
	}

	switch form.State {
	case huh.StateAborted:
		return a, tea.Quit

	case huh.StateCompleted:
		switch a.step {
		case stepStart:
			return a.startDone()
		case stepSections:
			return a.sectionsDone()
		case stepPick:
			return a.pickDone()
		case stepQuestions:
			return a.questionsDone()
		case stepReview:
			return a.reviewDone()
		}
	}

	return a, cmd
}

func (a *app) fail(err error) (tea.Model, tea.Cmd) {
	a.step = stepFailed
	a.err = err
	return a, nil
}

// currentForm is the form of the current step, nil when the step has none.
func (a *app) currentForm() *huh.Form {
	switch a.step {
	case stepStart:
		return a.startForm
	case stepSections:
		return a.sectionsForm
	case stepPick:
		return a.pickForm
	case stepQuestions:
		return a.form
	case stepReview:
		return a.reviewForm
	}
	return nil
}

// back moves to the previous question of the current form, skipping the
// ones that are hidden. From the first question it leaves the step, in the
// order start → model list → questions → review, landing on the last
// question of the step it returns to.
func (a *app) back() (tea.Model, tea.Cmd) {
	form := a.currentForm()

	// PrevGroup applies at once and stays put when no earlier group is
	// visible, or when the current one has a validation error.
	before := form.GetFocusedField()
	cmd := form.PrevGroup()
	if form.GetFocusedField() != before || len(form.Errors()) > 0 {
		return a, cmd
	}

	switch a.step {
	case stepPick, stepSections:
		a.step = stepStart
		form, cmd := reopenAt(a.newStartForm, a.startForm, len(a.files)+10)
		a.startForm = form
		return a, cmd

	case stepQuestions:
		if a.server {
			a.step = stepSections
			a.sectionsForm = a.newSectionsForm()
			return a, a.sectionsForm.Init()
		}
		a.step = stepPick
		a.pickForm = a.newPickForm()
		return a, a.pickForm.Init()

	case stepReview:
		a.step = stepQuestions
		form, cmd := reopenAt(a.newQuestionsForm, a.form, len(a.qs)*2)
		a.form = form
		return a, cmd
	}

	return a, nil
}

// fieldUsesEsc reports whether the focused field has esc bound right now,
// such as a list setting or clearing its filter. Esc goes to the field then,
// not back a step.
func (a *app) fieldUsesEsc() bool {
	form := a.currentForm()
	if form == nil {
		return false
	}

	field := form.GetFocusedField()
	if field == nil {
		return false
	}

	for _, b := range field.KeyBinds() {
		if b.Enabled() && slices.Contains(b.Keys(), "esc") {
			return true
		}
	}
	return false
}

// backToPick shows the picker again with err above it. Used when an installed
// model can't be read, so the user can pick another one.
func (a *app) backToPick(err error) (tea.Model, tea.Cmd) {
	if !a.choice.Validated {
		err = fmt.Errorf("%w\nThe model files look incomplete. Pull it again: kronk model pull --local %s", err, a.choice.ID)
	}

	a.pickErr = err
	a.picked = ""
	a.step = stepPick
	a.pickForm = a.newPickForm()
	return a, a.pickForm.Init()
}

func (a *app) startDone() (tea.Model, tea.Cmd) {
	a.path = a.start.path()
	a.server = a.start.server()

	if a.server {
		existing, err := readKMS(a.path)
		if err != nil {
			return a.fail(fmt.Errorf("read %s: %w", a.path, err))
		}

		a.facts = facts{}
		a.existing = existing
		a.sections = nil
		for k := range existing {
			if sec := sectionOf(k); !slices.Contains(a.sections, sec) {
				a.sections = append(a.sections, sec)
			}
		}
		a.qs = serverQuestions(kronkserver.Settings(), &a.sections, existing)
		a.ans = newAnswers(a.qs)
		a.ans.prefill(a.facts, existing)

		a.step = stepSections
		a.sectionsForm = a.newSectionsForm()
		return a, a.sectionsForm.Init()
	}

	a.step = stepLoading
	a.status = "Reading " + shortPath(a.path) + ", models and the catalog"
	return a, tea.Batch(a.spin.Tick, a.load(a.path))
}

func (a *app) sectionsDone() (tea.Model, tea.Cmd) {
	return a.openQuestions()
}

func (a *app) pickDone() (tea.Model, tea.Cmd) {
	a.pickErr = nil

	i := slices.IndexFunc(a.choices, func(c choice) bool { return c.ID == a.picked })
	if i < 0 {
		return a.fail(fmt.Errorf("model %q not found", a.picked))
	}

	a.choice = a.choices[i]
	a.step = stepFacts
	a.status = "Reading " + a.choice.ID
	if !a.choice.Installed {
		a.status += " from the catalog (may download the GGUF header)"
	}

	return a, tea.Batch(a.spin.Tick, a.loadFacts(a.choice))
}

func (a *app) startQuestions(f facts) (tea.Model, tea.Cmd) {
	a.facts = f
	a.qs = questions()
	a.ans = newAnswers(a.qs)
	a.existing = nil

	if mc, ok := a.doc.Models[f.ID]; ok {
		existing, err := flattenConfig(mc)
		if err != nil {
			return a.fail(fmt.Errorf("read existing entry: %w", err))
		}
		a.existing = existing
		a.ans.prefill(f, existing)
	}

	return a.openQuestions()
}

func (a *app) questionsDone() (tea.Model, tea.Cmd) {
	changes, err := diff(a.qs, a.facts, a.ans, a.existing)
	if err != nil {
		return a.fail(err)
	}
	a.changes = changes
	a.check, a.checkErr = nil, nil

	if a.server {
		return a.startReview()
	}

	cfg, err := toModelConfig(apply(a.existing, changes))
	if err != nil {
		return a.fail(err)
	}

	if !a.facts.HardwareKnown {
		return a.startReview()
	}

	a.step = stepCheck
	a.status = "Checking memory on this machine"
	if a.facts.Other != nil {
		a.status = "Checking memory on the target machine"
	}
	return a, tea.Batch(a.spin.Tick, a.runCheck(cfg))
}

func (a *app) startReview() (tea.Model, tea.Cmd) {
	a.step = stepReview
	a.action = actSave
	a.otherPath = ""
	a.reviewForm = a.newReviewForm()
	return a, a.reviewForm.Init()
}

// reopenAt builds a new form and moves it to the question that was on screen
// when old was submitted. A submitted huh form can't be reopened; the new one
// is bound to the same answers.
func reopenAt(build func() *huh.Form, old *huh.Form, limit int) (*huh.Form, tea.Cmd) {
	var target string
	if f := old.GetFocusedField(); f != nil {
		target = f.GetKey()
	}

	form := build()
	return form, initAt(form, target, limit)
}

// initAt moves a new form to the field with key target, then initializes it.
// Moving first matters: when the first group is hidden, Init skips it with a
// message that arrives after the first frame, so that frame would show the
// hidden question, and the late skip would also move past target. A visible
// target is never past the end, so NextGroup doesn't submit before reaching
// it; limit bounds the walk for a key that is not in the form.
func initAt(form *huh.Form, target string, limit int) tea.Cmd {
	var cmds []tea.Cmd
	for range limit {
		f := form.GetFocusedField()
		if f == nil || f.GetKey() == target {
			break
		}
		cmds = append(cmds, form.NextGroup())
	}

	return tea.Batch(append(cmds, form.Init())...)
}

// openQuestions builds the questions form on the first question that is
// asked.
func (a *app) openQuestions() (tea.Model, tea.Cmd) {
	var first string
	for _, q := range a.qs {
		if a.ans.applies(a.facts, q) {
			first = q.key
			break
		}
	}

	a.step = stepQuestions
	a.form = a.newQuestionsForm()
	return a, initAt(a.form, first, len(a.qs)*2)
}

func (a *app) editAnswers() (tea.Model, tea.Cmd) {
	return a.openQuestions()
}

func (a *app) reviewDone() (tea.Model, tea.Cmd) {
	switch a.action {
	case actEdit:
		return a.editAnswers()

	case actQuit:
		return a, tea.Quit

	case actOther:
		a.step = stepSaving
		a.status = "Saving"
		return a, tea.Batch(a.spin.Tick, a.save(expandHome(a.otherPath)))
	}

	a.step = stepSaving
	a.status = "Saving"
	return a, tea.Batch(a.spin.Tick, a.save(a.path))
}

// =============================================================================
// Forms

func (a *app) newPickForm() *huh.Form {
	rows := make([]pickRow, len(a.choices))
	var w pickWidths
	for i, c := range a.choices {
		rows[i] = newPickRow(c)
		w.id = max(w.id, len(rows[i].idText))
		w.source = max(w.source, len(rows[i].sourceText))
		w.kind = max(w.kind, len(rows[i].kindText))
		w.size = max(w.size, len(rows[i].sizeText))
	}

	opts := make([]huh.Option[string], 0, len(a.choices))
	for i, c := range a.choices {
		opts = append(opts, huh.NewOption(a.pickLabel(rows[i], w), c.ID))
	}

	sel := huh.NewSelect[string]().
		Title("Which model do you want to configure?").
		Description("Installed models first, then the catalog. Press / to filter.").
		Options(opts...).
		Height(16).
		Value(&a.picked)

	return a.sized(huh.NewForm(huh.NewGroup(sel)).WithTheme(a.theme).WithShowHelp(true))
}

// backKeyMap shows esc as the "back" key in the form help. Esc is handled by
// the app (see back); the binding itself stays on shift+tab, which still
// works but is not listed.
func backKeyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.Input.Prev.SetHelp("esc", "back")
	km.Select.Prev.SetHelp("esc", "back")
	km.Text.Prev.SetHelp("esc", "back")
	return km
}

// sized gives a new form the window size right away. Without it the form
// asks for the size in Init and pads its groups only when the answer comes,
// so the first frame jumps.
func (a *app) sized(f *huh.Form) *huh.Form {
	if a.width > 0 {
		f.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
	}
	return f
}

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

func newPickRow(c choice) pickRow {
	r := pickRow{idText: c.ID, sourceText: "catalog", kindText: "chat", sizeText: "-"}
	if c.Installed {
		r.sourceText = "installed"
		r.notValidated = !c.Validated
	}
	if c.Endpoint != "" {
		r.kindText = c.Endpoint
	}
	if c.Size > 0 {
		r.sizeText = formatBytes(uint64(c.Size))
	}
	if c.InFile {
		r.notes = append(r.notes, "in config")
	}

	return r
}

// pickLabel pads each column before styling it, so ANSI codes don't break
// the alignment.
func (a *app) pickLabel(r pickRow, w pickWidths) string {
	label := fmt.Sprintf("%-*s  %s  %s  %s",
		w.id, r.idText,
		a.st.muted.Render(fmt.Sprintf("%-*s", w.source, r.sourceText)),
		a.st.muted.Render(fmt.Sprintf("%-*s", w.kind, r.kindText)),
		a.st.muted.Render(fmt.Sprintf("%*s", w.size, r.sizeText)),
	)
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
		open := func() bool { return ans.applies(f, q) }
		hg = append(hg, huh.NewGroup(a.field(q)).WithHideFunc(func() bool { return !open() }))

		if q.options != nil && q.kind != kindChoice && q.kind != kindBool {
			other := huh.NewInput().
				Key(q.key + "/other").
				Title(upperFirst(q.title)).
				Description("Writes " + a.keyPath(q.key)).
				Placeholder("a value").
				Validate(q.validate).
				Value(ans.other[q.key])

			hg = append(hg, huh.NewGroup(other).WithHideFunc(func() bool {
				return !open() || *ans.sel[q.key] != otherValue
			}))
		}
	}

	return a.sized(huh.NewForm(hg...).WithTheme(a.theme).WithKeyMap(backKeyMap()).WithShowHelp(true))
}

// field builds the form field for one question.
func (a *app) field(q question) huh.Field {
	desc := a.describe(q)

	if q.kind == kindVars {
		return huh.NewText().
			Key(q.key).
			Title(upperFirst(q.title)).
			Description(desc).
			Placeholder(upperFirst(q.def(a.facts)) + " (default)").
			Lines(4).
			CharLimit(4000).
			Validate(q.validate).
			Value(a.ans.sel[q.key])
	}

	if q.options == nil {
		// A secret's hint says whether the file has one, not a default.
		placeholder := q.def(a.facts)
		if !q.secret {
			placeholder += " (default)"
		}

		in := huh.NewInput().
			Key(q.key).
			Title(upperFirst(q.title)).
			Description(desc).
			Placeholder(upperFirst(placeholder)).
			Validate(q.validate).
			Value(a.ans.sel[q.key])
		if q.secret || q.kind == kindPassword {
			in = in.EchoMode(huh.EchoModePassword)
		}
		return in
	}

	// The first option keeps the key out of the file. When the default is
	// one of the options, that option's label names it.
	options := q.options(a.facts)
	def := q.def(a.facts)
	for _, o := range options {
		if o.value == q.same {
			def = o.label
		}
	}

	opts := []huh.Option[string]{huh.NewOption(upperFirst(def)+" (default)", "")}
	for _, o := range options {
		if o.value == q.same {
			continue
		}
		opts = append(opts, huh.NewOption(upperFirst(o.label), o.value))
	}
	if q.kind != kindChoice && q.kind != kindBool {
		opts = append(opts, huh.NewOption("Other value…", otherValue))
	}

	return huh.NewSelect[string]().
		Key(q.key).
		Title(upperFirst(q.title)).
		Description(desc).
		Options(opts...).
		Value(a.ans.sel[q.key])
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

func (a *app) describe(q question) string {
	var lines []string
	if q.about != "" {
		lines = append(lines, upperFirst(q.about))
	}
	if q.kind == kindVars && len(a.facts.TemplateVars) > 0 {
		lines = append(lines, "This model's template checks: "+strings.Join(a.facts.TemplateVars, ", "))
	}
	// The default is shown once: in a list's "(default)" option, or
	// as an input's placeholder.
	keys := make([]string, 0, len(q.keys()))
	for _, k := range q.keys() {
		keys = append(keys, a.keyPath(k))
	}
	lines = append(lines, a.st.muted.Render("Writes ")+a.st.value.Render(strings.Join(keys, ", ")))

	return strings.Join(lines, "\n")
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
	for _, sec := range serverSections(settings) {
		var n, set int
		for _, st := range settings {
			if sectionOf(st.Key) != sec {
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
		label := sectionLabel(sec) + a.st.muted.Render(fmt.Sprintf("  %d %s", n, noun))
		if set > 0 {
			label += a.st.muted.Render(fmt.Sprintf(" · %d in the file", set))
		}
		opts = append(opts, huh.NewOption(label, sec))
	}

	sel := huh.NewMultiSelect[string]().
		Title("Which server settings do you want to change?").
		Description("Space selects, enter continues. Sections with values in the file start selected.").
		Options(opts...).
		Validate(func(v []string) error {
			if len(v) == 0 {
				return fmt.Errorf("select at least one section")
			}
			return nil
		}).
		Value(&a.sections)

	return a.sized(huh.NewForm(huh.NewGroup(sel)).WithTheme(a.theme).WithShowHelp(true))
}

func (a *app) newReviewForm() *huh.Form {
	opts := []huh.Option[string]{
		huh.NewOption("Save to "+shortPath(a.path), actSave),
		huh.NewOption("Save to another file…", actOther),
		huh.NewOption("Change answers", actEdit),
		huh.NewOption("Quit without saving", actQuit),
	}

	sel := huh.NewSelect[string]().
		Title("What next?").
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

	return a.sized(huh.NewForm(
		huh.NewGroup(sel),
		huh.NewGroup(path).WithHideFunc(func() bool { return a.action != actOther }),
	).WithTheme(a.theme).WithKeyMap(backKeyMap()).WithShowHelp(true))
}

// =============================================================================
// View

func (a *app) View() tea.View {
	var b strings.Builder

	b.WriteString(a.header())
	b.WriteString("\n\n")

	switch a.step {
	case stepLoading, stepFacts, stepCheck, stepSaving:
		b.WriteString(a.spin.View() + " " + a.status + "…")

	case stepStart:
		b.WriteString(a.startForm.View())

	case stepSections:
		b.WriteString(a.sectionsForm.View())

	case stepPick:
		if a.pickErr != nil {
			b.WriteString(a.st.bad.Render("Error: " + a.pickErr.Error()))
			b.WriteString("\n\n")
		}
		b.WriteString(a.pickForm.View())

	case stepQuestions:
		if a.server {
			b.WriteString(a.serverView())
		} else {
			b.WriteString(a.factsView())
		}
		b.WriteString("\n\n")
		b.WriteString(a.form.View())

	case stepReview:
		b.WriteString(a.reviewView())
		b.WriteString("\n\n")
		b.WriteString(a.reviewForm.View())

	case stepDone:
		b.WriteString(a.wrap(a.doneView()))

	case stepFailed:
		b.WriteString(a.wrap(a.st.bad.Render("Error: ") + a.err.Error()))
		b.WriteString("\n\n" + a.st.help.Render("Press any key to exit."))
	}

	v := tea.NewView(b.String() + "\n")
	v.AltScreen = true
	return v
}

func (a *app) header() string {
	title := a.st.brand.Render("kronk config")
	sub := "model_config.yaml"
	if a.path != "" {
		sub = shortPath(a.path)
	}

	return a.wrap(title + "  " + a.st.subtitle.Render(sub))
}

// wrap breaks text at the content width.
func (a *app) wrap(s string) string {
	if w := a.contentWidth(); w > 0 {
		return lipgloss.NewStyle().Width(w).Render(s)
	}
	return s
}

func (a *app) factsView() string {
	f := a.facts
	m := f.Analysis.Model
	r := f.Analysis.Recommended

	source := "installed"
	if !a.choice.Installed {
		source = "catalog, not downloaded"
	}

	var traits []string
	if f.moe() {
		traits = append(traits, "MoE")
	}
	if f.projection() {
		traits = append(traits, "vision")
	}
	if f.mtp() {
		traits = append(traits, "MTP head")
	}
	if f.swa() {
		traits = append(traits, "sliding window")
	}
	if len(traits) == 0 {
		traits = append(traits, "dense")
	}

	row := func(label, value string) string {
		return a.st.label.Render(fmt.Sprintf("%-10s", label)) + " " + a.st.value.Render(value)
	}

	lines := []string{
		a.st.heading.Render(f.ID) + "  " + a.st.muted.Render(source),
		row("Model", fmt.Sprintf("%s · %s · %s · trained on %d tokens", m.Architecture, m.Quantization, strings.Join(traits, ", "), m.TrainingContext)),
	}

	switch {
	case f.Other != nil:
		lines = append(lines, row("Machine", f.Other.String()))
	case !f.HardwareKnown:
		lines = append(lines, row("Machine", a.st.warn.Render("unknown, "+f.HardwareErr.Error())))
	default:
		hw := fmt.Sprintf("%d GPU, %s RAM", f.GPUs, formatBytes(f.Analysis.System.SystemRAMBytes))
		if f.Analysis.System.GPUTotalBytes > 0 {
			hw = fmt.Sprintf("%d GPU (%s), %s VRAM, %s RAM", f.GPUs, f.Analysis.System.GPUName,
				formatBytes(f.Analysis.System.GPUTotalBytes), formatBytes(f.Analysis.System.SystemRAMBytes))
		}
		lines = append(lines, row("Machine", hw))
	}

	auto := fmt.Sprintf("context %d · %d at a time · cache %s", r.ContextWindow, r.NSeqMax, r.CacheTypeK)
	if f.HardwareKnown {
		auto += " · " + fitText(r)
	}
	lines = append(lines, row("AutoTune", auto))

	if a.existing != nil {
		lines = append(lines, row("In file", a.st.warn.Render("this model has an entry; answers start from it")))
	}

	return a.boxed(lines)
}

func (a *app) serverView() string {
	return a.boxed([]string{
		a.st.heading.Render("Server settings") + "  " + a.st.muted.Render("kms: in "+shortPath(a.path)),
		a.st.value.Render("Environment variables and kronk server start flags override these values."),
	})
}

func (a *app) reviewView() string {
	var lines []string
	if a.server {
		lines = append(lines, a.st.heading.Render("Server settings"))
	} else {
		lines = append(lines, a.st.heading.Render(a.choice.ID))
	}

	if len(a.changes) == 0 {
		lines = append(lines, a.st.muted.Render("  No changes."))
	}

	for _, c := range a.changes {
		switch {
		case c.del:
			lines = append(lines, a.st.removed.Render(fmt.Sprintf("  - %s: %s", c.key, inlineYAML(c.old))))
		case c.old != nil:
			lines = append(lines, a.st.warn.Render(fmt.Sprintf("  ~ %s: %s → %s", c.key, inlineYAML(c.old), inlineYAML(c.new))))
		default:
			lines = append(lines, a.st.added.Render(fmt.Sprintf("  + %s: %s", c.key, inlineYAML(c.new))))
		}
	}

	// The preview runs the same edit as the save on the same file, so it
	// shows the lines exactly as they will be written.
	var entry string
	var err error
	if a.server {
		lines = append(lines, "", a.st.label.Render("Section as written"))
		entry, err = previewKMS(a.path, a.changes)
	} else {
		lines = append(lines, "", a.st.label.Render("Entry as written"))
		entry, err = previewEntry(a.path, a.choice.ID, a.changes)
	}
	switch {
	case err != nil:
		lines = append(lines, a.st.bad.Render("  "+err.Error()))
	case entry == "":
		// saveEntry removes an empty entry instead of writing it as {}.
		lines = append(lines, a.st.muted.Render("  Nothing for this model: Kronk's defaults and AutoTune apply."))
	default:
		for l := range strings.Lines(strings.TrimRight(entry, "\n")) {
			lines = append(lines, a.st.value.Render("  "+strings.TrimRight(l, "\n")))
		}
		if a.server && len(apply(a.existing, a.changes)) == 0 {
			lines = append(lines, a.st.muted.Render("  The server uses its defaults."))
		}
	}

	if a.server {
		return a.boxed(lines)
	}

	lines = append(lines, "")
	switch {
	case a.check != nil:
		r := a.check.Recommended
		lines = append(lines, a.st.label.Render("Memory    ")+fmt.Sprintf("context %d · %d at a time · cache %s · %s", r.ContextWindow, r.NSeqMax, r.CacheTypeK, fitText(r)))
		for _, w := range a.check.Warnings {
			lines = append(lines, a.st.warn.Render("  ! "+w))
		}
	case a.checkErr != nil:
		lines = append(lines, a.st.warn.Render("Memory check failed: "+a.checkErr.Error()))
	default:
		lines = append(lines, a.st.muted.Render("Memory check skipped: "+errNoLibs.Error()))
	}

	return a.boxed(lines)
}

// boxed draws lines in a box no wider than the terminal; long lines wrap.
func (a *app) boxed(lines []string) string {
	box := a.st.box
	if w := a.contentWidth(); w > 0 {
		box = box.Width(w)
	}
	return box.Render(strings.Join(lines, "\n"))
}

func (a *app) contentWidth() int {
	if a.width <= 0 {
		return 0
	}
	return min(a.width, 110)
}

func (a *app) doneView() string {
	var lines []string
	lines = append(lines, a.st.ok.Render("Saved ")+shortPath(a.savedPath))
	if a.backup != "" {
		lines = append(lines, a.st.muted.Render("Backup: "+shortPath(a.backup)))
	}
	lines = append(lines, "")

	switch {
	case a.facts.Other != nil:
		lines = append(lines,
			"This config is for "+a.facts.Other.String()+".",
			"Copy it there as ~/.kronk/models/model_config.yaml, or start that server with:",
			a.st.value.Render("  KRONK_POOL_MODEL_CONFIG_FILE=<path> kronk server start"))
	case a.savedPath == a.serverFile:
		lines = append(lines, "The server reads this file at startup. Restart it to apply the changes.")
	default:
		lines = append(lines,
			"The server reads "+shortPath(a.serverFile)+".",
			"To use this file, copy it there or start the server with:",
			a.st.value.Render("  KRONK_POOL_MODEL_CONFIG_FILE="+a.savedPath+" kronk server start"))
	}

	lines = append(lines, "", a.st.help.Render("Press any key to exit."))

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// =============================================================================

func fitText(r models.RuntimeRecommendation) string {
	mem := formatBytes(uint64(max(r.EstimatedVRAMBytes, 0)))
	if r.Fits {
		return "fits (" + mem + ")"
	}
	if r.Reason != "" {
		return "does not fit (" + mem + "): " + r.Reason
	}
	return "does not fit (" + mem + ")"
}

func formatBytes(n uint64) string {
	const gb = 1 << 30
	const mb = 1 << 20
	if n >= gb {
		return fmt.Sprintf("%.1f GB", float64(n)/gb)
	}
	return fmt.Sprintf("%d MB", n/mb)
}
