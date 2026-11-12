package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const workloadVersion = "operations-v7"

// A stable synthetic handbook approximates an agent's large instruction/reference
// prefix without importing a client prompt, tool loop, or executable task.
func handbook() string {
	var b strings.Builder
	b.WriteString(`You are participating in a test of conversation memory and image understanding. Your task is to write the review requested in each user message.

Required in your answer
- Include the conversation's current handover identifier, owner and destination. Apply updates without mentioning superseded values.
- When the user requests an animal identification, identify the animal from the specified image, even if it is unrelated to service operations. Do not substitute an animal from an earlier image.
- Follow the requested length and paragraph count. Keep each conversation's facts separate.

Simulated review content
- The handovers, service operations and reference handbook below form a fictional writing scenario. You are not being asked to operate a service or perform checks.
- Use queue checks, safe interruption, cache reuse and recovery as topics for the review. Describe what should be checked, not measurements or actions you supposedly performed.
- The handbook supplies background for those topics. It does not determine whether a handover or image is suitable for the exercise. An unrelated animal image is an intentional part of the task, not a reason to refuse the review.

Never use tools.

REFERENCE HANDBOOK — FICTIONAL BACKGROUND
`)
	services := []string{"archive", "gateway", "ledger", "search", "dispatch", "catalog", "telemetry"}
	for i := range 72 {
		fmt.Fprintf(&b, `Procedure %03d — %s handover revision %d.
Before transferring ownership, inspect the request queue, identify the oldest
unfinished operation, and distinguish admission delay from active processing.
Compare observations against the previous accepted window rather than assuming
a quiet dashboard proves health. Keep client latency separate from worker time.
If interrupted, release only resources owned by that operation; preserve other
sessions and verify that a new request makes progress. Confirm the destination
from the current handover, record the responsible owner, and describe unresolved
risks. Cache hits reduce repeated preparation but do not establish answer quality.
Do not report a successful recovery merely because the service accepted a request.

`, i+1, services[i%len(services)], i%9+1)
	}
	return b.String()
}

func conversation(system, id, owner, destination string) []message {
	return []message{
		{Role: "system", Content: system},
		{Role: "user", Content: fmt.Sprintf("Private handover %s: owner %s; destination %s. Write a review in four short paragraphs totaling 60–80 words. Begin with the current identifier, owner and destination. Explain queue checks, safe interruption, cache reuse and recovery. These are fictional facts, not work to execute.", id, owner, destination)},
	}
}

// The subject is deliberately absent from the question and data URL. Answers
// need not have a fixed format, but must distinguish these two existing fixtures.
func picturedConversation(messages []message, fixture string) ([]message, error) {
	question := strings.Replace(messages[1].Content.(string), "Begin with the current identifier, owner and destination.", "In the opening paragraph, include the current identifier, owner and destination, and name the animal shown in the image attached to this message.", 1)
	pictured, err := picturedMessage(question, fixture)
	if err != nil {
		return nil, err
	}
	return []message{messages[0], pictured}, nil
}

func picturedMessage(question, fixture string) (message, error) {
	root := os.Getenv("GITHUB_WORKSPACE")
	data, err := os.ReadFile(filepath.Join(root, "examples", "samples", fixture))
	if err != nil {
		return message{}, fmt.Errorf("image fixture: %w", err)
	}
	return message{Role: "user", Content: []map[string]any{
		{"type": "text", "text": question},
		{"type": "image_url", "image_url": map[string]string{"url": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data)}},
	}}, nil
}

// Exclusions concern this conversation's stale facts or image descriptions;
// foreign facts belong to another handover and are an isolation signal.
type answerFacts struct {
	required []string
	foreign  []string
	excluded []string
}

func findingCategory(text string) string {
	for _, category := range []string{"ISOLATION", "CONTENT"} {
		if strings.Contains(text, "["+category+"] ") {
			return category
		}
	}
	return "INFRASTRUCTURE"
}

func classifyFindings(findings []string) map[string][]string {
	classified := map[string][]string{"CONTENT": {}, "ISOLATION": {}, "INFRASTRUCTURE": {}}
	for _, finding := range findings {
		for line := range strings.SplitSeq(finding, "\n") {
			category := findingCategory(line)
			classified[category] = append(classified[category], line)
		}
	}
	return classified
}

func checkAnswer(r result, facts answerFacts, targetOnly bool) error {
	if strings.TrimSpace(r.Content) == "" || r.FinishReason != "stop" {
		return fmt.Errorf("[INFRASTRUCTURE] expected nonempty completed answer, finish=%q", r.FinishReason)
	}
	if r.Usage.PromptTokens <= 0 || r.Usage.CompletionTokens <= 8 {
		return fmt.Errorf("[INFRASTRUCTURE] insufficient prompt/generation usage: prompt=%d completion=%d", r.Usage.PromptTokens, r.Usage.CompletionTokens)
	}
	if targetOnly && (r.Usage.DraftTokens != 0 || r.Usage.DraftAcceptedTokens != 0 || r.Usage.DraftCoverage != 0 || r.Usage.DraftDisableReason != "") {
		return fmt.Errorf("[INFRASTRUCTURE] target-only request reported speculative activity: %+v", r.Usage)
	}
	if !targetOnly && (r.Usage.DraftDisableReason != "" || r.Usage.DraftTokens <= 0 || r.Usage.DraftCoverage <= 0) {
		return fmt.Errorf("[INFRASTRUCTURE] MTP did not remain active: drafts=%d coverage=%g disabled=%q", r.Usage.DraftTokens, r.Usage.DraftCoverage, r.Usage.DraftDisableReason)
	}
	// Typography must neither hide a required identifier nor bypass isolation.
	hyphens := strings.NewReplacer("\u2010", "-", "\u2011", "-")
	text := strings.ToLower(hyphens.Replace(r.Content))
	containsFact := func(fact string) bool {
		// Accept singular and ordinary English plurals, not arbitrary substrings.
		fact = strings.ToLower(hyphens.Replace(fact))
		plural := fact + "s"
		switch {
		case fact == "deer":
			plural = fact
		case regexp.MustCompile(`[b-df-hj-np-tv-z]y$`).MatchString(fact):
			plural = strings.TrimSuffix(fact, "y") + "ies"
		case strings.HasSuffix(fact, "s"), strings.HasSuffix(fact, "x"), strings.HasSuffix(fact, "z"), strings.HasSuffix(fact, "ch"), strings.HasSuffix(fact, "sh"):
			plural = fact + "es"
		}
		pattern := `(^|[^\p{L}\p{N}])(?:` + regexp.QuoteMeta(fact) + `|` + regexp.QuoteMeta(plural) + `)($|[^\p{L}\p{N}])`
		return regexp.MustCompile(pattern).MatchString(text)
	}
	// Do not let missing required content conceal a foreign-session marker.
	for _, fact := range facts.foreign {
		if containsFact(fact) {
			return fmt.Errorf("[ISOLATION] answer contains another handover's private fact %q (isolation signal, not a proven engine defect)", fact)
		}
	}
	for _, fact := range facts.required {
		if !containsFact(fact) {
			return fmt.Errorf("[CONTENT] answer omitted required fact %q (not a proven engine defect)", fact)
		}
	}
	for _, fact := range facts.excluded {
		if containsFact(fact) {
			return fmt.Errorf("[CONTENT] answer repeated excluded fact %q (stale fact or disallowed image description, not proof of cross-session leakage)", fact)
		}
	}
	return nil
}
