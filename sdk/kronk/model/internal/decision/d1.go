package decision

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// decisionD1Labels forms distinct single-token candidate groups. The caller
// supplies its choice-code convention beyond the shared A..Z range.
func decisionD1Labels(typ DecisionQuestionType, options []systemOneOption, choiceCode func(int) string, encode func(string) []llama.Token) ([]string, [][]llama.Token, error) {
	single := func(forms ...string) []llama.Token {
		var tokens []llama.Token
		for _, form := range forms {
			if ids := encode(form); len(ids) == 1 && !slices.Contains(tokens, ids[0]) {
				tokens = append(tokens, ids[0])
			}
		}
		return tokens
	}
	var labels []string
	var groups [][]llama.Token
	if typ != DecisionQuestionTypeChoice {
		for _, option := range options {
			group := single(option.name)
			if typ == DecisionQuestionTypeNoul {
				group = single("no", "No", "NO")
				if option.name == "true" {
					group = single("yes", "Yes", "YES")
				}
			}
			if len(group) == 0 {
				return nil, nil, fmt.Errorf("%w: label %q is not one token", ErrDecisionRequest, option.name)
			}
			labels = append(labels, option.name)
			groups = append(groups, group)
		}
		return labels, groups, nil
	}
	letters := true
	for _, option := range options {
		key := option.name
		letters = letters && len(key) == 1 && (key[0] >= 'a' && key[0] <= 'z' || key[0] >= 'A' && key[0] <= 'Z')
	}
	var used []llama.Token
	take := func(code string) bool {
		ids := encode(code)
		if len(ids) != 1 || slices.Contains(used, ids[0]) {
			return false
		}
		used = append(used, ids[0])
		labels = append(labels, code)
		groups = append(groups, single(code, " "+code))
		return true
	}
	var pool []string
	for c := 'A'; c <= 'Z'; c++ {
		pool = append(pool, string(c))
	}
	for i := range 100 {
		pool = append(pool, fmt.Sprintf("%02d", i))
	}
	for c := 'a'; c <= 'z'; c++ {
		pool = append(pool, string(c))
	}
	for i := range 200 {
		pool = append(pool, "#"+strconv.Itoa(i))
	}
	for a := 'A'; a <= 'Z'; a++ {
		for b := 'A'; b <= 'Z'; b++ {
			pool = append(pool, string([]rune{a, b}))
		}
	}
	for i, option := range options {
		code := choiceCode(i)
		if letters {
			code = option.name
		} else if len(options) <= 26 {
			code = string(rune('A' + i))
		}
		if take(code) {
			continue
		}
		found := slices.ContainsFunc(pool, take)
		if !found {
			return nil, nil, fmt.Errorf("%w: no single-token label left for %d options", ErrDecisionRequest, len(options))
		}
	}
	return labels, groups, nil
}
