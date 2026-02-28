package catalog

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// HFResolvedFiles contains the resolved file paths for a HuggingFace
// shorthand input like "owner/repo:TAG" or "owner/repo:TAG@revision".
type HFResolvedFiles struct {
	Owner      string
	Repo       string
	ModelFiles []string // full paths like "owner/repo/filename.gguf"
	ProjFile   string   // full path like "owner/repo/mmproj-file.gguf" (empty if none)
	Tag        string
	Revision   string // branch/revision, defaults to "main"
}

// splitSuffixRe matches the "-NNNNN-of-NNNNN" suffix on split GGUF files.
var splitSuffixRe = regexp.MustCompile(`-\d+-of-\d+$`)

// splitPartsRe captures the part and total from a split suffix.
var splitPartsRe = regexp.MustCompile(`-(\d+)-of-(\d+)$`)

// ResolveHuggingFaceShorthand resolves a HuggingFace shorthand reference
// (e.g. "owner/repo:Q4_K_M") into concrete file paths. The returned bool
// is true when the input was recognised as shorthand and resolved. When
// false the caller should fall back to existing URL/path handling.
func ResolveHuggingFaceShorthand(ctx context.Context, input string) (HFResolvedFiles, bool, error) {
	resolved, _, ok, err := resolveHFShorthandInternal(ctx, input)
	return resolved, ok, err
}

// resolveHFShorthandInternal is the internal implementation that also
// returns the fetched model metadata so callers like LookupHuggingFace
// can avoid a redundant API call.
func resolveHFShorthandInternal(ctx context.Context, input string) (HFResolvedFiles, hfModelMeta, bool, error) {
	owner, repo, tag, revision, ok := parseShorthand(input)
	if !ok {
		return HFResolvedFiles{}, hfModelMeta{}, false, nil
	}

	// Use model meta siblings for a complete recursive file listing.
	// The tree API only returns top-level entries and would miss split
	// models stored in subdirectories.
	meta, err := fetchHFModelMeta(ctx, owner, repo, revision)
	if err != nil {
		return HFResolvedFiles{}, hfModelMeta{}, true, fmt.Errorf("resolve-hf-shorthand: %w", err)
	}

	var repoFiles []HFRepoFile
	for _, s := range meta.Siblings {
		repoFiles = append(repoFiles, HFRepoFile{Filename: s.RFilename})
	}

	ggufFiles, projFiles := classifyGGUFFiles(repoFiles)

	candidates := matchByTag(ggufFiles, tag)
	if len(candidates) == 0 {
		const maxShow = 20
		var available []string
		for i, f := range ggufFiles {
			if i >= maxShow {
				available = append(available, fmt.Sprintf("(and %d more)", len(ggufFiles)-maxShow))
				break
			}
			available = append(available, f.Filename)
		}
		return HFResolvedFiles{}, hfModelMeta{}, true, fmt.Errorf("resolve-hf-shorthand: no GGUF files matching tag %q found in %s/%s, available: %v", tag, owner, repo, available)
	}

	groups := groupByModelID(candidates)
	if len(groups) > 1 {
		ids := make([]string, 0, len(groups))
		for id := range groups {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return HFResolvedFiles{}, hfModelMeta{}, true, fmt.Errorf("resolve-hf-shorthand: tag %q is ambiguous in %s/%s, matches multiple models: %v — use explicit URL instead", tag, owner, repo, ids)
	}

	var mID string
	var matched []HFRepoFile
	for id, files := range groups {
		mID = id
		matched = files
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].Filename < matched[j].Filename
	})

	if err := validateSplitCompleteness(matched); err != nil {
		return HFResolvedFiles{}, hfModelMeta{}, true, fmt.Errorf("resolve-hf-shorthand: %w", err)
	}

	// Build download paths. When using the default "main" branch, use
	// the short form (owner/repo/file) which NormalizeHuggingFaceDownloadURL
	// converts. For custom revisions, build full URLs directly.
	modelPaths := make([]string, len(matched))
	for i, f := range matched {
		if revision == "main" {
			modelPaths[i] = fmt.Sprintf("%s/%s/%s", owner, repo, f.Filename)
		} else {
			modelPaths[i] = fmt.Sprintf("https://huggingface.co/%s/%s/resolve/%s/%s", owner, repo, url.PathEscape(revision), f.Filename)
		}
	}

	projFile := detectProjection(projFiles, mID, tag)
	var projPath string
	if projFile != "" {
		if revision == "main" {
			projPath = fmt.Sprintf("%s/%s/%s", owner, repo, projFile)
		} else {
			projPath = fmt.Sprintf("https://huggingface.co/%s/%s/resolve/%s/%s", owner, repo, url.PathEscape(revision), projFile)
		}
	}

	return HFResolvedFiles{
		Owner:      owner,
		Repo:       repo,
		ModelFiles: modelPaths,
		ProjFile:   projPath,
		Tag:        tag,
		Revision:   revision,
	}, meta, true, nil
}

// =============================================================================

// parseShorthand attempts to parse the input as a HuggingFace shorthand
// reference. Returns owner, repo, tag, revision, and whether the input
// was recognised as shorthand.
func parseShorthand(input string) (owner, repo, tag, revision string, ok bool) {
	input = strings.TrimSpace(input)

	// Quick rejection: if it contains markers of a full URL/path, it's not
	// shorthand.
	lower := strings.ToLower(input)
	if strings.Contains(lower, ".gguf") || strings.Contains(lower, "/resolve/") || strings.Contains(lower, "/blob/") {
		return "", "", "", "", false
	}

	// Strip scheme and host prefixes.
	for _, prefix := range []string{
		"https://huggingface.co/",
		"http://huggingface.co/",
		"https://hf.co/",
		"http://hf.co/",
		"huggingface.co/",
		"hf.co/",
	} {
		if strings.HasPrefix(lower, prefix) {
			input = input[len(prefix):]
			break
		}
	}

	// After stripping, we expect "owner/repo:TAG" or "owner/repo:TAG@revision".
	// More than 2 path segments means it's not shorthand.
	before, after, ok0 := strings.Cut(input, ":")
	if !ok0 {
		return "", "", "", "", false
	}

	pathPart := before
	segments := strings.Split(pathPart, "/")
	if len(segments) != 2 {
		return "", "", "", "", false
	}

	owner = segments[0]
	repo = segments[1]
	if owner == "" || repo == "" {
		return "", "", "", "", false
	}

	tagRevision := after
	if tagRevision == "" {
		return "", "", "", "", false
	}

	// Split tag@revision.
	revision = "main"
	if atIdx := strings.Index(tagRevision, "@"); atIdx >= 0 {
		revision = tagRevision[atIdx+1:]
		tagRevision = tagRevision[:atIdx]
		if revision == "" {
			revision = "main"
		}
	}
	tag = tagRevision

	if tag == "" {
		return "", "", "", "", false
	}

	return owner, repo, tag, revision, true
}

// classifyGGUFFiles separates GGUF files into model files and projection
// (mmproj) files.
func classifyGGUFFiles(files []HFRepoFile) (gguf, proj []HFRepoFile) {
	for _, f := range files {
		if !strings.HasSuffix(strings.ToLower(f.Filename), ".gguf") {
			continue
		}
		if isProjFile(f.Filename) {
			proj = append(proj, f)
		} else {
			gguf = append(gguf, f)
		}
	}
	return gguf, proj
}

// isProjFile returns true if the filename represents a projection file.
func isProjFile(path string) bool {
	return strings.HasPrefix(strings.ToLower(baseName(path)), "mmproj")
}

// matchByTag returns files whose base name contains the tag (case-insensitive).
func matchByTag(files []HFRepoFile, tag string) []HFRepoFile {
	lowerTag := strings.ToLower(tag)

	var matched []HFRepoFile
	for _, f := range files {
		base := strings.ToLower(baseName(f.Filename))
		if strings.Contains(base, lowerTag) {
			matched = append(matched, f)
		}
	}

	return matched
}

// groupByModelID groups files by their model ID, which is the base name
// with the .gguf extension and any split suffix removed. Grouping is
// case-insensitive to avoid false ambiguity when filenames differ only
// by casing.
func groupByModelID(files []HFRepoFile) map[string][]HFRepoFile {
	groups := make(map[string][]HFRepoFile)
	for _, f := range files {
		id := strings.ToLower(modelID(f.Filename))
		groups[id] = append(groups[id], f)
	}
	return groups
}

// modelID returns the canonical model identifier for a filename by
// stripping the .gguf extension and any split suffix. The directory
// path is preserved to avoid collisions between files in different
// subdirectories with the same base name.
func modelID(filename string) string {
	lower := strings.ToLower(filename)
	if strings.HasSuffix(lower, ".gguf") {
		filename = filename[:len(filename)-len(".gguf")]
	}
	return splitSuffixRe.ReplaceAllString(filename, "")
}

// validateSplitCompleteness checks that split model files have all parts
// present. For non-split files (single file) this is a no-op. When
// multiple files are present, they must all be split parts with
// consistent totals.
func validateSplitCompleteness(files []HFRepoFile) error {
	if len(files) <= 1 {
		return nil
	}

	parts := make(map[int]bool)
	var total int
	var splitCount int

	for _, f := range files {
		s := f.Filename
		lower := strings.ToLower(s)
		if strings.HasSuffix(lower, ".gguf") {
			s = s[:len(s)-len(".gguf")]
		}

		m := splitPartsRe.FindStringSubmatch(s)
		if m == nil {
			continue
		}

		splitCount++

		partNum, _ := strconv.Atoi(m[1])
		partTotal, _ := strconv.Atoi(m[2])

		if total == 0 {
			total = partTotal
		} else if total != partTotal {
			return fmt.Errorf("split model has inconsistent totals: saw %d and %d", total, partTotal)
		}

		parts[partNum] = true
	}

	if splitCount == 0 {
		return fmt.Errorf("multiple candidate files matched but none are split parts; use explicit URL instead")
	}

	if splitCount != len(files) {
		return fmt.Errorf("split model set contains non-split files (files=%d, split parts=%d); use explicit URL instead", len(files), splitCount)
	}

	// Determine numbering scheme: 0-based (0..total-1) or 1-based (1..total).
	startIdx := 1
	if parts[0] {
		startIdx = 0
	}

	var missing []int
	for i := startIdx; i < startIdx+total; i++ {
		if !parts[i] {
			missing = append(missing, i)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("split model is incomplete: expected %d parts, missing parts %v", total, missing)
	}

	return nil
}

// detectProjection finds a matching mmproj file for the resolved model.
// It compares projection file model IDs (after stripping the mmproj
// prefix) against the selected model's base name ID. When multiple
// candidates match, it prefers projections in the same directory as
// the model files, then those whose name contains the tag.
func detectProjection(projFiles []HFRepoFile, selectedModelID, tag string) string {
	if len(projFiles) == 0 {
		return ""
	}

	// Extract just the base model ID (without directory) for comparison
	// since projection files use "mmproj-<baseModelID>" naming.
	selectedBaseID := baseName(selectedModelID)
	selectedDir := dirName(selectedModelID)

	var matched []HFRepoFile
	for _, f := range projFiles {
		projID := projBaseModelID(f.Filename)
		if strings.EqualFold(projID, selectedBaseID) {
			matched = append(matched, f)
		}
	}

	if len(matched) == 0 {
		// Fallback: check if any projection file contains the tag.
		lowerTag := strings.ToLower(tag)
		for _, f := range projFiles {
			if strings.Contains(strings.ToLower(baseName(f.Filename)), lowerTag) {
				return f.Filename
			}
		}
		return ""
	}

	if len(matched) == 1 {
		return matched[0].Filename
	}

	// Multiple matches — prefer same directory + tag, then same directory,
	// then tag, then first match.
	lowerTag := strings.ToLower(tag)

	for _, f := range matched {
		if dirName(f.Filename) == selectedDir && strings.Contains(strings.ToLower(baseName(f.Filename)), lowerTag) {
			return f.Filename
		}
	}
	for _, f := range matched {
		if dirName(f.Filename) == selectedDir {
			return f.Filename
		}
	}
	for _, f := range matched {
		if strings.Contains(strings.ToLower(baseName(f.Filename)), lowerTag) {
			return f.Filename
		}
	}
	return matched[0].Filename
}

// projBaseModelID extracts the base model ID from a projection filename
// by stripping the mmproj prefix from the basename after removing .gguf
// and split suffixes.
func projBaseModelID(filename string) string {
	base := baseName(filename)
	lower := strings.ToLower(base)
	if strings.HasSuffix(lower, ".gguf") {
		base = base[:len(base)-len(".gguf")]
	}
	base = splitSuffixRe.ReplaceAllString(base, "")

	// Strip mmproj prefix (with separator).
	lowerBase := strings.ToLower(base)
	for _, prefix := range []string{"mmproj-", "mmproj_", "mmproj"} {
		if strings.HasPrefix(lowerBase, prefix) {
			base = base[len(prefix):]
			break
		}
	}
	return base
}

// baseName returns the last element of a slash-separated path.
func baseName(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// dirName returns the directory portion of a slash-separated path,
// or an empty string if there is no directory component.
func dirName(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i]
	}
	return ""
}

// repoRelativePath extracts the repo-relative file path from a resolved
// model reference. For short form "owner/repo/path/file.gguf" it strips
// "owner/repo/". For full URLs it extracts the path after "/resolve/<rev>/".
func repoRelativePath(ref, owner, repo string) string {
	if strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "http://") {
		if _, after, ok := strings.Cut(ref, "/resolve/"); ok {
			rest := after
			if _, after, ok := strings.Cut(rest, "/"); ok {
				return after
			}
		}
		return baseName(ref)
	}

	prefix := owner + "/" + repo + "/"
	if strings.HasPrefix(ref, prefix) {
		return ref[len(prefix):]
	}
	return baseName(ref)
}
