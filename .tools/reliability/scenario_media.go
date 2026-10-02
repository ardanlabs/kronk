package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	mediaPollInterval = 20 * time.Millisecond
	mediaReadyEvents  = 5
)

type mediaCorrectnessResult struct {
	FirstImage  completionResult `json:"first_image"`
	TextOnly    completionResult `json:"text_only"`
	RepeatImage completionResult `json:"repeat_image"`
	Failures    []string         `json:"failures,omitempty"`
}

type mediaStreamResult struct {
	streamResult
	EventTimes []time.Time `json:"-"`
	Error      string      `json:"error,omitempty"`
}

type mediaPrefillResult struct {
	Generation                mediaStreamResult `json:"generation"`
	Image                     mediaStreamResult `json:"image"`
	SchedulerPolls            int               `json:"scheduler_polls"`
	PhaseCounts               map[string]int    `json:"phase_counts"`
	MediaPrefillObservations  int               `json:"media_prefill_observations"`
	GenerationOverlap         int               `json:"generation_overlap_observations"`
	BaselineMaximumGapSeconds float64           `json:"baseline_maximum_gap_seconds"`
	PrefillMaximumGapSeconds  float64           `json:"prefill_maximum_gap_seconds"`
	AllowedMaximumGapSeconds  float64           `json:"allowed_maximum_gap_seconds"`
	Failures                  []string          `json:"failures,omitempty"`
}

func runMedia(rc *runContext) scenarioResult {
	model := rc.cfg.MediaModel
	result := scenarioResult{Models: []string{model}, Details: map[string]any{}}
	imageURL, err := mediaDataURL(rc.cfg.MediaImage)
	if err != nil {
		result.Failures = append(result.Failures, err.Error())
		return result
	}
	loaded, err := rc.ensureModel(model)
	if err != nil {
		result.Failures = append(result.Failures, err.Error())
		return result
	}
	result.Details["loaded_model"] = loaded
	if loaded.HasProjection != nil && !*loaded.HasProjection {
		result.Failures = append(result.Failures, "loaded model reports has_projection=false")
	}

	if rc.cfg.MediaProfile == "all" || rc.cfg.MediaProfile == "correctness" {
		correctness := runMediaCorrectness(rc, model, imageURL)
		result.Details["correctness"] = correctness
		for _, failure := range correctness.Failures {
			appendFailure(&result.Failures, "correctness: %s", failure)
		}
	}
	if rc.cfg.MediaProfile == "all" || rc.cfg.MediaProfile == "prefill" {
		prefill := runMediaPrefill(rc, model, imageURL, loaded)
		result.Details["concurrent_prefill"] = prefill
		for _, failure := range prefill.Failures {
			appendFailure(&result.Failures, "prefill: %s", failure)
		}
	}
	return result
}

func runMediaCorrectness(rc *runContext, model, imageURL string) mediaCorrectnessResult {
	runID := fmt.Sprint(time.Now().UnixNano())
	prefix := "MEDIA-OBSERVATION-" + runID + ":"
	content := []map[string]any{
		{"type": "text", "text": "Describe the main subject concisely. Start your response with exactly " + prefix},
		{"type": "image_url", "image_url": map[string]any{"url": imageURL}},
	}
	result := mediaCorrectnessResult{}
	var err error
	result.FirstImage, err = rc.completion(model, []map[string]any{{"role": "user", "content": content}}, rc.cfg.MediaMaxTokens)
	if err != nil {
		appendFailure(&result.Failures, "first image: %v", err)
	} else {
		text := strings.TrimSpace(result.FirstImage.Content)
		if !strings.HasPrefix(text, prefix) || strings.TrimSpace(strings.TrimPrefix(text, prefix)) == "" {
			appendFailure(&result.Failures, "first image response lacks a nonempty %s prefix", prefix)
		}
		if !containsFold(text, rc.cfg.MediaExpectedTerms) {
			appendFailure(&result.Failures, "first image response lacks expected terms %v", rc.cfg.MediaExpectedTerms)
		}
	}

	marker := "TEXT-ONLY-" + runID
	result.TextOnly, err = rc.completion(model, []map[string]any{{
		"role": "user", "content": "Reply with exactly this marker and nothing else: " + marker,
	}}, rc.cfg.MediaMaxTokens)
	if err != nil {
		appendFailure(&result.Failures, "text-only isolation: %v", err)
	} else {
		if strings.TrimSpace(result.TextOnly.Content) != marker {
			result.Failures = append(result.Failures, "text-only response did not exactly match its marker")
		}
		if strings.Contains(result.TextOnly.Content, prefix) {
			result.Failures = append(result.Failures, "text-only response leaked the image response prefix")
		}
	}

	result.RepeatImage, err = rc.completion(model, []map[string]any{{"role": "user", "content": content}}, rc.cfg.MediaMaxTokens)
	if err != nil {
		appendFailure(&result.Failures, "repeat image: %v", err)
	} else {
		if result.RepeatImage.Content != result.FirstImage.Content {
			result.Failures = append(result.Failures, "repeat image response differs from the first deterministic response")
		}
		if !containsFold(result.RepeatImage.Content, rc.cfg.MediaExpectedTerms) {
			appendFailure(&result.Failures, "repeat image response lacks expected terms %v", rc.cfg.MediaExpectedTerms)
		}
		if result.RepeatImage.Usage.PromptTokensDetails.CachedTokens <= 0 {
			result.Failures = append(result.Failures, "repeat image response reported no cached tokens")
		}
	}
	stripCompletionContent(&result.FirstImage)
	stripCompletionContent(&result.TextOnly)
	stripCompletionContent(&result.RepeatImage)
	rc.artifacts.logf("media correctness: first_chars=%d text_chars=%d repeat_chars=%d repeat_cached=%d",
		result.FirstImage.Characters, result.TextOnly.Characters, result.RepeatImage.Characters,
		result.RepeatImage.Usage.PromptTokensDetails.CachedTokens)
	return result
}

func runMediaPrefill(rc *runContext, model, imageURL string, loaded loadedModel) mediaPrefillResult {
	probeCtx, cancel := context.WithCancel(rc)
	defer cancel()

	result := mediaPrefillResult{
		PhaseCounts:              make(map[string]int),
		AllowedMaximumGapSeconds: rc.cfg.MediaMaxGenerationGap.Seconds(),
	}
	if loaded.Slots < 2 {
		appendFailure(&result.Failures, "selected model must have at least 2 slots; got %d", loaded.Slots)
		return result
	}
	engine, err := rc.engine(model)
	if err != nil {
		appendFailure(&result.Failures, "scheduler diagnostics: %v", err)
		return result
	}
	if engine == nil || len(engine.Slots) < 2 {
		result.Failures = append(result.Failures, "scheduler diagnostics report fewer than 2 slots")
		return result
	}

	generationBody := map[string]any{
		"model": model, "stream": true, "stream_options": map[string]any{"include_usage": true},
		"temperature": 0, "seed": rc.cfg.Seed, "max_tokens": rc.cfg.MediaGenerationMaxTokens,
		"enable_thinking": false, "messages": []map[string]any{{
			"role": "user", "content": "Write the integers from 1 upward, one integer per line. Continue without commentary until the output limit.",
		}},
	}
	imageBody := map[string]any{
		"model": model, "stream": true, "stream_options": map[string]any{"include_usage": true},
		"temperature": 0, "seed": rc.cfg.Seed, "max_tokens": rc.cfg.MediaImageMaxTokens,
		"enable_thinking": false, "messages": []map[string]any{{
			"role": "user", "content": []map[string]any{
				{"type": "text", "text": fmt.Sprintf("Media load probe %d. Describe the main subject briefly.", time.Now().UnixNano())},
				{"type": "image_url", "image_url": map[string]any{"url": imageURL}},
			},
		}},
	}

	ready := make(chan bool, 1)
	var readyOnce sync.Once
	generationDone := make(chan mediaStreamResult, 1)
	go func() {
		var eventTimes []time.Time
		stream, err := rc.client.stream(probeCtx, rc.scenario, generationBody, func(chunk chatChunk, at time.Time) error {
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				eventTimes = append(eventTimes, at)
				if len(eventTimes) >= mediaReadyEvents {
					readyOnce.Do(func() { ready <- true })
				}
			}
			return nil
		})
		readyOnce.Do(func() { ready <- false })
		out := mediaStreamResult{streamResult: stream, EventTimes: eventTimes}
		if err != nil {
			out.Error = err.Error()
		}
		generationDone <- out
	}()

	select {
	case generationReady := <-ready:
		if !generationReady {
			result.Generation = <-generationDone
			result.Failures = append(result.Failures, "text generation ended before producing enough content events")
			return result
		}
	case <-time.After(rc.cfg.Timeout):
		result.Failures = append(result.Failures, "text generation did not become ready before timeout")
		return result
	}
	imageSubmittedAt := time.Now()
	imageDone := make(chan mediaStreamResult, 1)
	go func() {
		var eventTimes []time.Time
		stream, err := rc.client.stream(probeCtx, rc.scenario, imageBody, func(chunk chatChunk, at time.Time) error {
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				eventTimes = append(eventTimes, at)
			}
			return nil
		})
		out := mediaStreamResult{streamResult: stream, EventTimes: eventTimes}
		if err != nil {
			out.Error = err.Error()
		}
		imageDone <- out
	}()

	var generationFinished, imageFinished, pollingFailed bool
	deadline := time.Now().Add(rc.cfg.Timeout)
	for !generationFinished || !imageFinished {
		select {
		case result.Generation = <-generationDone:
			generationFinished = true
		case result.Image = <-imageDone:
			imageFinished = true
		default:
			if time.Now().After(deadline) {
				result.Failures = append(result.Failures, "media streams did not finish before timeout")
				return result
			}
			if pollingFailed {
				time.Sleep(mediaPollInterval)
				continue
			}
			var current []engineDiagnostic
			pollCtx, pollCancel := context.WithTimeout(probeCtx, 5*time.Second)
			_, err := rc.client.json(pollCtx, rc.scenario, http.MethodGet, "/v1/kronk/models/slots", nil, &current)
			pollCancel()
			if err != nil {
				appendFailure(&result.Failures, "slot polling: %v", err)
				pollingFailed = true
				time.Sleep(mediaPollInterval)
				continue
			}
			for _, candidate := range current {
				if !modelMatches(candidate.ModelID, model) {
					continue
				}
				result.SchedulerPolls++
				hasMedia, hasGeneration := false, false
				for _, slot := range candidate.Slots {
					if slot.Phase == "idle" {
						continue
					}
					result.PhaseCounts[slot.Phase]++
					hasMedia = hasMedia || slot.Phase == "media-prefill"
					hasGeneration = hasGeneration || slot.Phase == "generation"
				}
				if hasMedia {
					result.MediaPrefillObservations++
				}
				if hasMedia && hasGeneration {
					result.GenerationOverlap++
				}
			}
			time.Sleep(mediaPollInterval)
		}
	}

	if result.Generation.Error != "" {
		appendFailure(&result.Failures, "generation stream: %s", result.Generation.Error)
	}
	if result.Image.Error != "" {
		appendFailure(&result.Failures, "image stream: %s", result.Image.Error)
	}
	if result.Generation.ContentCharacters == 0 || result.Image.ContentCharacters == 0 {
		result.Failures = append(result.Failures, "generation and image streams must both return content")
	}
	if result.MediaPrefillObservations == 0 {
		result.Failures = append(result.Failures, "scheduler never observed media-prefill")
	}
	if result.GenerationOverlap == 0 {
		result.Failures = append(result.Failures, "scheduler never observed media-prefill and generation simultaneously")
	}
	if len(result.Image.EventTimes) == 0 {
		result.Failures = append(result.Failures, "image prefill window could not be measured")
		return result
	}
	imageFirstContent := result.Image.EventTimes[0]
	result.BaselineMaximumGapSeconds = maximumEventGap(result.Generation.EventTimes, func(start, end time.Time) bool {
		return end.Before(imageSubmittedAt) || end.Equal(imageSubmittedAt)
	})
	result.PrefillMaximumGapSeconds = maximumEventGap(result.Generation.EventTimes, func(start, end time.Time) bool {
		return !start.After(imageFirstContent) && !end.Before(imageSubmittedAt)
	})
	if result.PrefillMaximumGapSeconds == 0 {
		result.Failures = append(result.Failures, "no text content-event gap overlapped image submission through first image content")
	} else if result.PrefillMaximumGapSeconds > rc.cfg.MediaMaxGenerationGap.Seconds() {
		appendFailure(&result.Failures, "maximum overlapping text event gap %.3fs exceeds %.3fs",
			result.PrefillMaximumGapSeconds, rc.cfg.MediaMaxGenerationGap.Seconds())
	}
	rc.artifacts.logf("media prefill: polls=%d media_observations=%d overlap=%d baseline_gap=%.3fs prefill_gap=%.3fs allowed=%.3fs",
		result.SchedulerPolls, result.MediaPrefillObservations, result.GenerationOverlap,
		result.BaselineMaximumGapSeconds, result.PrefillMaximumGapSeconds, result.AllowedMaximumGapSeconds)
	return result
}

func mediaDataURL(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read media image: %w", err)
	}
	mediaType := mime.TypeByExtension(filepath.Ext(path))
	if mediaType == "" {
		mediaType = "image/jpeg"
	}
	return fmt.Sprintf("data:%s;base64,%s", mediaType, base64.StdEncoding.EncodeToString(data)), nil
}

func containsFold(text string, terms []string) bool {
	text = strings.ToLower(text)
	for _, term := range terms {
		if strings.Contains(text, strings.ToLower(term)) {
			return true
		}
	}
	return false
}

func stripCompletionContent(result *completionResult) {
	result.Content = ""
}

func maximumEventGap(events []time.Time, include func(time.Time, time.Time) bool) float64 {
	maximum := 0.0
	for index := 1; index < len(events); index++ {
		start, end := events[index-1], events[index]
		if include(start, end) {
			maximum = max(maximum, end.Sub(start).Seconds())
		}
	}
	return maximum
}
