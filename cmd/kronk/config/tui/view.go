package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ardanlabs/kronk/cmd/kronk/config/configfile"
	"github.com/ardanlabs/kronk/cmd/kronk/config/questions"
	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func (a *app) View() tea.View {
	var b strings.Builder

	switch a.step {
	case stepLoading, stepFacts, stepCheck, stepSaving:
		b.WriteString(a.spin.View() + " " + a.status + "…")

	case stepStart, stepSections, stepGroup, stepPick, stepVariant, stepQuestions, stepReview:
		// The open list takes the screen.
		if a.list.open {
			b.WriteString(a.panelView(a.contentWidth(), a.bodyHeight()))
			break
		}
		b.WriteString(a.aboveForm())
		b.WriteString(a.currentForm().View())

	case stepOffer:
		b.WriteString(a.offerView())

	case stepDownloading:
		b.WriteString(a.downloadView())

	case stepDone:
		b.WriteString(a.doneView())

	case stepFailed:
		b.WriteString(a.wrap(a.st.bad.Render("Error: ") + a.err.Error()))
		b.WriteString("\n\n" + a.st.help.Render("Press any key to exit."))
	}

	header := a.header()
	body := b.String()
	if a.showPanel() {
		// The answers sit beside the content, not at the far edge of a wide
		// terminal.
		left := lipgloss.NewStyle().Width(a.contentWidth()).Render(body)
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", a.panelView(a.panelWidth(), a.bodyHeight()))
	}

	v := tea.NewView(header + "\n\n" + body + "\n")
	v.AltScreen = true
	return v
}

// aboveForm is what a step with a form draws above it.
func (a *app) aboveForm() string {
	var b strings.Builder
	switch a.step {
	case stepStart, stepSections, stepGroup, stepPick, stepVariant:
		if a.formErr != nil {
			b.WriteString(a.wrap(a.st.bad.Render("Error: ") + a.formErr.Error()))
			b.WriteString("\n\n")
		}
		if a.step == stepPick {
			b.WriteString(a.pickHeading() + "\n")
		}

	case stepQuestions:
		switch {
		case a.server:
			b.WriteString(a.serverView())
		case a.choice.Kind != source.KindChatModel:
			b.WriteString(a.mediaView())
		default:
			b.WriteString(a.factsView())
		}
		b.WriteString("\n\n")

	case stepReview:
		if a.saveErr != nil {
			b.WriteString(a.wrap(a.st.bad.Render("Not saved: ") + a.saveErr.Error()))
			b.WriteString("\n\n")
		}
		// The box leaves the review form room for its menu and key help.
		room := a.bodyHeight() - lipgloss.Height(b.String()) - a.reviewFormHeight()
		b.WriteString(a.reviewView(room))
		b.WriteString("\n\n")
	}
	return b.String()
}

// bodyHeight is the number of rows under the header.
func (a *app) bodyHeight() int {
	return a.height - lipgloss.Height(a.header()) - 2
}

func (a *app) header() string {
	title := a.st.brand.Render("kronk config")
	sub := "model_config.yaml"
	if a.path != "" {
		sub = shortPath(a.path)
	}

	if a.width <= 0 {
		return title + "  " + a.st.subtitle.Render(sub)
	}

	// The header spans the answers column too. A long path keeps its end,
	// the file name, on one line.
	sub = truncateLeft(sub, a.width-lipgloss.Width(title)-2)
	return lipgloss.NewStyle().Width(a.width).Render(title + "  " + a.st.subtitle.Render(sub))
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

	origin := "installed"
	switch {
	case a.choice.Incomplete:
		origin = "incomplete download, read from the catalog"
	case !a.choice.Installed:
		origin = "catalog, not downloaded"
	}

	traits := []string{modelTraits(f)}
	if f.Projection() {
		// From the projector's header; "images or audio" when unreadable.
		traits = append(traits, "takes "+f.InputsText())
	}
	if f.MTP() {
		traits = append(traits, "MTP head")
	}

	row := func(label, value string) string {
		return a.st.label.Render(fmt.Sprintf("%-10s", label)) + " " + a.st.value.Render(value)
	}

	lines := []string{
		a.st.heading.Render(a.entryID) + "  " + a.st.muted.Render(origin),
		row("Model", fmt.Sprintf("%s · %s · %s · trained on %d tokens", m.Architecture, m.Quantization, strings.Join(traits, " · "), m.TrainingContext)),
	}

	switch {
	case f.Other != nil:
		lines = append(lines, row("Machine", f.Other.String()))
	case !f.HardwareKnown:
		lines = append(lines, row("Machine", a.st.warn.Render("unknown, "+f.HardwareErr.Error())))
	default:
		hw := fmt.Sprintf("%d GPU, %s RAM", f.GPUs(), source.FormatBytes(f.Analysis.System.SystemRAMBytes))
		if f.Analysis.System.GPUTotalBytes > 0 {
			hw = fmt.Sprintf("%d GPU (%s), %s VRAM, %s RAM", f.GPUs(), f.Analysis.System.GPUName,
				source.FormatBytes(f.Analysis.System.GPUTotalBytes), source.FormatBytes(f.Analysis.System.SystemRAMBytes))
		}
		lines = append(lines, row("Machine", hw))
	}

	auto := fmt.Sprintf("context %d · %d at a time · cache %s", r.ContextWindow, r.NSeqMax, r.CacheTypeK)
	lines = append(lines, row("AutoTune", auto))
	if f.HardwareKnown && !r.Fits {
		lines = append(lines, a.st.warn.Render("  ! "+fitWarning(r)))
	}

	if !f.Generates() {
		lines = append(lines, row("Asks", "loading and memory settings only: the model doesn't generate text (embeddings, rerank or decision)"))
	}

	if a.existing != nil {
		lines = append(lines, row("In file", a.st.warn.Render("this model has an entry; answers start from it")))
	}

	return a.boxed(lines)
}

// modelTraits describes how the model is built: dense or MoE with its
// experts, and the layers that are recurrent (hybrid) or use a sliding
// window, which change what memory it needs.
func modelTraits(f source.Facts) string {
	m := f.Analysis.Model
	parts := []string{"dense"}
	if moe := m.MoE; moe != nil {
		parts[0] = "MoE"
		if moe.ExpertCount > 0 && moe.ExpertUsedCount > 0 {
			parts[0] = fmt.Sprintf("MoE, %d of %d experts per token", moe.ExpertUsedCount, moe.ExpertCount)
		}
		if moe.HasSharedExperts {
			parts[0] += " + shared"
		}
	}

	layers := func(n int64, what string) string {
		if n > 0 && m.BlockCount > 0 {
			return fmt.Sprintf("%s on %d of %d layers", what, n, m.BlockCount)
		}
		return what
	}
	if n := m.Attention.RecurrentLayers; n > 0 {
		parts = append(parts, layers(n, "hybrid: recurrent"))
	}
	if f.SWA() {
		parts = append(parts, layers(m.Attention.SlidingWindowLayers, "sliding window"))
	}
	return strings.Join(parts, " · ")
}

func (a *app) mediaView() string {
	what := "Audio model (whisper.cpp)"
	if a.choice.Kind == source.KindImageModel {
		what = "Image model (stable-diffusion.cpp)"
	}
	if !a.choice.Installed {
		what += ", catalog, not downloaded"
	}
	return a.boxed([]string{
		a.st.heading.Render(a.choice.ID) + "  " + a.st.muted.Render(what),
		a.st.value.Render("Written under " + a.section + ": in " + shortPath(a.path) + "."),
	})
}

func (a *app) serverView() string {
	return a.boxed([]string{
		a.st.heading.Render("Server settings") + "  " + a.st.muted.Render("kms: in "+shortPath(a.path)),
		a.st.value.Render("Environment variables and kronk server start flags override these values."),
	})
}

// reviewView draws the review in a box at most maxHeight rows tall, when
// maxHeight is above 0.
func (a *app) reviewView(maxHeight int) string {
	var lines []string
	if a.server {
		lines = append(lines, a.st.heading.Render("Server settings"))
	} else {
		lines = append(lines, a.st.heading.Render(a.entryID))
	}

	if len(a.changes) == 0 {
		lines = append(lines, a.st.muted.Render("  No changes."))
	}

	for _, c := range a.changes {
		switch {
		case c.Del:
			lines = append(lines, a.st.removed.Render(fmt.Sprintf("  - %s: %s", c.Key, configfile.InlineYAML(c.Old))))
		case c.Old != nil:
			lines = append(lines, a.st.warn.Render(fmt.Sprintf("  ~ %s: %s → %s", c.Key, configfile.InlineYAML(c.Old), configfile.InlineYAML(c.New))))
		default:
			lines = append(lines, a.st.added.Render(fmt.Sprintf("  + %s: %s", c.Key, configfile.InlineYAML(c.New))))
		}
	}

	head := len(lines)
	label := "Entry as written"
	if a.server {
		label = "Section as written"
	}
	lines = append(lines, "", a.st.label.Render(label))

	entry, err := a.preview()
	switch {
	case err != nil:
		lines = append(lines, a.st.bad.Render("  "+err.Error()))
	case entry == "":
		// configfile.SaveEntry removes an empty entry instead of writing it as {}.
		lines = append(lines, a.st.muted.Render("  Nothing for this model: Kronk's defaults and AutoTune apply."))
	default:
		for l := range strings.Lines(strings.TrimRight(entry, "\n")) {
			lines = append(lines, a.st.value.Render("  "+strings.TrimRight(l, "\n")))
		}
		if a.server && len(questions.Apply(a.existing, a.changes)) == 0 {
			lines = append(lines, a.st.muted.Render("  The server uses its defaults."))
		}
	}
	end := len(lines)

	switch {
	case len(a.problems) > 0:
		lines = append(lines, "")
		for _, p := range a.problems {
			lines = append(lines, a.st.bad.Render("  ✗ "+p))
		}

	case a.server || a.choice.Kind != source.KindChatModel:
		// Server settings and audio or image models have no memory check.

	case a.check != nil:
		r := a.check.Recommended
		lines = append(lines, "", a.st.label.Render("Memory    ")+fmt.Sprintf("context %d · %d at a time · cache %s · %s", r.ContextWindow, r.NSeqMax, r.CacheTypeK, fitText(r)))
		for _, w := range questions.ReviewWarnings(a.check.Warnings) {
			lines = append(lines, a.st.warn.Render("  ! "+w))
		}

	case a.checkErr != nil:
		lines = append(lines, "", a.st.warn.Render("Memory check failed: "+a.checkErr.Error()))

	default:
		lines = append(lines, "", a.st.muted.Render("Memory check skipped: "+source.ErrNoLibs.Error()))
	}

	return a.fitBox(maxHeight, lines, head, end)
}

// previewCache is the last entry or section preview, for the file it was
// made against. The review draws on every frame; the preview reads the file.
type previewCache struct {
	ok        bool
	other     bool
	otherPath string
	text      string
	err       error
}

// preview is the entry or section as the save would write it. It runs the
// same edit as the save on the same file, so it shows the lines exactly as
// they will be written. Saving to another file replaces the entry or section
// there with the whole result, so the preview is made against that file.
func (a *app) preview() (string, error) {
	other := a.action == actOther
	if c := a.prev; c.ok && c.other == other && c.otherPath == a.otherPath {
		return c.text, c.err
	}

	values := questions.Apply(a.existing, a.changes)
	otherPath := expandHome(a.otherPath)
	var text string
	var err error
	switch {
	case a.server && other:
		text, err = configfile.PreviewKMSValues(otherPath, values)
	case a.server:
		text, err = configfile.PreviewKMS(a.path, a.changes)
	case other:
		text, err = configfile.PreviewEntryValues(otherPath, a.section, a.entryID, values)
	default:
		text, err = configfile.PreviewEntry(a.path, a.section, a.entryID, a.changes)
	}

	a.prev = previewCache{ok: true, other: other, otherPath: a.otherPath, text: text, err: err}
	return text, err
}

// fitBox draws the review lines boxed in at most maxHeight rows. The entry as
// written, lines[head:end], goes first: it repeats the changes. Then the
// changes are cut short. The heading and what follows the entry stay.
func (a *app) fitBox(maxHeight int, lines []string, head, end int) string {
	box := a.boxed(lines)
	if maxHeight <= 0 || lipgloss.Height(box) <= maxHeight {
		return box
	}

	changes, tail := lines[1:head], lines[end:]
	hidden := a.st.muted.Render("  The entry as written is hidden: the terminal is too short.")
	for n := len(changes); n >= 0; n-- {
		shown := changes[:n]
		if n < len(changes) {
			more := fmt.Sprintf("  … %d more, ctrl+o lists the answers", len(changes)-n)
			shown = append(slices.Clone(shown), a.st.muted.Render(more))
		}
		box = a.boxed(slices.Concat(lines[:1], shown, []string{"", hidden}, tail))
		if lipgloss.Height(box) <= maxHeight {
			break
		}
	}
	return box
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
	return min(a.formWidth(), 110)
}

// =============================================================================

// fitWarning warns that AutoTune's pick may not fit the machine the config is
// for. The review checks the answers themselves and reports the detail.
func fitWarning(r models.RuntimeRecommendation) string {
	mem := source.FormatBytes(uint64(max(r.EstimatedVRAMBytes, 0)))
	return "May not fully fit in the target GPU (needs " + mem + ")"
}

func fitText(r models.RuntimeRecommendation) string {
	mem := source.FormatBytes(uint64(max(r.EstimatedVRAMBytes, 0)))
	if r.Fits {
		return "fits (" + mem + ")"
	}
	// The SDK puts the profile's description before the reason it doesn't
	// fit; only the reason is useful here.
	if _, reason, ok := strings.Cut(r.Reason, "WARNING: "); ok {
		return "does not fit (" + mem + "): " + reason
	}
	if r.Reason != "" {
		return "does not fit (" + mem + "): " + r.Reason
	}
	return "does not fit (" + mem + ")"
}
