package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
)

// The picker first asks for a group (chat, embeddings, audio, ...), then
// lists that group's models, installed first. All models lists every group,
// to search across them.

// groupAll is the group choice that lists every model.
const groupAll = "all"

// groupLabels name the groups in the picker.
var groupLabels = map[source.Group]string{
	source.GroupChat:       "Chat",
	source.GroupEmbeddings: "Embeddings",
	source.GroupRerank:     "Rerank",
	source.GroupDecision:   "Decision",
	source.GroupAudio:      "Audio (whisper.cpp)",
	source.GroupImage:      "Image (stable-diffusion.cpp)",
}

// groupCount is how many models a group has, and how many are installed.
type groupCount struct {
	total, installed int
}

// groups lists the groups that have models, in source.Groups order, with
// their counts.
func (a *app) groups() ([]source.Group, map[source.Group]groupCount) {
	counts := map[source.Group]groupCount{}
	for _, c := range a.choices {
		n := counts[c.Group()]
		n.total++
		if c.Installed {
			n.installed++
		}
		counts[c.Group()] = n
	}

	var out []source.Group
	for _, g := range source.Groups {
		if counts[g].total > 0 {
			out = append(out, g)
		}
	}
	return out, counts
}

// openGroups asks for the group, or goes straight to the models when they
// are all in one group.
func (a *app) openGroups() (tea.Model, tea.Cmd) {
	groups, _ := a.groups()
	if len(groups) == 1 {
		a.group, a.groupAsked = string(groups[0]), false
		return a.open(stepPick)
	}

	a.groupAsked = true
	if a.group == "" {
		a.group = string(groups[0])
	}
	return a.open(stepGroup)
}

func (a *app) newGroupForm() *huh.Form {
	groups, counts := a.groups()

	width := 0
	for _, g := range groups {
		width = max(width, len(groupLabels[g]))
	}

	var opts []huh.Option[string]
	for _, g := range groups {
		n := counts[g]
		noun := "models"
		if n.total == 1 {
			noun = "model"
		}
		count := fmt.Sprintf("%3d %s", n.total, noun)
		if n.installed > 0 {
			count += fmt.Sprintf(" · %d installed", n.installed)
		}
		label := fmt.Sprintf("%-*s  ", width, groupLabels[g]) + a.st.muted.Render(count)
		opts = append(opts, huh.NewOption(label, string(g)))
	}
	opts = append(opts, huh.NewOption("All models "+a.st.muted.Render("(search everything)"), groupAll))

	desc := "Each group lists the installed models first, then the catalog."

	sel := huh.NewSelect[string]().
		Title("Which kind of model do you want to configure?").
		Description(desc).
		Options(opts...).
		Value(&a.group)

	return a.newForm(huh.NewGroup(sel))
}

func (a *app) groupDone() (tea.Model, tea.Cmd) {
	a.picked = ""
	return a.open(stepPick)
}

// pickChoices are the models of the group picked.
func (a *app) pickChoices() []source.Choice {
	if a.group == groupAll || a.group == "" {
		return a.choices
	}

	var out []source.Choice
	for _, c := range a.choices {
		if string(c.Group()) == a.group {
			out = append(out, c)
		}
	}
	return out
}

// groupLine names the group the list shows, with its numbers, e.g.
// "Chat · 28 models · 1 installed".
func (a *app) groupLine() string {
	choices := a.pickChoices()
	name := "All models"
	if label, ok := groupLabels[source.Group(a.group)]; ok {
		name = label
	}

	var installed int
	for _, c := range choices {
		if c.Installed {
			installed++
		}
	}

	parts := []string{fmt.Sprintf("%d models", len(choices))}
	if len(choices) == 1 {
		parts[0] = "1 model"
	}
	if installed > 0 {
		parts = append(parts, fmt.Sprintf("%d installed", installed))
	}

	return a.st.heading.Render(name) + a.st.muted.Render(" · "+strings.Join(parts, " · "))
}

// groupNouns name a group's models in the picker's question.
var groupNouns = map[source.Group]string{
	source.GroupChat:       "chat",
	source.GroupEmbeddings: "embedding",
	source.GroupRerank:     "rerank",
	source.GroupDecision:   "decision",
	source.GroupAudio:      "audio",
	source.GroupImage:      "image",
}

// pickQuestion is the picker's question for the group picked.
func (a *app) pickQuestion() string {
	if noun, ok := groupNouns[source.Group(a.group)]; ok && a.groupAsked {
		return "Which " + noun + " model do you want to configure?"
	}
	return "Which model do you want to configure?"
}
