package tui

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/ardanlabs/kronk/cmd/kronk/config/questions"
)

// The answers list shows every question asked for this model, in order, with
// its answer, so the choices can be checked as they are made. When the
// terminal is wide enough it is a column right of the questions and the
// review. ctrl+o opens it full screen, to pick an answer to change.

// panelMinWidth is the narrowest terminal that shows the answers column.
const panelMinWidth = 120

// answerRow is one line of the answers column.
type answerRow struct {
	group     string
	key       string
	value     string
	isDefault bool
}

// showPanel reports whether the answers column is on screen.
func (a *app) showPanel() bool {
	if a.width < panelMinWidth || a.ans == nil || a.list.open {
		return false
	}
	return a.step == stepQuestions || a.step == stepReview
}

// panelWidth is the width of the answers column, border included.
func (a *app) panelWidth() int {
	return min(56, a.width/3+4)
}

// formWidth is the width the forms get: the whole terminal, or what is left
// of it next to the answers column.
func (a *app) formWidth() int {
	if a.showPanel() {
		return a.width - a.panelWidth() - 2
	}
	return a.width
}

// answerRows returns the questions asked, in order, with their answers.
func (a *app) answerRows() []answerRow {
	var rows []answerRow
	for _, q := range a.qs {
		if !a.ans.Applies(a.facts, q) {
			continue
		}
		value, isDefault := a.answerText(q)
		rows = append(rows, answerRow{group: q.Group, key: q.Key, value: value, isDefault: isDefault})
	}
	return rows
}

// answerText is the answer to q as shown in the column and the review, and
// whether it is the default.
func (a *app) answerText(q questions.Question) (string, bool) {
	sel := *a.ans.Sel[q.Key]
	value := a.ans.Value(q.Key)

	switch {
	case q.Secret || q.Kind == questions.KindPassword:
		if value != "" {
			return "new value", false
		}
		if _, ok := a.existing[q.Key]; ok {
			return "unchanged", false
		}
	case sel == questions.OtherValue && value == "":
		return "…", false
	}

	if value == "" {
		// AutoTune's pick is the default; the prefix only takes room here.
		return strings.TrimPrefix(questions.DefaultLabel(q, a.facts), "AutoTune's pick: ") + " (default)", true
	}

	switch {
	case q.Kind == questions.KindMulti:
		return strings.ReplaceAll(value, ",", ", "), false
	case q.Kind == questions.KindVars || q.Kind == questions.KindAdapters:
		return strings.Join(questions.SplitList(value, "\n"), ", "), false
	case sel != questions.OtherValue:
		if o, ok := q.Option(a.facts, value); ok {
			return o.Label, false
		}
	}

	return value, false
}

// shortKey drops the sampling-parameters prefix, which every sampling key
// shares, to keep the column narrow.
func shortKey(key string) string {
	return strings.TrimPrefix(key, "sampling-parameters.")
}

// panelView draws the answers list in a box width wide. It scrolls to keep
// the cursor in view: the list cursor when the list is open, otherwise the
// question on screen.
func (a *app) panelView(width, height int) string {
	inner := width - 4 // border and padding
	rows := a.answerRows()

	cursor := ""
	switch {
	case a.list.open && a.list.at < len(rows):
		cursor = rows[a.list.at].key
	case a.step == stepQuestions:
		cursor = focusedKey(a.currentForm())
	}

	keyWidth := 0
	for _, r := range rows {
		keyWidth = max(keyWidth, len(shortKey(r.key)))
	}
	keyWidth = min(keyWidth, inner/2)

	// The group of the cursor row is bright too.
	cursorGroup := ""
	for _, r := range rows {
		if r.key == cursor {
			cursorGroup = r.group
		}
	}

	var lines []string
	at := 0
	group := ""
	for _, r := range rows {
		if r.group != group && r.group != "" {
			group = r.group
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			style := a.st.label
			if group == cursorGroup {
				style = a.st.selected
			}
			lines = append(lines, style.Render(group))
		}

		marker := "  "
		if r.key == cursor {
			at = len(lines)
			marker = a.st.heading.Render("› ")
		}
		key := truncate(shortKey(r.key), keyWidth)
		value := truncate(r.value, inner-keyWidth-3)
		keyStyle, style := a.st.label, a.st.value
		switch {
		case r.key == cursor:
			// The row being looked at stands out in bright bold.
			keyStyle, style = a.st.selected, a.st.selected
		case r.isDefault:
			style = a.st.muted
		}
		lines = append(lines, marker+keyStyle.Render(fmt.Sprintf("%-*s", keyWidth, key))+" "+style.Render(value))
	}

	// Border, heading and the blank line under it; the open list also has
	// its key help at the bottom.
	room := height - 4
	var help string
	if a.list.open {
		help = a.listHelp(inner)
		room -= lipgloss.Height(help) + 1
	}
	room = max(room, 3)

	// Scroll: keep the cursor in the middle when the list is taller than
	// the box.
	if len(lines) > room {
		start := min(max(at-room/2, 0), len(lines)-room)
		end := start + room
		more := len(lines) - end
		lines = lines[start:end]
		if start > 0 {
			lines[0] = a.st.muted.Render(fmt.Sprintf("  ↑ %d more", start+1))
		}
		if more > 0 {
			lines[len(lines)-1] = a.st.muted.Render(fmt.Sprintf("  ↓ %d more", more+1))
		}
	}

	lines = append([]string{a.st.heading.Render("Answers"), ""}, lines...)
	if help != "" {
		lines = append(lines, "", help)
	}
	return a.st.box.Width(width).Render(strings.Join(lines, "\n"))
}

// listHelp is the key help of the open list, in the forms' help style.
func (a *app) listHelp(width int) string {
	h := help.New()
	h.Styles = a.theme.Theme(false).Help
	h.SetWidth(width)
	return h.ShortHelpView([]key.Binding{
		key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "up")),
		key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "down")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "change")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "close")),
	})
}

// openList gives the keys to the answers list, with the cursor on the
// question on screen.
func (a *app) openList() (tea.Model, tea.Cmd) {
	rows := a.answerRows()
	if len(rows) == 0 {
		return a, nil
	}

	a.list.open = true
	a.list.at = min(a.list.at, len(rows)-1)
	if a.step == stepQuestions {
		if i := slices.IndexFunc(rows, func(r answerRow) bool { return r.key == focusedKey(a.currentForm()) }); i >= 0 {
			a.list.at = i
		}
	}
	return a, nil
}

// listKey handles a key while the answers list is open.
func (a *app) listKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	rows := a.answerRows()
	page := max(a.height/2, 1)

	switch msg.String() {
	case "ctrl+c":
		return a, tea.Quit
	case "esc", keyAnswers:
		a.list.open = false
	case "up", "k":
		a.list.at--
	case "down", "j":
		a.list.at++
	case "pgup":
		a.list.at -= page
	case "pgdown":
		a.list.at += page
	case "home", "g":
		a.list.at = 0
	case "end", "G":
		a.list.at = len(rows) - 1
	case "enter":
		a.list.open = false
		if a.list.at >= 0 && a.list.at < len(rows) {
			return a.answerPicked(rows[a.list.at].key)
		}
	}
	a.list.at = min(max(a.list.at, 0), len(rows)-1)
	return a, nil
}

// answerPicked opens the question picked in the answers list. From the
// review, the review comes back once it is answered; from the questions,
// they resume at the question that was on screen.
func (a *app) answerPicked(key string) (tea.Model, tea.Cmd) {
	if a.step == stepQuestions && a.jump.to == "" {
		current := focusedKey(a.currentForm())
		if current == "" || current == key {
			return a, nil
		}
		a.jump.back = current
	}

	return a.editOne(key)
}

// withAnswersKey adds ctrl+o to a field's key help. huh shows the focused
// field's keys there, so the wrapper appends one to them.
func withAnswersKey(f huh.Field) huh.Field {
	return &answersKey{Field: f}
}

type answersKey struct {
	huh.Field
}

var answersBinding = key.NewBinding(key.WithKeys(keyAnswers), key.WithHelp(keyAnswers, "answers"))

func (w *answersKey) KeyBinds() []key.Binding {
	return append(w.Field.KeyBinds(), answersBinding)
}

// The methods below return the wrapper, so huh keeps it in the group.

func (w *answersKey) Update(msg tea.Msg) (huh.Model, tea.Cmd) {
	m, cmd := w.Field.Update(msg)
	w.Field = m.(huh.Field)
	return w, cmd
}

func (w *answersKey) WithTheme(t huh.Theme) huh.Field {
	w.Field = w.Field.WithTheme(t)
	return w
}

func (w *answersKey) WithKeyMap(k *huh.KeyMap) huh.Field {
	w.Field = w.Field.WithKeyMap(k)
	return w
}

func (w *answersKey) WithWidth(n int) huh.Field {
	w.Field = w.Field.WithWidth(n)
	return w
}

func (w *answersKey) WithHeight(n int) huh.Field {
	w.Field = w.Field.WithHeight(n)
	return w
}

func (w *answersKey) WithPosition(p huh.FieldPosition) huh.Field {
	w.Field = w.Field.WithPosition(p)
	return w
}

// truncate shortens s to n cells with an ellipsis.
func truncate(s string, n int) string {
	if n <= 1 || lipgloss.Width(s) <= n {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > n {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}
