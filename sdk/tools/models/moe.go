package models

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// MoEInfo contains Mixture of Experts metadata extracted from GGUF files.
type MoEInfo struct {
	IsMoE            bool
	ExpertCount      int64
	ExpertUsedCount  int64
	HasSharedExperts bool
}

// DetectMoE extracts Mixture of Experts information from GGUF metadata.
// It checks architecture-prefixed keys first, then falls back to scanning
// all metadata keys for expert-related suffixes.
func detectMoE(metadata map[string]string) MoEInfo {
	arch := detectArchitecture(metadata)

	var info MoEInfo

	// Try arch-prefixed expert_count first, then fallback to suffix scan.
	expertCount, err := parseMetadataInt64(metadata, arch+".expert_count")
	if err != nil {
		expertCount, err = scanSuffixInt64(metadata, ".expert_count")
	}
	if err == nil {
		info.ExpertCount = expertCount
	}

	// Try arch-prefixed expert_used_count first, then fallback to suffix scan.
	expertUsedCount, err := parseMetadataInt64(metadata, arch+".expert_used_count")
	if err != nil {
		expertUsedCount, _ = scanSuffixInt64(metadata, ".expert_used_count")
	}
	info.ExpertUsedCount = expertUsedCount

	// Check for shared experts via arch-prefixed key or suffix fallback.
	sharedCount, err := parseMetadataInt64(metadata, arch+".ffn_shared_expert_count")
	if err != nil {
		sharedCount, err = scanSuffixInt64(metadata, ".ffn_shared_expert_count")
	}
	if err == nil && sharedCount > 0 {
		info.HasSharedExperts = true
	}

	info.IsMoE = info.ExpertCount > 0

	return info
}

// scanSuffixInt64 scans all metadata keys for any key ending with the given
// suffix and returns the parsed int64 value of the first match.
func scanSuffixInt64(metadata map[string]string, suffix string) (int64, error) {
	var matchingKeys []string
	for k := range metadata {
		if strings.HasSuffix(k, suffix) {
			matchingKeys = append(matchingKeys, k)
		}
	}

	if len(matchingKeys) == 0 {
		return 0, fmt.Errorf("scan-suffix-int64: no key with suffix %q found", suffix)
	}

	sort.Strings(matchingKeys)

	for _, k := range matchingKeys {
		n, err := strconv.ParseInt(metadata[k], 10, 64)
		if err != nil {
			continue
		}
		return n, nil
	}

	return 0, fmt.Errorf("scan-suffix-int64: no valid int64 value for suffix %q", suffix)
}
