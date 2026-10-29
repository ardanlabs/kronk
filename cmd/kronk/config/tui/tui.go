// Package tui is the config tool's terminal UI: a Bubble Tea program with
// one huh form per step (start, sections or model, variant, questions,
// review) and the answers list beside them.
package tui

import (
	"context"
	"fmt"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/ardanlabs/kronk/cmd/kronk/config/configfile"
	"github.com/ardanlabs/kronk/cmd/kronk/config/questions"
	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

type step int

const (
	stepLoading step = iota
	stepStart
	stepSections
	stepGroup // the kind of model, before the list
	stepPick
	stepVariant
	stepFacts
	stepQuestions
	stepCheck
	stepReview
	stepSaving
	stepOffer       // saved; the model is not downloaded
	stepDownloading // the model is downloading
	stepDone
	stepFailed
)

// Review actions.
const (
	actSave  = "save"
	actOther = "other"
	actOne   = "one"
	actQuit  = "quit"
	actFix   = "fix:" // followed by the index in app.fixes
)

// keyAnswers opens the answers list. No huh field or text input binds it.
const keyAnswers = "ctrl+o"

// keyAction is the key of the review's action field, which saves or goes back.
const keyAction = "action"

type startMsg struct {
	serverFile string
	files      []string
	err        error
}

type loadedMsg struct {
	path    string
	doc     modelconfig.Document
	choices []source.Choice
	err     error
}

type factsMsg struct {
	facts source.Facts
	err   error
}

type checkMsg struct {
	analysis models.Analysis
	fixes    []questions.Fix
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
	src    *source.Source
	st     styles
	theme  huh.Theme
	colors *colors // the palette, which follows the background
	spin   spinner.Model
	width  int
	height int

	step   step
	status string
	err    error

	// forms holds the last form of each step that has one. The form of a
	// step left behind is kept, so going back can reopen it where it was.
	forms map[step]*huh.Form

	serverFile string
	files      []string
	start      *start

	path    string
	doc     modelconfig.Document
	choices []source.Choice

	// server is set when the user configures the server settings (kms:)
	// instead of a model. sections holds the sections to change.
	server   bool
	sections []string

	// formErr is shown above the form of the start, sections, pick and
	// variant steps, until that form is submitted.
	formErr error

	picked string

	// group is the kind of model listed (source.Group, or groupAll).
	// groupAsked is set when the picker asked for it.
	group      string
	groupAsked bool

	choice source.Choice

	// entryID is the entry written: the model ID, or model/NAME for a
	// variant. section is models, bucky-models or malina-models.
	entryID      string
	section      string
	variantPick  string
	variantName  string
	variantAsked bool

	facts    source.Facts
	qs       []questions.Question
	ans      *questions.Answers
	existing map[string]any

	changes  []configfile.Change
	check    *models.Analysis
	checkErr error
	fixes    []questions.Fix // offered when the check says it doesn't fit
	problems []string        // answers the SDK refuses together; saving waits

	action    string
	otherPath string
	saveErr   error // the last save failed; the review offers another file

	jump jump
	list list

	savedPath string
	backup    string
	pull      pull
	done      buttons // All done's buttons
}

// jump is changing one answer on its own, picked in the review or in the
// answers list.
type jump struct {
	// to is the question to change. Once it is answered the review comes
	// back, unless the answer opened questions that were not asked before
	// (seen lists those that were).
	to   string
	seen map[string]bool

	// back is the question that was on screen when the answers list opened
	// from the questions; after the change the questions resume there.
	back string
}

// list is the answers list while it has the keys (ctrl+o).
type list struct {
	open bool
	at   int // cursor
}

// New returns the Bubble Tea model of the config tool. isDark is the
// terminal's background at startup; with follow the colors also follow it
// when it changes.
func New(ctx context.Context, src *source.Source, isDark bool, follow bool) tea.Model {
	a := newApp(ctx, src, isDark)
	a.colors.follow = follow
	return a
}

func newApp(ctx context.Context, src *source.Source, isDark bool) *app {
	st := newStyles(isDark)
	c := newColors(isDark, false)

	return &app{
		ctx:    ctx,
		src:    src,
		st:     st,
		colors: c,
		theme:  huh.ThemeFunc(func(bool) *huh.Styles { return c.form() }),
		spin:   spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(st.heading)),
		status: "Looking for config files",
		forms:  map[step]*huh.Form{},
	}
}

func (a *app) Init() tea.Cmd {
	return tea.Batch(a.spin.Tick, a.loadStart, a.watchBackground())
}

// =============================================================================
// Commands

func (a *app) loadStart() tea.Msg {
	path, err := a.src.ConfigFile()
	if err != nil {
		return startMsg{err: err}
	}

	return startMsg{serverFile: path, files: source.ConfigFiles(path)}
}

func (a *app) load(path string) tea.Cmd {
	return func() tea.Msg {
		doc, err := a.src.LoadDoc(path)
		if err != nil {
			return loadedMsg{err: fmt.Errorf("read %s: %w", path, err)}
		}

		choices, err := a.src.Choices(doc)
		if err != nil {
			return loadedMsg{err: err}
		}

		return loadedMsg{path: path, doc: doc, choices: choices}
	}
}

func (a *app) loadFacts(c source.Choice) tea.Cmd {
	var other *source.Machine
	if m, ok := a.start.machine(); ok {
		other = &m
	}

	return func() tea.Msg {
		f, err := a.src.Facts(a.ctx, c, other)
		return factsMsg{facts: f, err: err}
	}
}

func (a *app) runCheck(cfg models.ModelConfig) tea.Cmd {
	f := a.facts
	return func() tea.Msg {
		an, err := a.src.Check(f, cfg)
		if err != nil {
			return checkMsg{err: err}
		}
		return checkMsg{analysis: an, fixes: questions.Fixes(f.Info, f.Devices, cfg, an)}
	}
}

func (a *app) save(path string) tea.Cmd {
	id, section, changes, server := a.entryID, a.section, a.changes, a.server
	values := questions.Apply(a.existing, changes)
	other := path != a.path
	return func() tea.Msg {
		var backup string
		var err error
		switch {
		case server && other:
			backup, err = configfile.SaveKMSValues(path, values)
		case server:
			backup, err = configfile.SaveKMS(path, changes)
		case other:
			backup, err = configfile.SaveEntryValues(path, section, id, values)
		default:
			backup, err = configfile.SaveEntry(path, section, id, changes)
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
		return a.updateForm(a.formSize())

	case tea.KeyPressMsg:
		switch a.step {
		case stepFailed:
			return a, tea.Quit
		case stepDone:
			return a.doneKey(msg)
		case stepLoading, stepFacts, stepCheck, stepSaving:
			if msg.String() == "ctrl+c" {
				return a, tea.Quit
			}
		case stepDownloading:
			return a.pullKey(msg)
		case stepOffer:
			return a.offerKey(msg)
		case stepStart, stepSections, stepGroup, stepPick, stepVariant, stepQuestions, stepReview:
			if a.list.open {
				return a.listKey(msg)
			}
			if msg.String() == keyAnswers && (a.step == stepQuestions || a.step == stepReview) {
				return a.openList()
			}
			if msg.String() == "esc" && !a.fieldUsesEsc() {
				return a.back()
			}
			if msg.String() == "esc" && a.step == stepPick {
				return a.clearSearch(msg)
			}
		}

	case backgroundTickMsg:
		return a, tea.Batch(tea.RequestBackgroundColor, a.watchBackground())

	case tea.BackgroundColorMsg:
		return a.setBackground(msg.IsDark())

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
		return a.open(stepStart)

	case loadedMsg:
		if msg.err != nil {
			return a.fail(msg.err)
		}
		a.path, a.doc, a.choices = msg.path, msg.doc, msg.choices
		if len(a.choices) == 0 {
			return a.fail(fmt.Errorf("no models installed and the catalog is empty, pull a model first: kronk model pull --local <id>"))
		}
		return a.openGroups()

	case factsMsg:
		if msg.err != nil {
			return a.backToPick(msg.err)
		}
		return a.startQuestions(msg.facts)

	case checkMsg:
		if msg.err != nil {
			a.checkErr = msg.err
		} else {
			a.check = &msg.analysis
			a.fixes = msg.fixes
		}
		return a.startReview()

	case savedMsg:
		if msg.err != nil {
			a.saveErr = msg.err
			return a.startReview()
		}
		a.savedPath, a.backup = msg.path, msg.backup
		if a.needsPull() {
			return a.offerPull()
		}
		return a.finish()

	case pullProgressMsg:
		a.pull.last = models.DownloadProgress(msg)
		return a, a.waitPull()

	case pullDoneMsg:
		return a.pullDone(msg.err)
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

	if a.step == stepQuestions && a.jump.to != "" && form.State == huh.StateNormal {
		done, cancel := a.jumpDone()
		switch {
		case cancel:
			a.jump = jump{}
		case done:
			// Skip the questions already asked, to the next one the new
			// answer opened, if any.
			if next := a.nextNew(); next != "" {
				return a, tea.Batch(append([]tea.Cmd{cmd}, skipTo(form, next, a.maxGroups())...)...)
			}
			return a.endJump()
		}
	}

	switch form.State {
	case huh.StateAborted:
		return a, tea.Quit

	case huh.StateCompleted:
		a.formErr = nil
		switch a.step {
		case stepStart:
			return a.startDone()
		case stepSections:
			return a.sectionsDone()
		case stepGroup:
			return a.groupDone()
		case stepPick:
			return a.pickDone()
		case stepVariant:
			return a.variantDone()
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

// busy shows the spinner with status while cmd runs.
func (a *app) busy(s step, status string, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	a.step, a.status = s, status
	return a, tea.Batch(a.spin.Tick, cmd)
}
