package main

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"
)

var mtpTopics = []struct {
	name       string
	vocabulary string
}{
	{"ASTRONOMY", "orbit nebula telescope galaxy comet"},
	{"COOKING", "recipe skillet rosemary pastry simmer"},
	{"GARDENING", "garden seedling compost orchard trellis"},
	{"ARCHITECTURE", "building archway masonry blueprint column"},
	{"MUSIC", "melody rhythm violin concert harmony"},
	{"SAILING", "harbor compass sailboat tide anchor"},
}

type mtpProfile struct {
	Name  string
	Model string
	Slots int
}

type mtpRequestResult struct {
	Request                int            `json:"request"`
	Wave                   int            `json:"wave"`
	TraceID                string         `json:"trace_id,omitempty"`
	CalibratedPromptTokens int            `json:"calibrated_prompt_tokens"`
	ElapsedSeconds         float64        `json:"elapsed_seconds"`
	FinishReason           string         `json:"finish_reason,omitempty"`
	OutputSHA256           string         `json:"output_sha256,omitempty"`
	Usage                  usage          `json:"usage"`
	Error                  string         `json:"error,omitempty"`
	RollbackRounds         int            `json:"rollback_rounds,omitempty"`
	RollbackStrategies     map[string]int `json:"rollback_strategies,omitempty"`
}

type mtpProfileResult struct {
	Name              string             `json:"name"`
	Model             string             `json:"model"`
	Slots             int                `json:"expected_slots"`
	Requests          int                `json:"requests"`
	Waves             int                `json:"waves"`
	MinRollbackRounds int                `json:"min_rollback_rounds"`
	Loaded            loadedModel        `json:"loaded_model"`
	Scheduler         *engineDiagnostic  `json:"scheduler,omitempty"`
	Results           []mtpRequestResult `json:"results,omitempty"`
	Recovery          completionResult   `json:"recovery"`
	Failures          []string           `json:"failures,omitempty"`
}

func runMTP(rc *runContext) scenarioResult {
	profiles := []mtpProfile{
		{Name: "embedded", Model: rc.cfg.MTPEmbeddedModel, Slots: rc.cfg.MTPEmbeddedSlots},
		{Name: "companion", Model: rc.cfg.MTPCompanionModel, Slots: rc.cfg.MTPCompanionSlots},
	}
	if rc.cfg.MTPProfile != "all" {
		for _, profile := range profiles {
			if profile.Name == rc.cfg.MTPProfile {
				profiles = []mtpProfile{profile}
				break
			}
		}
	}

	result := scenarioResult{Details: map[string]any{}}
	profileResults := make([]mtpProfileResult, 0, len(profiles))
	for _, profile := range profiles {
		rc.artifacts.logf("MTP profile=%s model=%s", profile.Name, profile.Model)
		profileResult := runMTPProfile(rc, profile)
		profileResults = append(profileResults, profileResult)
		result.Models = append(result.Models, profile.Model)
		for _, failure := range profileResult.Failures {
			appendFailure(&result.Failures, "%s: %s", profile.Name, failure)
		}
	}
	result.Details["profiles"] = profileResults
	return result
}

func runMTPProfile(rc *runContext, profile mtpProfile) mtpProfileResult {
	result := mtpProfileResult{Name: profile.Name, Model: profile.Model, Slots: profile.Slots, Waves: rc.cfg.MTPWaves, MinRollbackRounds: rc.cfg.MTPMinRollbackRounds}
	loaded, err := rc.ensureModel(profile.Model)
	if err != nil {
		result.Failures = append(result.Failures, err.Error())
		return result
	}
	result.Loaded = loaded
	if loaded.Slots != profile.Slots {
		appendFailure(&result.Failures, "loaded model reports %d slots, want %d", loaded.Slots, profile.Slots)
	}
	engine, err := rc.engine(profile.Model)
	if err != nil {
		appendFailure(&result.Failures, "scheduler diagnostics: %v", err)
		return result
	}
	result.Scheduler = engine
	if engine == nil {
		result.Failures = append(result.Failures, "scheduler diagnostics do not contain the selected model")
		return result
	}
	if rc.cfg.MTP == "on" && !engine.MTP {
		result.Failures = append(result.Failures, "scheduler reports MTP inactive")
	}
	if len(engine.Slots) != profile.Slots {
		appendFailure(&result.Failures, "scheduler reports %d slots, want %d", len(engine.Slots), profile.Slots)
	}

	requests := rc.cfg.MTPRequests
	if requests == 0 {
		requests = profile.Slots
	}
	result.Requests = requests
	if requests < profile.Slots {
		appendFailure(&result.Failures, "%d requests cannot exercise every one of %d configured slots", requests, profile.Slots)
		return result
	}

	prompts := make([]string, requests)
	counts := make([]int, requests)
	for index := range requests {
		prompt, count, err := calibrateMTPPrompt(rc, profile.Model, index, rc.cfg.MTPPromptTokens)
		if err != nil {
			appendFailure(&result.Failures, "calibrate request %d: %v", index+1, err)
			return result
		}
		prompts[index], counts[index] = prompt, count
		rc.artifacts.logf("prepared %s request %d: %d templated prompt tokens", profile.Name, index+1, count)
	}

	for wave := range rc.cfg.MTPWaves {
		rc.artifacts.logf("MTP %s wave %d/%d", profile.Name, wave+1, rc.cfg.MTPWaves)
		start := make(chan struct{})
		results := make(chan mtpRequestResult, requests)
		var ready sync.WaitGroup
		ready.Add(requests)
		for index := range requests {
			go func() {
				ready.Done()
				<-start
				completion, err := rc.completion(profile.Model, []map[string]any{{"role": "user", "content": prompts[index]}}, rc.cfg.MTPMaxTokens)
				requestResult := mtpRequestResult{
					Request:                wave*requests + index + 1,
					Wave:                   wave + 1,
					TraceID:                completion.TraceID,
					CalibratedPromptTokens: counts[index],
					ElapsedSeconds:         completion.Elapsed,
					FinishReason:           completion.FinishReason,
					Usage:                  completion.Usage,
				}
				if err != nil {
					requestResult.Error = err.Error()
				} else {
					requestResult.OutputSHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte(completion.Content)))
					if err := verifyMTPOutput(completion.Content, index); err != nil {
						requestResult.Error = err.Error()
					}
				}
				results <- requestResult
			}()
		}
		ready.Wait()
		close(start)
		waveResults := make([]mtpRequestResult, requests)
		for range requests {
			requestResult := <-results
			waveResults[requestResult.Request-wave*requests-1] = requestResult
		}
		result.Results = append(result.Results, waveResults...)
	}

	for _, requestResult := range result.Results {
		switch {
		case requestResult.Error != "":
			appendFailure(&result.Failures, "request %d: %s", requestResult.Request, requestResult.Error)
		case requestResult.FinishReason != "stop":
			appendFailure(&result.Failures, "request %d finish reason=%q, want stop", requestResult.Request, requestResult.FinishReason)
		case rc.cfg.MTP == "on" && requestResult.Usage.DraftTokens <= 0:
			appendFailure(&result.Failures, "request %d produced no draft tokens", requestResult.Request)
		case rc.cfg.MTP == "on" && requestResult.Usage.DraftCoverage <= 0:
			appendFailure(&result.Failures, "request %d produced no draft coverage", requestResult.Request)
		}
		rc.artifacts.logf("%s request %d: elapsed=%.3fs prompt=%d completion=%d drafted=%d accepted=%d coverage=%.3f finish=%s",
			profile.Name, requestResult.Request, requestResult.ElapsedSeconds, requestResult.Usage.PromptTokens,
			requestResult.Usage.CompletionTokens, requestResult.Usage.DraftTokens,
			requestResult.Usage.DraftAcceptedTokens, requestResult.Usage.DraftCoverage, requestResult.FinishReason)
	}
	marker := fmt.Sprintf("MTP-RECOVERY-%d", time.Now().UnixNano())
	result.Recovery, err = rc.completion(profile.Model, []map[string]any{{
		"role": "user", "content": "Reply with exactly " + marker + " and nothing else.",
	}}, 64)
	if err != nil {
		appendFailure(&result.Failures, "post-pressure recovery: %v", err)
	} else if strings.TrimSpace(result.Recovery.Content) != marker {
		result.Failures = append(result.Failures, "post-pressure recovery did not exactly match its fresh marker")
	}
	return result
}

func calibrateMTPPrompt(rc *runContext, model string, index, target int) (string, int, error) {
	low, high := 1, max(2, target)
	for {
		count, err := rc.tokenCount(model, mtpPrompt(index, high), true)
		if err != nil {
			return "", 0, err
		}
		if count >= target {
			break
		}
		high *= 2
	}
	for low < high {
		middle := (low + high) / 2
		count, err := rc.tokenCount(model, mtpPrompt(index, middle), true)
		if err != nil {
			return "", 0, err
		}
		if count < target {
			low = middle + 1
		} else {
			high = middle
		}
	}
	prompt := mtpPrompt(index, low)
	count, err := rc.tokenCount(model, prompt, true)
	return prompt, count, err
}

func mtpPrompt(index, records int) string {
	topic := mtpTopics[index%len(mtpTopics)]
	var result strings.Builder
	result.WriteString(fmt.Sprintf("REQUEST-%d %s. Read these deterministic records carefully.\n", index+1, topic.name))
	for record := range records {
		result.WriteString(fmt.Sprintf("%s record %04d: %s; marker %d-%04d.\n", topic.name, record, topic.vocabulary, index+1, record))
	}
	return result.String() + fmt.Sprintf("This session's key is MTP-SESSION-%04d. Include that key once, then give five short bullets, one for each supplied value: %s. Copy each value exactly and briefly describe it using only this session's records. Do not include any other session key.", index+1, topic.vocabulary)
}

// Check supplied facts and isolation, not a golden wording or a speculative
// implementation's output. Equivalent prose and formatting are allowed.
func verifyMTPOutput(output string, index int) error {
	words := strings.FieldsFunc(strings.ToLower(output), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '-'
	})
	key := fmt.Sprintf("mtp-session-%04d", index+1)
	found := make(map[string]bool)
	for _, word := range words {
		if strings.HasPrefix(word, "mtp-session-") && word != key {
			return fmt.Errorf("response leaked foreign session key %s", word)
		}
		found[word] = true
	}
	for _, value := range append(strings.Fields(mtpTopics[index%len(mtpTopics)].vocabulary), key) {
		if !found[value] {
			return fmt.Errorf("response lacks supplied fact or session key %s", value)
		}
	}
	return nil
}
