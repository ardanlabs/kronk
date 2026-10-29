package questions

import (
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
)

func TestOnlyTextModelsGetGenerationQuestions(t *testing.T) {
	generation := []string{
		"template", "sampling-parameters.enable_thinking", "sampling-parameters.reasoning_effort", KeyTemplateVars,
		"sampling-parameters.temperature", "sampling-parameters.top_p", "sampling-parameters.top_k", "sampling-parameters.min_p",
		"sampling-parameters.max_tokens", "sampling-parameters.repeat_penalty", "sampling-parameters.frequency_penalty",
		"sampling-parameters.presence_penalty", "incremental-cache", "cache-min-tokens", "imc-session-capacity",
		"speculation", "draft-model.ndraft",
	}
	always := []string{"context-window", "nseq-max", "cache-type-k", "flash-attention", "queue-depth", "prefill-batch-size"}

	for _, endpoint := range []string{"embeddings", "rerank", "decision"} {
		f := sampleFacts()
		f.Endpoint = endpoint
		a := NewAnswers(Chat())
		*a.Sel["speculation"] = "auto" // ndraft follows speculation; it must still be skipped

		for _, key := range generation {
			if a.Applies(f, a.ByKey[key]) {
				t.Errorf("%s model: %s asked, but the model doesn't generate text", endpoint, key)
			}
		}
		for _, key := range always {
			if !a.Applies(f, a.ByKey[key]) {
				t.Errorf("%s model: %s not asked", endpoint, key)
			}
		}
	}

	f := sampleFacts() // a chat model
	a := NewAnswers(Chat())
	for _, key := range generation {
		if key == "draft-model.ndraft" {
			continue // follows the speculation answer
		}
		if !a.Applies(f, a.ByKey[key]) {
			t.Errorf("chat model: %s not asked", key)
		}
	}
}

func TestStoredClassicSpeculationIgnoredWhenNotAsked(t *testing.T) {
	f := sampleFacts()
	f.Endpoint = "embeddings"
	qs := Chat()
	a := NewAnswers(qs)
	existing := map[string]any{"speculation": "classic", "nseq-max": 4}
	a.Prefill(f, existing)

	if got := changesFor(t, qs, f, a, existing); len(got) != 0 {
		t.Fatalf("changes = %+v; an embeddings model keeps its entry when nothing is answered", got)
	}
}

func TestNoConversationsForModelsThatDontGenerate(t *testing.T) {
	f := sampleFacts()
	f.Endpoint = "embeddings"
	a := NewAnswers(Chat())
	for _, q := range Chat() {
		if !a.Applies(f, q) {
			continue
		}
		q = q.For(f)
		for _, text := range []string{q.Title, q.About, q.Tip} {
			if strings.Contains(strings.ToLower(text), "conversation") || strings.Contains(text, "Chat ") {
				t.Errorf("%s asks an embeddings model about chat: %q", q.Key, text)
			}
		}
	}
}

func TestProjectorQuestionsFollowItsInputs(t *testing.T) {
	a := NewAnswers(Chat())
	asked := func(f source.Facts, key string) bool { return a.Applies(f, a.ByKey[key]) }
	about := func(f source.Facts, key string) string { return a.ByKey[key].For(f).About }

	audio := sampleFacts()
	audio.Inputs = source.Inputs{Known: true, Audio: true}
	if asked(audio, "image-token-budget") || !asked(audio, "proj-on-cpu") {
		t.Fatal("audio-only projector: the projector is asked, the image budget is not")
	}
	if got := about(audio, "proj-on-cpu"); !strings.Contains(got, "turns audio into tokens") {
		t.Fatalf("audio-only projector wording: %q", got)
	}

	both := sampleFacts()
	both.Inputs = source.Inputs{Known: true, Images: true, Audio: true}
	if !asked(both, "image-token-budget") || !strings.Contains(about(both, "proj-device"), "images and audio") {
		t.Fatal("images and audio: image budget asked, both named")
	}

	unknown := sampleFacts() // projector header not read
	if !asked(unknown, "image-token-budget") || !strings.Contains(about(unknown, "proj-on-cpu"), "images or audio") {
		t.Fatal("unreadable projector: its questions are asked anyway")
	}
}

func TestDraftsShareTheTokenizer(t *testing.T) {
	a := NewAnswers(Chat())
	f := sampleFacts() // drafts: test/draft

	has := func(f source.Facts, value string) bool {
		_, ok := a.ByKey["speculation"].Option(f, value)
		return ok
	}
	if !has(f, "classic") {
		t.Fatal("a compatible draft is installed: classic is offered")
	}

	f.Drafts = nil
	if has(f, "classic") {
		t.Fatal("no installed model shares the tokenizer: classic must not be offered")
	}
}
