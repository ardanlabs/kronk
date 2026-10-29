package tui

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/ardanlabs/kronk/cmd/kronk/config/configfile"
	"github.com/ardanlabs/kronk/cmd/kronk/config/questions"
	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
	kronkserver "github.com/ardanlabs/kronk/cmd/server/api/services/kronk"
)

// currentForm is the form of the current step, nil when the step has none.
func (a *app) currentForm() *huh.Form {
	return a.forms[a.step]
}

// buildForm builds a new form for step s, bound to the answers so far.
func (a *app) buildForm(s step) *huh.Form {
	switch s {
	case stepStart:
		return a.newStartForm()
	case stepSections:
		return a.newSectionsForm()
	case stepGroup:
		return a.newGroupForm()
	case stepPick:
		return a.newPickForm()
	case stepVariant:
		return a.newVariantForm()
	case stepQuestions:
		return a.newQuestionsForm()
	case stepReview:
		return a.newReviewForm()
	}
	return nil
}

// open moves to step s with a new form on its first question.
func (a *app) open(s step) (tea.Model, tea.Cmd) {
	a.step = s
	a.forms[s] = a.buildForm(s)
	return a, a.forms[s].Init()
}

// openAt moves to step s with a new form on the field with key target.
func (a *app) openAt(s step, target string) (tea.Model, tea.Cmd) {
	a.step = s
	a.forms[s] = a.buildForm(s)
	return a, initAt(a.forms[s], target, a.maxGroups())
}

// reopen moves to step s with a new form on the field that was on screen
// when its last form was left. A submitted huh form can't be reopened; the
// new one is bound to the same answers.
func (a *app) reopen(s step) (tea.Model, tea.Cmd) {
	var target string
	if old := a.forms[s]; old != nil {
		if f := old.GetFocusedField(); f != nil {
			target = f.GetKey()
		}
	}
	return a.openAt(s, target)
}

// maxGroups bounds the walk to a field that is not in the form: more than
// any form has groups.
func (a *app) maxGroups() int {
	return 2*len(a.qs) + len(a.files) + 10
}

// focusedKey is the question key of the focused field, without the /other
// of a free-form input. Empty when nothing is focused.
func focusedKey(form *huh.Form) string {
	if form == nil {
		return ""
	}
	f := form.GetFocusedField()
	if f == nil {
		return ""
	}
	key, _, _ := strings.Cut(f.GetKey(), "/")
	return key
}

// back moves to the previous question of the current form, skipping the
// ones that are hidden. From the first question it leaves the step, in the
// order start → model list → questions → review, landing on the last
// question of the step it returns to.
func (a *app) back() (tea.Model, tea.Cmd) {
	form := a.currentForm()

	// Changing one answer from the review: esc goes back to the review,
	// unless the answer on screen is not valid yet.
	if a.step == stepQuestions && a.jump.to != "" {
		key := focusedKey(form)
		if q, ok := a.ans.ByKey[key]; ok && q.Validate(a.ans.Value(key)) == nil {
			return a.endJump()
		}
	}

	// In the review, esc from the file path or the answer list goes back
	// to the menu, even when the path is still empty.
	if a.step == stepReview {
		if f := form.GetFocusedField(); f != nil && f.GetKey() != keyAction {
			return a.open(stepReview)
		}
	}

	// PrevGroup applies at once and stays put when no earlier group is
	// visible, or when the current one has a validation error.
	before := form.GetFocusedField()
	cmd := form.PrevGroup()
	if form.GetFocusedField() != before || len(form.Errors()) > 0 {
		return a, cmd
	}

	switch a.step {
	case stepGroup, stepSections:
		return a.reopen(stepStart)

	case stepPick:
		if a.groupAsked {
			return a.reopen(stepGroup)
		}
		return a.reopen(stepStart)

	case stepVariant:
		return a.open(stepPick)

	case stepQuestions:
		switch {
		case a.server:
			return a.open(stepSections)
		case a.variantAsked:
			return a.open(stepVariant)
		}
		return a.open(stepPick)

	case stepReview:
		return a.reopen(stepQuestions)
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

// backToPick shows the picker again with err above it. Used when a model
// can't be read, so the user can pick another one.
func (a *app) backToPick(err error) (tea.Model, tea.Cmd) {
	if a.choice.Installed && !a.choice.Validated {
		err = fmt.Errorf("%w\nThe model files look incomplete. Pull it again: kronk model pull --local %s", err, a.choice.ID)
	}

	a.formErr = err
	a.picked = ""
	return a.open(stepPick)
}

func (a *app) startDone() (tea.Model, tea.Cmd) {
	same := a.server && a.path == a.start.path()
	a.path = a.start.path()
	a.server = a.start.server()

	// Back from the sections to the start and on again: keep the sections
	// picked and the answers given.
	if a.server && same {
		return a.open(stepSections)
	}

	if a.server {
		existing, err := configfile.ReadKMS(a.path)
		if err != nil {
			return a.fail(fmt.Errorf("read %s: %w", a.path, err))
		}

		a.facts = source.Facts{}
		a.existing = existing
		a.sections = nil
		for k := range existing {
			if sec := questions.ServerSection(k); !slices.Contains(a.sections, sec) {
				a.sections = append(a.sections, sec)
			}
		}
		a.qs = questions.Server(kronkserver.Settings(), &a.sections, existing)
		a.ans = questions.NewAnswers(a.qs)
		a.ans.Prefill(a.facts, existing)

		return a.open(stepSections)
	}

	return a.busy(stepLoading, "Reading "+shortPath(a.path)+", models and the catalog", a.load(a.path))
}

// sectionsDone asks the questions of the sections picked. The form has no
// Validate for an empty pick: huh's multi-select runs it whenever its value
// is set, so the error would show before the user picks anything.
func (a *app) sectionsDone() (tea.Model, tea.Cmd) {
	if len(a.sections) == 0 {
		a.formErr = errors.New("select at least one section")
		return a.open(stepSections)
	}
	return a.openQuestions()
}

func (a *app) pickDone() (tea.Model, tea.Cmd) {

	i := slices.IndexFunc(a.choices, func(c source.Choice) bool { return c.ID == a.picked })
	if i < 0 {
		return a.fail(fmt.Errorf("model %q not found", a.picked))
	}

	a.choice = a.choices[i]
	a.section = source.SectionOf(a.choice.Kind)
	a.entryID = a.choice.ID
	a.variantAsked = false

	if a.choice.Kind != source.KindChatModel {
		return a.startMediaQuestions()
	}

	// A model with entries in the file asks which one to change, or for
	// a new variant.
	if len(source.EntriesFor(a.doc, a.choice.ID)) > 0 {
		a.variantAsked = true
		a.variantPick, a.variantName = a.choice.ID, ""
		return a.open(stepVariant)
	}

	return a.readFacts()
}

func (a *app) variantDone() (tea.Model, tea.Cmd) {
	a.entryID = a.variantPick
	if a.variantPick == newVariant {
		a.entryID = a.choice.ID + "/" + strings.TrimSpace(a.variantName)
	}
	return a.readFacts()
}

func (a *app) readFacts() (tea.Model, tea.Cmd) {
	status := "Reading " + a.choice.ID
	if !a.choice.Installed {
		status += " from the catalog (may download the GGUF header)"
	}

	return a.busy(stepFacts, status, a.loadFacts(a.choice))
}

// startMediaQuestions asks the audio or image model questions. These models
// have no GGUF analysis and no memory check.
func (a *app) startMediaQuestions() (tea.Model, tea.Cmd) {
	a.facts = source.Facts{ID: a.choice.ID}
	a.qs = questions.Audio()
	if a.choice.Kind == source.KindImageModel {
		a.qs = questions.Image()
	}
	a.ans = questions.NewAnswers(a.qs)

	existing, err := questions.MediaExisting(a.doc, a.choice.Kind, a.choice.ID)
	if err != nil {
		return a.fail(fmt.Errorf("read existing entry: %w", err))
	}
	a.existing = existing
	a.ans.Prefill(a.facts, existing)

	return a.openQuestions()
}

func (a *app) startQuestions(f source.Facts) (tea.Model, tea.Cmd) {
	// The drafts for classic speculation: installed chat models with the
	// same tokenizer. Without the model's tokenizer none can be checked.
	if tok := f.Tokenizer(); tok != "" {
		for _, c := range a.choices {
			if c.Installed && c.Kind == source.KindChatModel && c.Endpoint == "" && c.ID != f.ID && c.Tokenizer == tok {
				f.Drafts = append(f.Drafts, c.ID)
			}
		}
	}

	a.facts = f
	a.qs = questions.Chat()
	a.ans = questions.NewAnswers(a.qs)
	a.existing = nil

	if mc, ok := a.doc.Models[a.entryID]; ok {
		existing, err := configfile.FlattenYAML(mc)
		if err != nil {
			return a.fail(fmt.Errorf("read existing entry: %w", err))
		}
		a.existing = existing
		a.ans.Prefill(f, existing)
	}

	return a.openQuestions()
}

func (a *app) questionsDone() (tea.Model, tea.Cmd) {
	a.jump = jump{}
	changes, err := questions.Diff(a.qs, a.facts, a.ans, a.existing)
	if err != nil {
		return a.fail(err)
	}
	a.changes = changes
	a.check, a.checkErr, a.fixes, a.problems, a.saveErr = nil, nil, nil, nil, nil

	if a.server {
		return a.startReview()
	}

	// Answers the SDK would refuse are shown in the review, which doesn't
	// save until they are fixed. They skip the memory check.
	flat := questions.Apply(a.existing, changes)
	if a.choice.Kind != source.KindChatModel {
		if err := questions.CheckMedia(a.choice.Kind, flat); err != nil {
			a.problems = []string{err.Error()}
		}
		return a.startReview()
	}

	cfg, err := questions.ToModelConfig(flat)
	if err != nil {
		a.problems = []string{err.Error()}
		return a.startReview()
	}
	if a.problems = questions.Problems(cfg); len(a.problems) > 0 {
		return a.startReview()
	}

	if !a.facts.HardwareKnown {
		return a.startReview()
	}

	status := "Checking memory on this machine"
	if a.facts.Other != nil {
		status = "Checking memory on the target machine"
	}
	return a.busy(stepCheck, status, a.runCheck(cfg))
}

func (a *app) startReview() (tea.Model, tea.Cmd) {
	a.action = actSave
	switch {
	case len(a.problems) > 0:
		a.action = actOne
	case a.saveErr != nil:
		// Keep the path typed before, so it can be corrected.
		a.action = actOther
	case len(a.fixes) > 0:
		a.action = actFix + "0"
	}
	if a.saveErr == nil {
		a.otherPath = ""
	}
	return a.open(stepReview)
}

// initAt moves a new form to the field with key target, then initializes it.
// Moving first matters: when the first group is hidden, Init skips it with a
// message that arrives after the first frame, so that frame would show the
// hidden question, and the late skip would also move past target. A visible
// target is never past the end, so NextGroup doesn't submit before reaching
// it; limit bounds the walk for a key that is not in the form.
func initAt(form *huh.Form, target string, limit int) tea.Cmd {
	return tea.Batch(append(skipTo(form, target, limit), form.Init())...)
}

// skipTo moves form forward to the field with key target.
func skipTo(form *huh.Form, target string, limit int) []tea.Cmd {
	var cmds []tea.Cmd
	for range limit {
		f := form.GetFocusedField()
		if f == nil || f.GetKey() == target {
			break
		}
		cmds = append(cmds, form.NextGroup())
	}
	return cmds
}

// openQuestions builds the questions form on the first question that is
// asked.
func (a *app) openQuestions() (tea.Model, tea.Cmd) {
	var first string
	for _, q := range a.qs {
		if a.ans.Applies(a.facts, q) {
			first = q.Key
			break
		}
	}

	return a.openAt(stepQuestions, first)
}

func (a *app) editOne(key string) (tea.Model, tea.Cmd) {
	a.jump.to = key
	a.jump.seen = map[string]bool{}
	for _, r := range a.answerRows() {
		a.jump.seen[r.key] = true
	}

	return a.openAt(stepQuestions, key)
}

// endJump ends changing one answer: back to the review, or to the question
// the answers list was opened on.
func (a *app) endJump() (tea.Model, tea.Cmd) {
	back := a.jump.back
	a.jump = jump{}
	if back == "" {
		return a.questionsDone()
	}

	// The question may be hidden now; resume at the next one asked.
	from := false
	for _, q := range a.qs {
		from = from || q.Key == back
		if from && a.ans.Applies(a.facts, q) {
			return a.openAt(stepQuestions, q.Key)
		}
	}
	return a.questionsDone()
}

// nextNew returns the first question from the one on screen on that is
// asked now but was not when the review sent the user to change one answer.
func (a *app) nextNew() string {
	key := focusedKey(a.forms[stepQuestions])
	if key == "" {
		return ""
	}

	from := false
	for _, q := range a.qs {
		from = from || q.Key == key
		if from && !a.jump.seen[q.Key] && a.ans.Applies(a.facts, q) {
			return q.Key
		}
	}
	return ""
}

// jumpDone reports whether the question picked in the review is answered:
// the form moved on to a question that was already asked. cancel reports
// that it moved back before it instead, so the questions go on from there.
func (a *app) jumpDone() (done bool, cancel bool) {
	key := focusedKey(a.forms[stepQuestions])
	if key == "" || key == a.jump.to {
		return false, false
	}

	for _, q := range a.qs {
		switch q.Key {
		case key:
			return false, true // reached before the picked question
		case a.jump.to:
			return a.jump.seen[key], false
		}
	}
	return false, false
}

func (a *app) reviewDone() (tea.Model, tea.Cmd) {
	if n, ok := strings.CutPrefix(a.action, actFix); ok {
		i, _ := strconv.Atoi(n)
		if i >= 0 && i < len(a.fixes) {
			a.ans.ApplyFix(a.facts, a.fixes[i])
		}
		return a.questionsDone()
	}

	switch a.action {
	case actOne:
		// The submitted review can't be reopened; a new one waits under
		// the list.
		m, cmd := a.open(stepReview)
		a.openList()
		return m, cmd

	case actQuit:
		return a, tea.Quit
	}

	path := a.path
	if a.action == actOther {
		path = expandHome(a.otherPath)
	}
	a.saveErr = nil
	return a.busy(stepSaving, "Saving", a.save(path))
}
