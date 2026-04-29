package models

import (
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// quantSuffixRe matches a trailing quant tag on a GGUF model id, e.g.:
//
//	-Q4_K_M, -Q5_K_S, -IQ3_M, -UD-Q4_K_M, -BF16, -F16, -F32
//
// The match is anchored to the end of the string.
var quantSuffixRe = regexp.MustCompile(`(?i)(-(UD-)?(IQ|Q)\d+(_[A-Z0-9]+)*|-(BF16|F16|F32))$`)

// resolverSplitSuffixRe matches the "-NNNNN-of-NNNNN" GGUF split suffix.
var resolverSplitSuffixRe = regexp.MustCompile(`-\d+-of-\d+$`)

// resolverSplitPartsRe captures the part and total from a split suffix.
var resolverSplitPartsRe = regexp.MustCompile(`-(\d+)-of-(\d+)$`)

// f16Re matches a standalone F16 quant tag in a filename, rejecting BF16.
var f16Re = regexp.MustCompile(`(?i)(^|[^a-z])f16([^a-z0-9]|$)`)

// stripQuantSuffix removes a trailing quant tag (and any split suffix
// before it) from a model id, yielding the model "family" used as the
// HuggingFace search query.
func stripQuantSuffix(modelID string) string {
	out := resolverSplitSuffixRe.ReplaceAllString(modelID, "")
	out = quantSuffixRe.ReplaceAllString(out, "")
	return out
}

// hasQuantSuffix reports whether modelID already carries a quant tag.
func hasQuantSuffix(modelID string) bool {
	clean := resolverSplitSuffixRe.ReplaceAllString(modelID, "")
	return quantSuffixRe.MatchString(clean)
}

// selectFiles picks the GGUF model files (and optional F16 mmproj) that
// match a requested modelID from a list of repo-relative sibling paths.
//
// Matching rules:
//
//   - Exact: a sibling whose basename equals "<modelID>.gguf" or whose
//     model id (basename minus extension and split suffix) equals modelID
//     case-insensitively. If the matched file is a split member, every
//     part is returned.
//   - No quant in input: try "<modelID>-UD-Q4_K_M" first, then
//     "<modelID>-Q4_K_M".
//   - mmproj: pick a sibling matching mmproj*F16*.gguf for the chosen
//     model; preferred in this order: same directory + matching base id,
//     same directory, any matching F16 mmproj, any F16 mmproj.
func selectFiles(siblings []string, modelID string) (files []string, mmproj string, ok bool) {
	gguf, proj := classifySiblings(siblings)

	target, matched := matchModel(gguf, modelID)
	if !matched {
		if !hasQuantSuffix(modelID) {
			if t, m := matchModel(gguf, modelID+"-UD-Q4_K_M"); m {
				target, matched = t, m
			} else if t, m := matchModel(gguf, modelID+"-Q4_K_M"); m {
				target, matched = t, m
			}
		}
	}
	if !matched {
		return nil, "", false
	}

	files = collectSplitParts(gguf, target)
	if len(files) == 0 {
		files = []string{target}
	}
	sort.Strings(files)

	mmproj = pickF16Mmproj(proj, target)

	return files, mmproj, true
}

// classifySiblings separates GGUF files from mmproj files. Non-GGUF
// siblings are dropped.
func classifySiblings(siblings []string) (gguf, proj []string) {
	for _, s := range siblings {
		if !strings.HasSuffix(strings.ToLower(s), ".gguf") {
			continue
		}

		base := strings.ToLower(path.Base(s))
		if strings.HasPrefix(base, "mmproj") {
			proj = append(proj, s)
			continue
		}

		gguf = append(gguf, s)
	}

	return gguf, proj
}

// matchModel finds the sibling whose model id (basename without .gguf
// extension and without split suffix) matches modelID. When two
// candidates match (e.g. UD- and non-UD variants share the same id
// after stripping), the UD- one wins.
func matchModel(gguf []string, modelID string) (string, bool) {
	target := strings.ToLower(modelID)

	var candidates []string
	for _, f := range gguf {
		if strings.EqualFold(siblingModelID(f), target) {
			candidates = append(candidates, f)
		}
	}

	switch len(candidates) {
	case 0:
		return "", false
	case 1:
		return candidates[0], true
	default:
		// Multiple candidates with the same model id but different
		// directories or casings. Prefer ones whose basename matches
		// (without quirks) and that contain "UD-" in the path.
		sort.SliceStable(candidates, func(i, j int) bool {
			return scoreCandidate(candidates[i]) > scoreCandidate(candidates[j])
		})
		return candidates[0], true
	}
}

// scoreCandidate ranks otherwise-equal model files. Higher is better.
func scoreCandidate(f string) int {
	base := strings.ToLower(path.Base(f))
	score := 0

	if strings.Contains(base, "ud-") {
		score += 10
	}

	if !strings.Contains(f, "/") {
		score += 1 // prefer top-level files
	}

	return score
}

// siblingModelID returns the canonical model id for a sibling path:
// the basename with its .gguf extension and any split suffix stripped.
func siblingModelID(s string) string {
	base := path.Base(s)
	if strings.HasSuffix(strings.ToLower(base), ".gguf") {
		base = base[:len(base)-len(".gguf")]
	}

	return resolverSplitSuffixRe.ReplaceAllString(base, "")
}

// collectSplitParts returns every sibling that is part of the same
// split set as target. When target is not a split file the result is
// empty (caller substitutes [target]).
func collectSplitParts(gguf []string, target string) []string {
	tID := siblingModelID(target)
	tDir := dirSlash(target)

	var parts []string
	var totals []int
	for _, f := range gguf {
		if dirSlash(f) != tDir {
			continue
		}

		if !strings.EqualFold(siblingModelID(f), tID) {
			continue
		}

		base := f
		if strings.HasSuffix(strings.ToLower(base), ".gguf") {
			base = base[:len(base)-len(".gguf")]
		}

		m := resolverSplitPartsRe.FindStringSubmatch(base)
		if m == nil {
			continue
		}

		parts = append(parts, f)
		if t, err := strconv.Atoi(m[2]); err == nil {
			totals = append(totals, t)
		}
	}

	if len(parts) == 0 {
		return nil
	}

	// Validate: every total agrees, and we have N files.
	for _, t := range totals {
		if t != totals[0] {
			return nil
		}
	}

	if len(parts) != totals[0] {
		return nil
	}

	return parts
}

// pickF16Mmproj returns the best F16 mmproj sibling for the chosen
// model, or "" when none is suitable.
func pickF16Mmproj(proj []string, target string) string {
	if len(proj) == 0 {
		return ""
	}

	tDir := dirSlash(target)

	// Filter to F16 candidates only. Use a regex that rejects BF16.
	var f16 []string
	for _, p := range proj {
		base := path.Base(p)
		if f16Re.MatchString(base) {
			f16 = append(f16, p)
		}
	}

	if len(f16) == 0 {
		return ""
	}

	// Prefer same-directory matches.
	for _, p := range f16 {
		if dirSlash(p) == tDir {
			return p
		}
	}

	// Otherwise fall back to the first F16 mmproj available.
	sort.Strings(f16)
	return f16[0]
}

// dirSlash returns the directory portion of a slash-separated path
// (no trailing slash); empty string for top-level files.
func dirSlash(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}

	return ""
}
