package questions

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/cmd/kronk/config/configfile"
	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
)

// A context window longer than the model's training context needs a choice:
// lower it to the training context, or keep it with YaRN. YaRN writes the
// scaling keys chapter 7 of the manual describes: rope-freq-scale = training
// / chosen, yarn-orig-ctx = training.

// KeyRopeScaling is the key of the YaRN question.
const KeyRopeScaling = "rope-scaling-type"

// chosenContext is the context window answered, 0 when it is left to AutoTune.
func chosenContext(a *Answers) int64 {
	n, _ := strconv.ParseInt(a.Value("context-window"), 10, 64)
	return n
}

// longContext reports whether the answered context is longer than the
// model's training context.
func longContext(f source.Facts, a *Answers) bool {
	train := f.Analysis.Model.TrainingContext
	return train > 0 && chosenContext(a) > train
}

// fixContext adds the changes that follow from the YaRN answer.
func fixContext(f source.Facts, a *Answers, existing map[string]any, out []configfile.Change) []configfile.Change {
	if !longContext(f, a) {
		return out
	}

	train := f.Analysis.Model.TrainingContext

	if a.Value(KeyRopeScaling) == "yarn" {
		scale := math.Round(float64(train)/float64(chosenContext(a))*1e6) / 1e6
		out = setChange(out, existing, "rope-freq-scale", scale)
		return setChange(out, existing, "yarn-orig-ctx", int(train))
	}

	out = setChange(out, existing, "context-window", int(train))
	for _, key := range []string{"rope-freq-scale", "yarn-orig-ctx"} {
		out = slices.DeleteFunc(out, func(c configfile.Change) bool { return c.Key == key })
		if old, had := existing[key]; had {
			out = append(out, configfile.Change{Key: key, Old: old, Del: true})
		}
	}
	return out
}

// setChange replaces any change to key with one that writes v, or none when
// the file already has v.
func setChange(out []configfile.Change, existing map[string]any, key string, v any) []configfile.Change {
	out = slices.DeleteFunc(out, func(c configfile.Change) bool { return c.Key == key })

	old, had := existing[key]
	if had && sameValue(old, v) {
		return out
	}

	c := configfile.Change{Key: key, New: v}
	if had {
		c.Old = old
	}
	return append(out, c)
}

// ContextNote explains why AutoTune's context is shorter than the training
// context: AutoTune picks from fixed sizes (8K, 16K, 32K, ...).
func ContextNote(f source.Facts) string {
	train, picked := f.Analysis.Model.TrainingContext, f.Analysis.Recommended.ContextWindow
	if train <= 0 || picked <= 0 || picked >= train {
		return ""
	}
	return fmt.Sprintf("AutoTune picks %d here; the model was trained on %d. Pick Training max for the full length.", picked, train)
}

// sdkContextWarning starts the analysis warning about a context below the
// training context. The questions already say it, so the review leaves it
// out; TestContextWarningPrefix fails when the SDK text changes.
const sdkContextWarning = "Context window capped to "

// ReviewWarnings are the analysis warnings the review shows.
func ReviewWarnings(warnings []string) []string {
	return slices.DeleteFunc(slices.Clone(warnings), func(w string) bool {
		return strings.HasPrefix(w, sdkContextWarning)
	})
}
