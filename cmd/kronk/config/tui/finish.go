package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/ardanlabs/kronk/cmd/kronk/config/questions"
	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
)

// After saving, two screens end the flow. When the model isn't installed,
// the first asks whether to install it now or copy its pull command. The
// second, All done, sums up what was written and where, with buttons to
// configure something else or exit.

// buttons is a row of buttons; left and right move between them.
type buttons struct {
	labels []string
	at     int
}

// key moves the focus, or reports the button pressed with enter.
func (b *buttons) key(msg tea.KeyPressMsg) (pressed int, ok bool) {
	switch msg.String() {
	case "left", "h", "shift+tab":
		b.at = max(b.at-1, 0)
	case "right", "l", "tab":
		b.at = min(b.at+1, len(b.labels)-1)
	case "enter":
		return b.at, true
	}
	return 0, false
}

// buttonsView draws the buttons in a row, or one per line when the row is
// wider than width (0: no limit).
func (a *app) buttonsView(b buttons, width int) string {
	st := a.theme.Theme(false)
	parts := make([]string, len(b.labels))
	for i, l := range b.labels {
		style := st.Focused.BlurredButton
		if i == b.at {
			style = st.Focused.FocusedButton
		}
		parts[i] = style.Render(l)
	}

	// Too narrow for a row: one button per line.
	row := lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	if width > 0 && lipgloss.Width(row) > width {
		return lipgloss.JoinVertical(lipgloss.Left, parts...)
	}
	return row
}

// titledBox draws content in a rounded box with title set in its top border.
// It fits its content, and wraps it when wider than width (0: no limit).
func (a *app) titledBox(title string, content string, width int) string {
	style := a.st.box.BorderTop(false)
	body := style.Render(content)
	if width > 0 && lipgloss.Width(body) > width {
		body = style.Width(width).Render(content)
	}

	border := lipgloss.NewStyle().Foreground(a.st.box.GetBorderLeftForeground())
	label := " " + title + " "
	fill := max(lipgloss.Width(body)-lipgloss.Width(label)-3, 0)
	top := border.Render("╭─") + a.st.label.Render(label) + border.Render(strings.Repeat("─", fill)+"╮")

	return top + "\n" + body
}

// =============================================================================
// Install offer

// Install offer buttons.
const (
	btnInstall = "Install now"
	btnCopy    = "Copy command"
)

// offerPull opens the install offer for a saved model that isn't installed.
// For another machine it can only copy the command, to run there.
func (a *app) offerPull() (tea.Model, tea.Cmd) {
	a.step = stepOffer
	a.pull.offer = buttons{labels: []string{btnCopy, btnInstall}, at: 1}
	if a.forAnother() {
		a.pull.offer = buttons{labels: []string{btnCopy}}
	}
	return a, nil
}

func (a *app) offerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return a, tea.Quit
	case "esc":
		return a.finish()
	}

	i, ok := a.pull.offer.key(msg)
	if !ok {
		return a, nil
	}

	switch a.pull.offer.labels[i] {
	case btnInstall:
		return a.startPull()
	case btnCopy:
		// OSC 52 sets the clipboard; terminals without it ignore it, so the
		// command stays on the All done screen too.
		a.pull.copied = true
		m, cmd := a.finish()
		return m, tea.Batch(tea.SetClipboard(source.PullCommand(a.choice)), cmd)
	}
	return a, nil
}

func (a *app) offerView() string {
	name := a.st.heading.Render(a.choice.ID)
	size := ""
	if a.choice.Size > 0 {
		size = " It's " + source.FormatBytes(uint64(a.choice.Size)) + "."
	}

	lines := []string{
		a.st.ok.Render("✓ ") + a.st.muted.Render("Saved to "+shortPath(a.savedPath)),
		"",
	}
	if a.forAnother() {
		who := "that machine"
		if a.facts.Other != nil {
			who = strings.TrimPrefix(a.facts.Other.String(), "another machine: ")
		}
		lines = append(lines,
			name+" is not installed on this machine.",
			"If "+who+" doesn't have it, run the pull command there.")
	} else if a.choice.Incomplete {
		lines = append(lines,
			name+" is incomplete: its download never finished.",
			"Do you want to download it again now?"+size+" The server can't load it until it is.")
	} else {
		lines = append(lines,
			name+" is not installed yet.",
			"Do you want to install it now?"+size+" The server can't load it until it is.")
	}

	switch {
	case a.pull.err != nil:
		lines = append(lines, "", a.st.bad.Render("Download failed: ")+a.pull.err.Error())
	case a.pull.note != "":
		lines = append(lines, "", a.st.warn.Render(a.pull.note))
	}

	box := a.st.box.BorderForeground(a.st.heading.GetForeground()).Padding(1, 2)
	pull := func(width int) string {
		return a.titledBox("Pull command", a.st.value.Render(source.PullCommand(a.choice)), width)
	}
	return a.buttonBox(box, func(width int) string {
		return a.wrapTo(strings.Join(lines, "\n"), width) + "\n\n" + pull(width)
	}, a.pull.offer, "esc skip")
}

// buttonBox draws the content in box with the buttons centered under it, and
// the key help under the box. The box fits the content and the buttons, and
// wraps only when the screen is narrower: content gets the width it may take
// (0: no limit).
func (a *app) buttonBox(box lipgloss.Style, content func(width int) string, b buttons, extra string) string {
	frame := box.GetHorizontalFrameSize()
	room := 0
	if w := a.contentWidth(); w > 0 {
		room = max(w-frame, 1)
	}

	text := content(room)
	row := a.buttonsView(b, room)
	inner := max(lipgloss.Width(text), lipgloss.Width(row))
	if room > 0 && inner > room {
		box = box.Width(room + frame)
		inner = room
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		box.Render(text+"\n\n"+lipgloss.PlaceHorizontal(inner, lipgloss.Center, row)),
		"",
		a.wrap(a.st.help.Render(a.buttonsHelp(b, extra))),
	)
}

// wrapTo breaks text at width (0: no limit).
func (a *app) wrapTo(s string, width int) string {
	if width > 0 && lipgloss.Width(s) > width {
		return lipgloss.NewStyle().Width(width).Render(s)
	}
	return s
}

func (a *app) buttonsHelp(b buttons, extra string) string {
	help := "enter select · " + extra
	if len(b.labels) > 1 {
		help = "← → choose · " + help
	}
	return help
}

// =============================================================================
// All done

// All done buttons.
const btnExit = "Exit configuration tool"

// doneLabelWidth is the width of All done's labels; values line up after it.
const doneLabelWidth = 7

// finish opens the All done screen, on Exit.
func (a *app) finish() (tea.Model, tea.Cmd) {
	again := "Configure something else"
	if !a.server {
		again = "Configure another model"
	}

	a.step = stepDone
	a.done = buttons{labels: []string{again, btnExit}, at: 1}
	if !a.server && a.src != nil && (a.choice.Installed || a.pull.pulled) {
		a.pull.dir = a.src.ModelDir(a.choice)
	}
	return a, nil
}

func (a *app) doneKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if a.canReoffer() {
			return a.offerPull()
		}
		return a, tea.Quit
	case "ctrl+c", "q":
		return a, tea.Quit
	}

	i, ok := a.done.key(msg)
	switch {
	case !ok:
		return a, nil
	case a.done.labels[i] == btnExit:
		return a, tea.Quit
	}
	return a.again()
}

// again starts over from the start form, keeping its answers: the file and
// the machine. The rest is read again, with the entry just saved.
func (a *app) again() (tea.Model, tea.Cmd) {
	*a = app{
		ctx:        a.ctx,
		src:        a.src,
		st:         a.st,
		colors:     a.colors,
		theme:      a.theme,
		spin:       a.spin,
		width:      a.width,
		height:     a.height,
		forms:      map[step]*huh.Form{},
		serverFile: a.serverFile,
		files:      a.files,
		start:      a.start,
		group:      a.group,
	}
	return a.open(stepStart)
}

func (a *app) doneView() string {
	row := func(label, value string) string {
		return a.st.label.Render(padRight(label, doneLabelWidth)) + " " + a.st.value.Render(value)
	}

	lines := []string{
		a.st.ok.Bold(true).Render("✓ All done!"),
		"",
		row("Config", shortPath(a.savedPath)),
	}
	if a.backup != "" {
		lines = append(lines, row("Backup", shortPath(a.backup)))
	}
	// An empty entry or section is not written: the defaults apply.
	empty := len(questions.Apply(a.existing, a.changes)) == 0
	switch {
	case a.server && empty:
		lines = append(lines, row("Server", "no settings in kms:, the server's defaults apply"))
	case a.server:
		lines = append(lines, row("Server", "settings under kms:"))
	case empty:
		lines = append(lines, row("Entry", "none for "+a.entryID+": Kronk's defaults apply"))
		lines = append(lines, a.modelLines(row)...)
	default:
		lines = append(lines, row("Entry", a.entryID+" under "+a.section+":"))
		lines = append(lines, a.modelLines(row)...)
	}

	lines = append(lines, "")
	switch {
	case a.forAnother():
		machine := "another machine"
		if a.facts.Other != nil {
			machine = a.facts.Other.String()
		}
		lines = append(lines,
			"This config is for "+machine+".",
			"Copy it there as ~/.kronk/models/model_config.yaml, or start that server with:",
			a.st.value.Render("  KRONK_POOL_MODEL_CONFIG_FILE=<path> kronk server start"))
	case a.savedPath == a.serverFile:
		lines = append(lines,
			"The server reads this file at startup.",
			"Restart it to apply the changes.")
	default:
		lines = append(lines,
			"The server reads "+shortPath(a.serverFile)+".",
			"To use this file, copy it there or start the server with:",
			a.st.value.Render("  KRONK_POOL_MODEL_CONFIG_FILE="+a.savedPath+" kronk server start"))
	}

	extra := "esc exit"
	if a.canReoffer() {
		extra = "esc back to install · q exit"
	}
	box := a.st.box.BorderForeground(a.st.ok.GetForeground()).Padding(1, 2)
	return a.buttonBox(box, func(int) string { return strings.Join(lines, "\n") }, a.done, extra)
}

// forAnother reports whether the config is for another machine.
func (a *app) forAnother() bool {
	return a.start != nil && a.start.forAnother()
}

// canReoffer reports whether All done can go back to the install offer: the
// saved model is still not downloaded.
func (a *app) canReoffer() bool {
	return a.needsPull() && !a.pull.pulled
}

// modelLines tell where the model is, or what to run to install it.
func (a *app) modelLines(row func(label, value string) string) []string {
	cmd := padRight("", doneLabelWidth+1) + a.st.value.Render(source.PullCommand(a.choice))

	switch {
	case a.forAnother():
		return []string{row("Model", "on that machine, if it isn't there, run:"), cmd}
	case a.pull.pulled && a.pull.dir != "":
		return []string{row("Model", "downloaded to "+shortPath(a.pull.dir))}
	case a.pull.pulled:
		return []string{row("Model", "downloaded")}
	case a.choice.Installed && a.pull.dir != "":
		return []string{row("Model", "installed in "+shortPath(a.pull.dir))}
	case a.choice.Installed:
		return []string{row("Model", "installed")}
	case a.pull.copied:
		return []string{row("Model", a.st.warn.Render("not installed yet: the pull command is copied, run it before starting the server")), cmd}
	}
	return []string{row("Model", a.st.warn.Render("not installed yet: run this before starting the server")), cmd}
}

func padRight(s string, n int) string {
	if pad := n - lipgloss.Width(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}
