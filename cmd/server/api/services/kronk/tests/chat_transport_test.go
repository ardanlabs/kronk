package chatapi_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

// These cases deliberately share each model/configuration with the surrounding
// API group. Only the peer-survival case needs a separate two-slot profile.
func chatTransport(t *testing.T, test *apitest.Test, token, modelID string, slots int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	krn, err := test.Pool.Kronk.AquireModel(ctx, modelID)
	if err != nil {
		t.Fatal(err)
	}
	if cfg := krn.ModelConfig(); cfg.NSeqMax() != slots {
		t.Fatalf("unexpected transport configuration: %s", cfg.String())
	}
	requireRollback := krn.ModelConfig().Speculation == model.SpeculationMTP
	t.Logf("transport %s: %s / %s / %s; %s", modelID,
		krn.ModelInfo().Type, krn.ModelInfo().SequenceProcessing, krn.ModelInfo().Architecture, krn.ModelConfig().String())

	var handlers atomic.Int64
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlers.Add(1)
		defer handlers.Add(-1)
		test.ServeHTTP(w, r)
	}))
	server.Config.BaseContext = func(net.Listener) context.Context { return ctx }
	server.Start()
	client := server.Client()
	defer func() {
		cancel()
		client.CloseIdleConnections()
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := server.Config.Shutdown(cleanup); err != nil {
			t.Errorf("bounded HTTP shutdown: %v", err)
			if err := server.Config.Close(); err != nil {
				t.Errorf("closing HTTP connections: %v", err)
			}
			return
		}
		server.Close()
	}()

	wait := func(t *testing.T, label string, limit time.Duration, predicate func(model.BatchEngineSnapshot) bool) {
		t.Helper()
		deadline, stop := context.WithTimeout(ctx, limit)
		defer stop()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			snapshot, ok := krn.BatchEngineSnapshot()
			if ok && predicate(snapshot) {
				t.Logf("%s: active=%d handlers=%d queued=%d pending=%d slots=%+v", label,
					krn.ActiveStreams(), handlers.Load(), snapshot.QueuedRequests, snapshot.PendingRequests, snapshot.Slots)
				return
			}
			select {
			case <-deadline.Done():
				t.Fatalf("%s not observed: %v; active=%d handlers=%d snapshot=%+v", label,
					deadline.Err(), krn.ActiveStreams(), handlers.Load(), snapshot)
			case <-ticker.C:
			}
		}
	}
	cancelled := func(id string) bool {
		for _, record := range test.ModelEvents() {
			if record.Attributes["id"] == id && record.Attributes["status"] == "cancel" {
				return true
			}
		}
		return false
	}
	rolledBack := func(id string, marker int) bool {
		for _, record := range test.ModelEvents()[marker:] {
			if record.Message == "speculative" && record.Attributes["status"] == "target-rollback" && record.Attributes["id"] == id {
				return true
			}
		}
		return false
	}
	idle := func(t *testing.T) {
		t.Helper()
		wait(t, "returned capacity and handler cleanup", 5*time.Second, func(model.BatchEngineSnapshot) bool {
			return krn.ActiveStreams() == 0 && handlers.Load() == 0
		})
	}
	start := func(t *testing.T, request model.D) *liveChat {
		t.Helper()
		request["model"], request["stream"] = modelID, true
		return startLiveChat(ctx, t, client, server.URL, token, request)
	}
	recover := func(t *testing.T) {
		t.Helper()
		idle(t)
		request := testlib.StateRequest("COPPER", "ZULU", 303)
		messages := request["messages"].([]model.D)
		messages[0]["content"] = t.Name() + " recovery session. " + messages[0]["content"].(string)
		stream := start(t, request)
		checkLiveState(t, stream.result(t), modelID, "COPPER")
		idle(t)
	}
	longRequest := func(label string) model.D {
		request := testlib.StateRequest("BRAVO", "ECHO", 202)
		messages := request["messages"].([]model.D)
		messages[0]["content"] = label + " session. " + messages[0]["content"].(string)
		messages[1]["content"] = "Ignore this padding:\n" +
			strings.Repeat("red green blue orange violet yellow ", 900) + "\nWhat is your first code word?"
		return request
	}
	hold := func(t *testing.T) (*liveChat, string) {
		t.Helper()
		request := testlib.StateRequest("ALPHA", "DELTA", 101)
		request["messages"] = []model.D{{"role": "user", "content": "Count from 1 to 2000. Write every number, one per line. Do not abbreviate or skip any numbers."}}
		if requireRollback {
			request["messages"] = []model.D{{"role": "user", "content": "Write a detailed tutorial with 40 sections comparing fault recovery in TCP, database transactions and distributed job queues. Include concrete examples and explain the limits of each analogy."}}
			request["temperature"] = 0.7
		}
		request["max_tokens"] = 4096
		stream := start(t, request)
		first := stream.content(t)
		wait(t, "content delivered before generation completes", 20*time.Second, func(s model.BatchEngineSnapshot) bool {
			for _, slot := range s.Slots {
				if slot.RequestID == first.ID && slot.Phase == "generation" && krn.ActiveStreams() > 0 &&
					(!requireRollback || rolledBack(first.ID, 0)) {
					return true
				}
			}
			return false
		})
		return stream, first.ID
	}

	if slots == 2 {
		t.Run("cancel-with-surviving-peer", func(t *testing.T) {
			holder, id := hold(t)
			marker := len(test.ModelEvents())
			peer := start(t, longRequest("peer"))
			wait(t, "two occupied slots", 20*time.Second, func(s model.BatchEngineSnapshot) bool {
				occupied := 0
				for _, slot := range s.Slots {
					if slot.RequestID != "" {
						occupied++
					}
				}
				return occupied == 2 && krn.ActiveStreams() == 2 && (!requireRollback || rolledBack(id, marker))
			})
			holder.cancel()
			holder.interrupted(t)
			wait(t, "holder cancelled server-side", 5*time.Second, func(model.BatchEngineSnapshot) bool { return cancelled(id) })
			checkLiveState(t, peer.result(t), modelID, "BRAVO")
			recover(t)
		})
		return
	}

	t.Run("cancel-queued", func(t *testing.T) {
		holder, holderID := hold(t)
		marker := len(test.ModelEvents())
		queued := start(t, testlib.StateRequest("BRAVO", "ECHO", 202))
		var queuedID string
		wait(t, "queued behind occupied slot", 20*time.Second, func(s model.BatchEngineSnapshot) bool {
			for _, record := range test.ModelEvents()[marker:] {
				if record.Attributes["status"] == "queued" && record.Attributes["id"] != holderID {
					queuedID, _ = record.Attributes["id"].(string)
				}
			}
			return queuedID != "" && s.QueuedRequests+s.PendingRequests > 0 &&
				len(s.Slots) == 1 && s.Slots[0].RequestID == holderID && krn.ActiveStreams() == 2
		})
		queued.cancel()
		queued.interrupted(t)
		wait(t, "queued cancellation preserves holder", 5*time.Second, func(s model.BatchEngineSnapshot) bool {
			return cancelled(queuedID) && krn.ActiveStreams() == 1 && handlers.Load() == 1 && s.Slots[0].RequestID == holderID
		})
		holder.cancel()
		holder.interrupted(t)
		wait(t, "holder cancelled server-side", 5*time.Second, func(model.BatchEngineSnapshot) bool { return cancelled(holderID) })
		recover(t)
	})
	t.Run("cancel-partial-prefill", func(t *testing.T) {
		stream := start(t, longRequest("prefill"))
		var id string
		wait(t, "partial prefill", 20*time.Second, func(s model.BatchEngineSnapshot) bool {
			for _, slot := range s.Slots {
				if (slot.Phase == "prefill" && slot.PrefilledTokens > 0 && slot.PrefillRemaining > 0) ||
					(slot.Phase == "prefill-imc" && slot.IMCPreparedTokens > 0 && slot.IMCPreparationRemaining > 0) {
					id = slot.RequestID
					return id != "" && krn.ActiveStreams() == 1
				}
			}
			return false
		})
		stream.cancel()
		stream.interrupted(t)
		wait(t, "prefill cancelled server-side", 5*time.Second, func(model.BatchEngineSnapshot) bool { return cancelled(id) })
		recover(t)
	})
	for _, closeBody := range []bool{false, true} {
		t.Run(fmt.Sprintf("generation-close-body-%t", closeBody), func(t *testing.T) {
			stream, id := hold(t)
			if closeBody {
				if err := stream.body.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				stream.cancel()
			}
			stream.interrupted(t)
			wait(t, "generation cancelled server-side", 5*time.Second, func(model.BatchEngineSnapshot) bool { return cancelled(id) })
			recover(t)
		})
	}
}

type liveChat struct {
	cancel context.CancelFunc
	body   io.ReadCloser
	first  chan model.ChatResponse
	done   chan liveChatResult
}

type liveChatResult struct {
	events       []json.RawMessage
	contentTimes []time.Time
	done         bool
	readErr      bool
	err          error
}

func startLiveChat(ctx context.Context, t *testing.T, client *http.Client, url, token string, request model.D) *liveChat {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	t.Cleanup(cancel)
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("closing response: %v", err)
		}
	})
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode != http.StatusOK || err != nil || mediaType != "text/event-stream" ||
		resp.ContentLength != -1 {
		t.Fatalf("invalid SSE headers: status=%d headers=%v error=%v", resp.StatusCode, resp.Header, err)
	}
	stream := liveChat{cancel: cancel, body: resp.Body, first: make(chan model.ChatResponse, 1), done: make(chan liveChatResult, 1)}
	go func() {
		result := liveChatResult{}
		defer func() { stream.done <- result }()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		var pending string
		var sentFirst bool
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, ":"):
				continue
			case strings.HasPrefix(line, "data:") && !result.done:
				if pending != "" {
					pending += "\n"
				}
				pending += strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " ")
			case line == "" && pending != "":
				if pending == "[DONE]" {
					result.done = true
					pending = ""
					continue
				}
				var chunk model.ChatResponse
				if err := json.Unmarshal([]byte(pending), &chunk); err != nil {
					result.err = err
					return
				}
				result.events = append(result.events, json.RawMessage(pending))
				pending = ""
				for _, choice := range chunk.Choices {
					if choice.Delta != nil && choice.Delta.Content != "" {
						result.contentTimes = append(result.contentTimes, time.Now())
					}
					if !sentFirst && choice.Delta != nil && choice.Delta.Content != "" {
						stream.first <- chunk
						sentFirst = true
					}
				}
			case line == "":
			default:
				result.err = fmt.Errorf("invalid chat SSE framing: %q", line)
				return
			}
		}
		result.err = scanner.Err()
		result.readErr = result.err != nil
		if result.err == nil && (!result.done || pending != "") {
			result.err = fmt.Errorf("truncated SSE: done=%t pending=%q", result.done, pending)
		}
	}()
	return &stream
}

func (s *liveChat) content(t *testing.T) model.ChatResponse {
	t.Helper()
	select {
	case chunk := <-s.first:
		return chunk
	case result := <-s.done:
		t.Fatalf("stream ended before observing in-flight content: %+v", result)
	case <-time.After(20 * time.Second):
		t.Fatal("no content received within 20s")
	}
	return model.ChatResponse{}
}

func (s *liveChat) result(t *testing.T) liveChatResult {
	t.Helper()
	select {
	case result := <-s.done:
		return result
	case <-time.After(30 * time.Second):
		t.Fatal("stream did not terminate within 30s")
	}
	return liveChatResult{}
}

func (s *liveChat) interrupted(t *testing.T) {
	t.Helper()
	select {
	case result := <-s.done:
		if !result.readErr || result.done {
			t.Fatalf("expected interrupted stream, got done=%t error=%v", result.done, result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled HTTP reader did not terminate within 5s")
	}
}

func checkLiveState(t *testing.T, result liveChatResult, modelID, want string) {
	t.Helper()
	if result.err != nil || !result.done {
		t.Fatalf("incomplete live stream: done=%t error=%v", result.done, result.err)
	}
	if diff := validateChatUsageStreamReasoning(result.events, false); diff != "" {
		t.Fatal(diff)
	}
	var terminal model.ChatResponse
	var text strings.Builder
	for _, data := range result.events {
		var chunk model.ChatResponse
		if err := json.Unmarshal(data, &chunk); err != nil {
			t.Fatal(err)
		}
		if chunk.Model != modelID {
			t.Fatalf("model: got %q, want %q", chunk.Model, modelID)
		}
		for _, choice := range chunk.Choices {
			if choice.Delta != nil {
				text.WriteString(choice.Delta.Content)
			}
			if choice.FinishReason() != "" {
				terminal = chunk
			}
		}
		if chunk.Usage != nil {
			terminal.Usage = chunk.Usage
		}
	}
	if err := testlib.CheckStateReply(terminal, text.String(), want); err != nil {
		t.Fatal(err)
	}
	t.Logf("live recovery/peer: model=%s answer=%q", modelID, text.String())
}
