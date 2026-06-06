package model

import "maps"

// =============================================================================
// Reasoning Normalization
//
// Reasoning ("thinking") content is ephemeral. Replaying it on prior assistant
// turns costs prompt tokens without improving generation, and chat templates
// render it inconsistently across turns — the same assistant turn emits its
// reasoning while it is the most recent turn, then drops it once a newer turn
// arrives. That shift re-tokenizes the IMC prefix and forces a full cache
// rebuild.
//
// The engine therefore drops reasoning from assistant history before the Jinja
// render (and before the IMC render fingerprint) whenever IMC is enabled and
// preserve_thinking is off. The field-drop is family-agnostic; the selected
// parser supplies the lineage-specific markup stripping via ReasoningNormalizer.
// =============================================================================

// preserveThinking reports whether reasoning content should be kept in the
// rendered prompt. It reads the request's preserve_thinking flag, defaulting
// to false: reasoning is dropped from history unless a caller explicitly opts
// back in.
func preserveThinking(d D) bool {
	v, ok := d["preserve_thinking"].(bool)
	if !ok {
		return false
	}
	return v
}

// stripReasoning reports whether the engine should remove reasoning from
// assistant history for this request. It is gated on IMC being enabled (the
// cache-stability win) and preserve_thinking being off.
func (m *Model) stripReasoning(d D) bool {
	return m.cfg.IncrementalCache() && !preserveThinking(d)
}

// normalizeHistoryReasoning removes reasoning from assistant messages so the
// rendered prompt is byte-stable across turns. It drops the reasoning and
// reasoning_content fields (family-agnostic) and, when the selected parser
// implements ReasoningNormalizer, strips lineage-specific reasoning spans
// embedded directly in assistant content. Messages are cloned copy-on-write so
// the caller's maps are never mutated.
func (m *Model) normalizeHistoryReasoning(d D) D {
	if !m.stripReasoning(d) {
		return d
	}

	messages, ok := d["messages"].([]D)
	if !ok {
		return d
	}

	norm, _ := m.parser.(ReasoningNormalizer)

	var copied bool
	for i, msg := range messages {
		if msg["role"] != "assistant" {
			continue
		}

		_, hasReasoning := msg["reasoning"]
		_, hasReasoningContent := msg["reasoning_content"]

		var newContent string
		var contentChanged bool
		if norm != nil {
			if s, ok := msg["content"].(string); ok {
				if stripped := norm.StripReasoningContent(s); stripped != s {
					newContent = stripped
					contentChanged = true
				}
			}
		}

		if !hasReasoning && !hasReasoningContent && !contentChanged {
			continue
		}

		if !copied {
			newMsgs := make([]D, len(messages))
			copy(newMsgs, messages)
			messages = newMsgs
			d["messages"] = messages
			copied = true
		}

		nm := maps.Clone(msg)
		delete(nm, "reasoning")
		delete(nm, "reasoning_content")
		if contentChanged {
			nm["content"] = newContent
		}
		messages[i] = nm
	}

	return d
}
