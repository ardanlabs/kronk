package config

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

// A context window longer than the model's training context needs a choice
// (question E1a of the init map): lower it to the training context, or keep
// it with YaRN. YaRN writes the scaling keys chapter 7 of the manual
// describes: rope-freq-scale = training / chosen, yarn-orig-ctx = training.

const keyRopeScaling = "rope-scaling-type"

// chosenContext is the context window answered, 0 when it is left to AutoTune.
func chosenContext(a *answers) int64 {
	n, _ := strconv.ParseInt(a.value("context-window"), 10, 64)
	return n
}

// longContext reports whether the answered context is longer than the
// model's training context.
func longContext(f facts, a *answers) bool {
	train := f.Analysis.Model.TrainingContext
	return train > 0 && chosenContext(a) > train
}

// fixContext adds the changes that follow from the E1a answer.
func fixContext(f facts, a *answers, existing map[string]any, out []change) []change {
	if !longContext(f, a) {
		return out
	}

	train := f.Analysis.Model.TrainingContext

	if a.value(keyRopeScaling) == "yarn" {
		scale := math.Round(float64(train)/float64(chosenContext(a))*1e6) / 1e6
		out = setChange(out, existing, "rope-freq-scale", scale)
		return setChange(out, existing, "yarn-orig-ctx", int(train))
	}

	out = setChange(out, existing, "context-window", int(train))
	for _, key := range []string{"rope-freq-scale", "yarn-orig-ctx"} {
		out = slices.DeleteFunc(out, func(c change) bool { return c.key == key })
		if old, had := existing[key]; had {
			out = append(out, change{key: key, old: old, del: true})
		}
	}
	return out
}

// setChange replaces any change to key with one that writes v, or none when
// the file already has v.
func setChange(out []change, existing map[string]any, key string, v any) []change {
	out = slices.DeleteFunc(out, func(c change) bool { return c.key == key })

	old, had := existing[key]
	if had && fmt.Sprint(old) == fmt.Sprint(v) {
		return out
	}

	c := change{key: key, new: v}
	if had {
		c.old = old
	}
	return append(out, c)
}

// contextNote explains why AutoTune's context is shorter than the training
// context: AutoTune picks from fixed sizes (8K, 16K, 32K, ...).
func contextNote(f facts) string {
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

func reviewWarnings(warnings []string) []string {
	return slices.DeleteFunc(slices.Clone(warnings), func(w string) bool {
		return strings.HasPrefix(w, sdkContextWarning)
	})
}
