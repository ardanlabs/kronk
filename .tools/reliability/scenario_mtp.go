package main

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
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
	Request                int     `json:"request"`
	TraceID                string  `json:"trace_id,omitempty"`
	CalibratedPromptTokens int     `json:"calibrated_prompt_tokens"`
	ElapsedSeconds         float64 `json:"elapsed_seconds"`
	FinishReason           string  `json:"finish_reason,omitempty"`
	OutputSHA256           string  `json:"output_sha256,omitempty"`
	Usage                  usage   `json:"usage"`
	Error                  string  `json:"error,omitempty"`
}

type mtpProfileResult struct {
	Name      string             `json:"name"`
	Model     string             `json:"model"`
	Slots     int                `json:"expected_slots"`
	Requests  int                `json:"requests"`
	Loaded    loadedModel        `json:"loaded_model"`
	Scheduler *engineDiagnostic  `json:"scheduler,omitempty"`
	Results   []mtpRequestResult `json:"results,omitempty"`
	Failures  []string           `json:"failures,omitempty"`
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
	result := mtpProfileResult{Name: profile.Name, Model: profile.Model, Slots: profile.Slots}
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
	if !engine.MTP {
		result.Failures = append(result.Failures, "scheduler reports MTP inactive")
	}
	if engine.NDraft <= 0 {
		appendFailure(&result.Failures, "scheduler reports invalid ndraft=%d", engine.NDraft)
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
				Request:                index + 1,
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
			}
			results <- requestResult
		}()
	}
	ready.Wait()
	close(start)
	result.Results = make([]mtpRequestResult, requests)
	for range requests {
		requestResult := <-results
		result.Results[requestResult.Request-1] = requestResult
	}

	for _, requestResult := range result.Results {
		switch {
		case requestResult.Error != "":
			appendFailure(&result.Failures, "request %d: %s", requestResult.Request, requestResult.Error)
		case requestResult.FinishReason != "stop":
			appendFailure(&result.Failures, "request %d finish reason=%q, want stop", requestResult.Request, requestResult.FinishReason)
		case requestResult.Usage.DraftTokens <= 0:
			appendFailure(&result.Failures, "request %d produced no draft tokens", requestResult.Request)
		case requestResult.Usage.DraftCoverage <= 0:
			appendFailure(&result.Failures, "request %d produced no draft coverage", requestResult.Request)
		case requestResult.Usage.DraftDisableReason != "":
			appendFailure(&result.Failures, "request %d disabled MTP: %s", requestResult.Request, requestResult.Usage.DraftDisableReason)
		}
		rc.artifacts.logf("%s request %d: elapsed=%.3fs prompt=%d completion=%d drafted=%d accepted=%d coverage=%.3f finish=%s",
			profile.Name, requestResult.Request, requestResult.ElapsedSeconds, requestResult.Usage.PromptTokens,
			requestResult.Usage.CompletionTokens, requestResult.Usage.DraftTokens,
			requestResult.Usage.DraftAcceptedTokens, requestResult.Usage.DraftCoverage, requestResult.FinishReason)
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
	return result.String() + fmt.Sprintf("Return exactly five short bullets summarizing only the %s records.", topic.name)
}
