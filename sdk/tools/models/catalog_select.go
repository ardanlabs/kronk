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
//	.Q8_0, .Q4_K_M (mradermacher-style separator)
//
// The match is anchored to the end of the string.
var quantSuffixRe = regexp.MustCompile(`(?i)([-.](UD[-.])?(IQ|Q)\d+(_[A-Z0-9]+)*|[-.](BF16|F16|F32))$`)

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

// extractQuantTag returns just the quant tag portion of modelID
// (e.g. "UD-Q8_K_XL" from "Qwen3.6-35B-A3B-UD-Q8_K_XL"), or "" when
// modelID has no recognisable quant suffix. The leading "-" or "."
// separator is stripped so the result can be used as the tag in the
// "provider/repo:tag" resolver shape.
func extractQuantTag(modelID string) string {
	clean := resolverSplitSuffixRe.ReplaceAllString(modelID, "")
	loc := quantSuffixRe.FindStringIndex(clean)
	if loc == nil {
		return ""
	}

	tag := clean[loc[0]:loc[1]]
	if len(tag) > 0 && (tag[0] == '-' || tag[0] == '.') {
		tag = tag[1:]
	}

	return tag
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

// selectFilesByTag picks the GGUF model files (and optional F16 mmproj)
// from siblings whose model id (basename minus .gguf and any split
// suffix) ends with "-<tag>" or ".<tag>" (case-insensitive). The
// separator requirement prevents partial-tag matches — e.g. tag "Q4_K_M"
// must not match "...UD-Q4_K_XL".
//
// When multiple candidates pass the suffix filter (e.g. "Qwen-Q4_K_XL"
// and "Qwen-UD-Q4_K_XL" both match tag "Q4_K_XL"), the UD variant wins.
// Split (multi-file) models are expanded via collectSplitParts.
func selectFilesByTag(siblings []string, tag string) (files []string, mmproj string, ok bool) {
	if tag == "" {
		return nil, "", false
	}

	gguf, proj := classifySiblings(siblings)

	var candidates []string
	for _, f := range gguf {
		if fileMatchesTag(f, tag) {
			candidates = append(candidates, f)
		}
	}
	if len(candidates) == 0 {
		return nil, "", false
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return scoreCandidate(candidates[i]) > scoreCandidate(candidates[j])
	})

	target := candidates[0]

	files = collectSplitParts(gguf, target)
	if len(files) == 0 {
		files = []string{target}
	}
	sort.Strings(files)

	mmproj = pickF16Mmproj(proj, target)

	return files, mmproj, true
}

// fileMatchesTag reports whether siblingModelID(file) ends with "-<tag>"
// or ".<tag>" (case-insensitive). The leading separator anchors the
// match so tags like "Q4_K_M" don't bleed into "Q4_K_XL" filenames.
func fileMatchesTag(file, tag string) bool {
	if tag == "" {
		return false
	}

	id := strings.ToLower(siblingModelID(file))
	t := strings.ToLower(tag)

	return strings.HasSuffix(id, "-"+t) || strings.HasSuffix(id, "."+t)
}

// classifySiblings separates GGUF files from mmproj files. Non-GGUF
// siblings are dropped.
//
// Recognized mmproj naming patterns:
//
//   - Leading:  mmproj-<rest>.gguf, mmproj.gguf
//     (used by unsloth, ggml-org, and most catalog defaults)
//   - Embedded: <modelID>.mmproj-<rest>.gguf, <modelID>-mmproj-<rest>.gguf
//     (used by mradermacher and other community quantizers that prefix
//     the repo's model id onto every artifact, including the projection)
//
// The token must be delimited by a non-alphanumeric character on both
// sides so unrelated names containing "mmproj" inside another word do
// not get misclassified.
func classifySiblings(siblings []string) (gguf, proj []string) {
	for _, s := range siblings {
		if !strings.HasSuffix(strings.ToLower(s), ".gguf") {
			continue
		}

		if isMMProj(strings.ToLower(path.Base(s))) {
			proj = append(proj, s)
			continue
		}

		gguf = append(gguf, s)
	}

	return gguf, proj
}

// isMMProj reports whether base is a GGUF mmproj filename. It accepts
// both the leading form ("mmproj...") and the embedded form
// ("<id>.mmproj-..." / "<id>-mmproj-...") used by some quantizers.
//
// base must be lower-cased.
func isMMProj(base string) bool {
	const tok = "mmproj"

	idx := strings.Index(base, tok)
	if idx < 0 {
		return false
	}

	// Left boundary: start of name, or a separator.
	if idx > 0 {
		c := base[idx-1]
		if !isMMProjSep(c) {
			return false
		}
	}

	// Right boundary: end of name, or a separator.
	end := idx + len(tok)
	if end < len(base) {
		c := base[end]
		if !isMMProjSep(c) {
			return false
		}
	}

	return true
}

func isMMProjSep(c byte) bool {
	return c == '-' || c == '.' || c == '_'
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

// pickF16Mmproj returns the best mmproj sibling for the chosen model,
// or "" when none is suitable.
//
// Selection policy (in priority order):
//
//  1. F16 in the same directory as the model file.
//  2. F16 anywhere in the repo.
//  3. Highest-quality non-F16 quant (Q8, Q6, Q5, Q4, BF16, others) in the
//     same directory as the model file.
//  4. Highest-quality non-F16 quant anywhere in the repo.
//
// F16 is preferred because the projection runs at full precision in many
// templates and quantization can degrade vision/audio embedding quality.
// Falling back to a quantized projection is necessary because some
// community quantizers (mradermacher etc.) only publish quantized
// mmprojs — refusing them entirely leaves the model unable to process
// media at all.
func pickF16Mmproj(proj []string, target string) string {
	if len(proj) == 0 {
		return ""
	}

	tDir := dirSlash(target)

	var f16, others []string
	for _, p := range proj {
		base := path.Base(p)
		if f16Re.MatchString(base) {
			f16 = append(f16, p)
			continue
		}
		others = append(others, p)
	}

	if pick := pickBestInDir(f16, tDir); pick != "" {
		return pick
	}
	if len(f16) > 0 {
		sort.Strings(f16)
		return f16[0]
	}

	if pick := pickBestInDir(rankNonF16(others), tDir); pick != "" {
		return pick
	}
	if len(others) > 0 {
		sorted := rankNonF16(others)
		return sorted[0]
	}

	return ""
}

// pickBestInDir returns the first candidate whose directory matches dir,
// or "" when none match. Candidates are scanned in given order.
func pickBestInDir(candidates []string, dir string) string {
	for _, p := range candidates {
		if dirSlash(p) == dir {
			return p
		}
	}
	return ""
}

// rankNonF16 returns mmproj candidates in best-to-worst quant order.
// Used as the fallback when no F16 projection is published.
func rankNonF16(candidates []string) []string {
	out := append([]string(nil), candidates...)
	sort.SliceStable(out, func(i, j int) bool {
		return mmprojQuantScore(path.Base(out[i])) > mmprojQuantScore(path.Base(out[j]))
	})
	return out
}

// mmprojQuantScore returns a higher value for higher-precision mmproj
// quantization, so callers can rank candidates without enumerating the
// full quant set. Unknown / unparseable quant tags get the lowest score.
func mmprojQuantScore(base string) int {
	b := strings.ToLower(base)
	switch {
	case strings.Contains(b, "bf16"):
		return 90
	case strings.Contains(b, "q8"):
		return 80
	case strings.Contains(b, "q6"):
		return 60
	case strings.Contains(b, "q5"):
		return 50
	case strings.Contains(b, "q4"):
		return 40
	case strings.Contains(b, "q3"):
		return 30
	case strings.Contains(b, "q2"):
		return 20
	}
	return 0
}

// dirSlash returns the directory portion of a slash-separated path
// (no trailing slash); empty string for top-level files.
func dirSlash(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i]
	}

	return ""
}
