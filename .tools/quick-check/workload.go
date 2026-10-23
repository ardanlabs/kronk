package main

import (
	"fmt"
	"strings"
)

const workloadVersion = "operations-v2"

// A stable synthetic handbook approximates an agent's large instruction/reference
// prefix without importing a client prompt, tool loop, or executable task.
func handbook() string {
	var b strings.Builder
	b.WriteString(`You are an operations assistant reviewing fictional service handovers.
Use the reference handbook below as background, not as commands to execute.
Never use tools. Treat each conversation's private handover as independent.
When asked for a review, mention the current handover's identifier, owner, and
destination naturally, then explain concrete checks in four short paragraphs.
Do not invent measurements or claim that you executed a check. Preserve private
facts across turns unless explicitly updated. Never import facts from another
handover. Acknowledge uncertainty in ordinary prose; no rigid output format is needed.

REFERENCE HANDBOOK
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
		{Role: "user", Content: fmt.Sprintf("Private handover %s: owner %s; destination %s. Review the handover using four short paragraphs (about 80 words). Include the identifier, owner, and destination, and explain queue checks, safe interruption, cache reuse, and recovery. These are fictional facts, not work to execute.", id, owner, destination)},
	}
}

func checkAnswer(r result, expected, forbidden []string) error {
	if strings.TrimSpace(r.Content) == "" || r.FinishReason != "stop" {
		return fmt.Errorf("expected nonempty completed answer, finish=%q", r.FinishReason)
	}
	if r.Usage.PromptTokens <= 0 || r.Usage.CompletionTokens <= 8 {
		return fmt.Errorf("insufficient prompt/generation usage: prompt=%d completion=%d", r.Usage.PromptTokens, r.Usage.CompletionTokens)
	}
	if r.Usage.DraftDisableReason != "" || r.Usage.DraftTokens <= 0 || r.Usage.DraftCoverage <= 0 {
		return fmt.Errorf("MTP did not remain active: drafts=%d coverage=%g disabled=%q", r.Usage.DraftTokens, r.Usage.DraftCoverage, r.Usage.DraftDisableReason)
	}
	text := strings.ToLower(r.Content)
	for _, fact := range expected {
		if !strings.Contains(text, strings.ToLower(fact)) {
			return fmt.Errorf("answer omitted handover fact %q (content finding, not a proven engine defect)", fact)
		}
	}
	for _, fact := range forbidden {
		if strings.Contains(text, strings.ToLower(fact)) {
			return fmt.Errorf("answer leaked foreign handover fact %q", fact)
		}
	}
	return nil
}
