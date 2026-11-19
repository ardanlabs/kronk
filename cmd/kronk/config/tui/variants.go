package tui

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"charm.land/huh/v2"
	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
)

// A variant is a second set of settings for the same downloaded model, in an
// entry named model/NAME (chapter 3 of the manual). When the picked model
// already has entries, the tool asks which one to change, or for a new
// variant.

const newVariant = "\x00new"

var variantName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func (a *app) newVariantForm() *huh.Form {
	entries := source.EntriesFor(a.doc, a.choice.ID)

	var opts []huh.Option[string]
	for _, e := range entries {
		opts = append(opts, huh.NewOption("Change "+e, e))
	}
	opts = append(opts, huh.NewOption("New variant…", newVariant))

	sel := huh.NewSelect[string]().
		Title("This model already has settings in the file. Change them, or add a variant?").
		Description("A variant is another set of settings for the same model, used as " + a.choice.ID + "/NAME in requests.").
		Options(opts...).
		Value(&a.variantPick)

	name := huh.NewInput().
		Title("Variant name?").
		Description("Letters, digits, dot, dash or underscore, e.g. LONG or AGENT.").
		Validate(func(s string) error {
			s = strings.TrimSpace(s)
			switch {
			case s == "":
				return errors.New("enter a name")
			case !variantName.MatchString(s):
				return errors.New("use letters, digits, dot, dash or underscore")
			case slices.Contains(entries, a.choice.ID+"/"+s):
				return fmt.Errorf("%s/%s exists; pick it in the list to change it", a.choice.ID, s)
			}
			return nil
		}).
		Value(&a.variantName)

	return a.newForm(
		huh.NewGroup(sel),
		huh.NewGroup(name).WithHideFunc(func() bool { return a.variantPick != newVariant }),
	)
}
