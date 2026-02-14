package kronk_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/uuid"
)

var grammarJSONObject = `root ::= object
value ::= object | array | string | number | "true" | "false" | "null"
object ::= "{" ws ( string ":" ws value ("," ws string ":" ws value)* )? ws "}"
array ::= "[" ws ( value ("," ws value)* )? ws "]"
string ::= "\"" ([^"\\] | "\\" ["\\bfnrt/] | "\\u" [0-9a-fA-F]{4})* "\""
number ::= "-"? ("0" | [1-9][0-9]*) ("." [0-9]+)? ([eE] [+-]? [0-9]+)?
ws ::= [ \t\n\r]*`

var dGrammarJSON model.D

func init() {
	dGrammarJSON = model.D{
		"messages": []model.D{
			{
				"role":    "user",
				"content": "List 3 programming languages with their year of creation. Respond in JSON format.",
			},
		},
		"grammar":     grammarJSONObject,
		"temperature": 0.7,
		"max_tokens":  512,
	}
}

func testGrammarJSON(t *testing.T, krn *kronk.Kronk) {
	if runInParallel {
		t.Parallel()
	}

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		id := uuid.New().String()
		now := time.Now()
		defer func() {
			done := time.Now()
			t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, krn.ModelInfo().ID, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
		}()

		resp, err := krn.Chat(ctx, dGrammarJSON)
		if err != nil {
			return fmt.Errorf("grammar chat: %w", err)
		}

		result := testGrammarJSONResponse(resp, krn.ModelInfo().ID)

		for _, w := range result.Warnings {
			t.Logf("WARNING: %s", w)
		}

		if result.Err != nil {
			t.Logf("%#v", resp)
			return result.Err
		}

		return nil
	}

	if err := f(); err != nil {
		t.Errorf("error: %v", err)
	}
}

func testGrammarJSONStreaming(t *testing.T, krn *kronk.Kronk) {
	if runInParallel {
		t.Parallel()
	}

	f := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), testDuration)
		defer cancel()

		id := uuid.New().String()
		now := time.Now()
		defer func() {
			done := time.Now()
			t.Logf("%s: %s, st: %v, en: %v, Duration: %s", id, krn.ModelInfo().ID, now.Format("15:04:05.000"), done.Format("15:04:05.000"), done.Sub(now))
		}()

		ch, err := krn.ChatStreaming(ctx, dGrammarJSON)
		if err != nil {
			return fmt.Errorf("grammar streaming: %w", err)
		}

		var content strings.Builder
		var lastResp model.ChatResponse
		for resp := range ch {
			lastResp = resp

			if resp.Choice[0].FinishReason() == model.FinishReasonStop {
				break
			}

			if len(resp.Choice) > 0 && resp.Choice[0].Delta != nil {
				content.WriteString(resp.Choice[0].Delta.Content)
			}
		}

		result := testGrammarStreamingContent(content.String(), krn.ModelInfo().ID)

		for _, w := range result.Warnings {
			t.Logf("WARNING: %s", w)
		}

		if result.Err != nil {
			t.Logf("accumulated content: %q", content.String())
			t.Logf("%#v", lastResp)
			return result.Err
		}

		return nil
	}

	if err := f(); err != nil {
		t.Errorf("error: %v", err)
	}
}

func testGrammarJSONResponse(resp model.ChatResponse, modelName string) testResult {
	var result testResult

	if resp.ID == "" {
		result.Err = fmt.Errorf("expected id")
		return result
	}

	if resp.Object != model.ObjectChatTextFinal {
		result.Err = fmt.Errorf("expected object type to be %s, got %s", model.ObjectChatTextFinal, resp.Object)
		return result
	}

	if resp.Model != modelName {
		result.Err = fmt.Errorf("expected model to be %s, got %s", modelName, resp.Model)
		return result
	}

	if len(resp.Choice) == 0 {
		result.Err = fmt.Errorf("expected choice, got %d", len(resp.Choice))
		return result
	}

	msg := resp.Choice[0].Message
	if msg == nil {
		result.Err = fmt.Errorf("expected message to be non-nil")
		return result
	}

	content := strings.TrimSpace(msg.Content)
	if content == "" {
		result.Err = fmt.Errorf("expected content to be non-empty")
		return result
	}

	var js any
	if err := json.Unmarshal([]byte(content), &js); err != nil {
		result.Err = fmt.Errorf("grammar: expected valid JSON, got parse error: %w\ncontent: %s", err, content)
		return result
	}

	return result
}

func testGrammarStreamingContent(content string, modelName string) testResult {
	var result testResult

	content = strings.TrimSpace(content)
	if content == "" {
		result.Err = fmt.Errorf("grammar streaming: expected content, got empty")
		return result
	}

	var js any
	if err := json.Unmarshal([]byte(content), &js); err != nil {
		result.Err = fmt.Errorf("grammar streaming: expected valid JSON, got parse error: %w\ncontent: %s", err, content)
		return result
	}

	return result
}
