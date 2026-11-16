package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/imc"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/storage/disk"
	"github.com/charmbracelet/x/ansi"
)

func TestMouseScrollingAndLogPath(t *testing.T) {
	for _, width := range []int{100, 48} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := chat{input: textarea.New(), view: viewport.New(), logPath: "/Users/example/.kronk/examples/chat.log"}
			m.view.SoftWrap = true
			m.newConversation()
			m.Update(tea.WindowSizeMsg{Width: width, Height: 32})
			messages := m.request["messages"].([]model.D)
			m.request["messages"] = append(messages, model.D{"role": "assistant", "content": strings.Repeat("A long line that wraps at narrow terminal widths.\n", 80)})
			m.refresh(false)
			m.view.GotoTop()
			if m.View().MouseMode != tea.MouseModeCellMotion {
				t.Fatal("terminal mouse reporting is disabled")
			}
			m.Update(tea.KeyPressMsg{Code: tea.KeyF2})
			if m.View().MouseMode != tea.MouseModeNone {
				t.Fatal("text selection mode did not release terminal mouse reporting")
			}
			m.Update(tea.MouseWheelMsg{X: 5, Y: 6, Button: tea.MouseWheelDown})
			if !m.view.AtTop() {
				t.Fatal("text selection mode handled a mouse scrolling event")
			}
			m.Update(tea.KeyPressMsg{Code: tea.KeyF2})
			m.Update(tea.MouseWheelMsg{X: 5, Y: 6, Button: tea.MouseWheelDown})
			if m.view.YOffset() != 3 {
				t.Fatal("mouse wheel did not scroll the conversation")
			}
			m.Update(tea.MouseWheelMsg{X: 5, Y: 27, Button: tea.MouseWheelDown})
			if m.view.YOffset() != 3 {
				t.Fatal("mouse wheel over composer scrolled the conversation")
			}
			m.view.GotoTop()
			m.Update(tea.MouseClickMsg{X: width - 3, Y: 4, Button: tea.MouseLeft})
			m.Update(tea.MouseMotionMsg{X: width - 3, Y: 100, Button: tea.MouseLeft})
			if !m.view.AtBottom() {
				t.Fatal("dragging below the track did not reach the bottom")
			}
			m.Update(tea.MouseReleaseMsg{X: width - 3, Y: 100, Button: tea.MouseLeft})
			m.Update(tea.MouseMotionMsg{X: width - 3, Y: 4, Button: tea.MouseLeft})
			if !m.view.AtBottom() {
				t.Fatal("released scrollbar continued dragging")
			}
			m.Update(tea.MouseClickMsg{X: width - 3, Y: 4, Button: tea.MouseLeft})
			m.Update(tea.MouseReleaseMsg{X: width - 3, Y: 4, Button: tea.MouseLeft})
			if !m.view.AtTop() {
				t.Fatal("clicking the top of the track did not scroll to the top")
			}
			m.Update(streamChunk{})
			if !m.view.AtTop() {
				t.Fatal("stream update moved the manually scrolled conversation")
			}
			m.newConversation()
			m.input.SetValue("/log")
			m.submit()
			if !strings.Contains(m.view.GetContent(), m.logPath) || len(m.request["messages"].([]model.D)) != 1 {
				t.Fatal("/log did not display the full path without sending it to the model")
			}
		})
	}
}

func TestCompletedReasoningStaysVisibleOnlyInUI(t *testing.T) {
	store, err := disk.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	m := chat{input: textarea.New(), view: viewport.New(), store: store}
	m.newConversation()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 32})
	for i := range 2 {
		messages := m.request["messages"].([]model.D)
		m.request["messages"] = append(messages, model.D{"role": "user", "content": "Continue"})
		m.ctx, m.cancel = context.WithCancel(t.Context())
		m.busy = true
		m.reasoning, m.answer = fmt.Sprintf("Private thought %d", i), fmt.Sprintf("Public answer %d", i)
		_, cmd := m.Update(streamChunk{closed: true})
		m.Update(cmd())
		m.view.GotoTop()
		view := ansi.Strip(m.View().Content)
		for j := range i + 1 {
			if !strings.Contains(view, fmt.Sprintf("Private thought %d", j)) || !strings.Contains(view, fmt.Sprintf("Public answer %d", j)) {
				t.Fatal("a completed turn lost its reasoning or final answer")
			}
		}
		request, err := json.Marshal(m.request)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(request), "Private thought") {
			t.Fatal("UI-only reasoning leaked into the model/persistence request")
		}
	}
	m.newConversation()
	if strings.Contains(m.view.GetContent(), "Private thought") {
		t.Fatal("new conversation retained another conversation's reasoning")
	}
}

func TestHistoryPreservesImagesAndToolExchanges(t *testing.T) {
	image, err := readAttachment("../samples/giraffe.jpg")
	if err != nil {
		t.Fatal(err)
	}
	request := model.D{"messages": append(model.ImageMessage("Name the animal", image.data, image.kind),
		model.D{"role": "assistant", "content": "", "tool_calls": []model.D{{"id": "call-1", "type": "function", "function": model.D{"name": "tool_read_file", "arguments": `{"path":"notes.txt"}`}}}},
		model.D{"role": "tool", "tool_call_id": "call-1", "name": "tool_read_file", "content": `{"status":"SUCCESS","data":{"file_contents":"713"}}`}),
		"tools": toolDocuments(), "temperature": 0.35}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().UTC()
	ref := imc.Ref{Key: imc.Key{Model: "vision", ID: "conversation"}, Revision: 1}
	snapshot := imc.Snapshot{Record: imc.Record{Version: 1, Summary: imc.Summary{
		Ref: ref, CreatedAt: stamp, SavedAt: stamp, Compatibility: [32]byte{1},
		Prefix: imc.Prefix{Length: 5, Hash: sha256.Sum256([]byte("prefix"))}},
		Request: encoded, Restore: json.RawMessage(`{}`), AssistantText: "A giraffe; project 713."}, Target: []byte{9, 2, 7}}
	directory := t.TempDir()
	store, err := disk.New(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = disk.New(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	restored, err := loadHistory(t.Context(), store, ref)
	if err != nil {
		t.Fatal(err)
	}
	messages := restored["messages"].([]model.D)
	if len(messages) != 4 || messages[3]["content"] != "A giraffe; project 713." || messages[2]["tool_call_id"] != "call-1" {
		t.Fatalf("history lost or duplicated a message: %v", messages)
	}
	tools, ok := restored["tools"].([]model.D)
	if !ok || len(tools) != 4 {
		t.Fatal("restored tools are not SDK documents")
	}
	if _, ok := tools[0]["function"].(model.D); !ok {
		t.Fatal("restored tool function is not an SDK document")
	}
	parts := messages[0]["content"].([]model.D)
	url := parts[0]["image_url"].(model.D)["url"]
	wantURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(image.data)
	if url != wantURL || restored["temperature"] != 0.35 {
		t.Fatal("history changed image bytes, settings or tools")
	}
	if got := messageText(messages[0]); got != "[image attached]\nName the animal" {
		t.Errorf("image display: got %q", got)
	}
	if got := messageText(model.D{"content": "hello\x1b[2J\x00world"}); got != "helloworld" {
		t.Errorf("unsafe terminal display: got %q", got)
	}
}

func TestFileTools(t *testing.T) {
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	}()
	call := func(name string, args model.ToolCallArguments) (any, error) {
		return callTool(t.Context(), root, model.ResponseToolCall{ID: "call", Function: model.ResponseToolCallFunction{Name: name, Arguments: args}})
	}
	if _, err := call("tool_create_file", model.ToolCallArguments{"path": "nested/main.go"}); err != nil {
		t.Fatal(err)
	}
	if _, err := call("tool_create_file", model.ToolCallArguments{"path": "nested/main.go"}); err == nil {
		t.Fatal("create overwrote an existing file")
	}
	if _, err := call("tool_go_code_editor", model.ToolCallArguments{"path": "nested/main.go", "line_number": 1, "type_change": "replace", "line_change": "package main\n\nconst code = 713\n"}); err != nil {
		t.Fatal(err)
	}
	result, err := call("tool_search_files", model.ToolCallArguments{"path": "nested", "filter": `\.go$`, "contains": "713"})
	if err != nil || !slices.Equal(result.(model.D)["files"].([]string), []string{"nested/main.go"}) {
		t.Fatalf("nested content search: got %v, %v", result, err)
	}
	result, err = call("tool_read_file", model.ToolCallArguments{"path": "nested/main.go"})
	if err != nil || !strings.Contains(result.(model.D)["file_contents"].(string), "3: const code = 713") {
		t.Fatalf("numbered file read: got %v, %v", result, err)
	}
	before, err := root.ReadFile("nested/main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range []model.ToolCallArguments{
		{"path": "nested/main.go", "line_number": 3, "type_change": "replace", "line_change": "not valid Go !"},
		{"path": "nested/main.go", "line_number": 0, "type_change": "delete"},
		{"path": "../escape.go", "line_number": 1, "type_change": "replace", "line_change": "package escape"},
	} {
		if _, err := call("tool_go_code_editor", args); err == nil {
			t.Errorf("accepted invalid edit: %v", args)
		}
	}
	after, err := root.ReadFile("nested/main.go")
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("invalid edit changed the file")
	}
	outside := filepath.Join(t.TempDir(), "private.txt")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := root.Symlink(outside, "escape"); err != nil {
		t.Fatal(err)
	}
	if _, err := call("tool_read_file", model.ToolCallArguments{"path": "escape"}); err == nil {
		t.Fatal("read through a symlink outside the workspace")
	}
}

// Opt-in native coverage drives the same Bubble Tea Update/command interface
// as the terminal. No scripted model answers or mocked inference are used.
func TestChatRestart(t *testing.T) {
	path := os.Getenv("KRONK_CHAT_TEST_MODEL")
	if path == "" {
		t.Skip("set KRONK_CHAT_TEST_MODEL to run native chat/restart coverage")
	}
	if err := kronk.Init(); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	for phase := range 2 {
		t.Run(fmt.Sprint(phase), func(t *testing.T) {
			store, err := disk.New(directory)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := store.Close(); err != nil {
					t.Error(err)
				}
			})
			krn, err := kronk.New(model.WithModelFiles([]string{path}), model.WithContextWindow(8192), model.WithNSeqMax(1),
				model.WithAutoTune(true), model.WithIncrementalCache(true), model.WithCacheMinTokens(1),
				model.WithSpeculationMode(model.SpeculationDisabled), model.WithStorage(model.StorageConfig{Store: store}))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := krn.Unload(context.Background()); err != nil {
					t.Error(err)
				}
			})
			root, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := root.Close(); err != nil {
					t.Error(err)
				}
			})
			app := newChat(krn, store, root, false)
			app.Init()
			app.Update(tea.WindowSizeMsg{Width: 100, Height: 32})
			drive := func(cmd tea.Cmd) {
				t.Helper()
				for cmd != nil && app.busy {
					_, cmd = app.Update(cmd())
				}
			}
			send := func(text string) {
				app.Update(tea.PasteMsg{Content: text})
				_, cmd := app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				drive(cmd)
			}
			if phase == 0 {
				send("Remember our project code is 713. Reply with only the number.")
			}
			if phase == 1 {
				send("/history")
				_, cmd := app.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				drive(cmd)
				send("What is our project code? Reply with only the number. Do not use tools.")
				if app.usage.PromptTokensDetails.CachedTokens == 0 {
					t.Fatal("restart did not reuse cached input tokens")
				}
			}
			view := ansi.Strip(app.View().Content)
			messages := app.request["messages"].([]model.D)
			answer, _ := messages[len(messages)-1]["content"].(string)
			if !strings.Contains(view, "saved revision") || strings.TrimSpace(answer) != "713" || strings.Contains(view, "Request failed") {
				t.Fatalf("chat did not complete and persist:\n%s", view)
			}
		})
	}
}
