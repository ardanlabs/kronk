package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/imc"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
)

type chat struct {
	krn       *kronk.Kronk
	store     imc.Store
	modelID   string
	root      *os.Root
	vision    bool
	input     textarea.Model
	view      viewport.Model
	width     int
	height    int
	dragging  bool
	dragGrab  int
	selecting bool
	logPath   string
	showLog   bool
	request   model.D
	image     attachment
	status    string
	busy      bool
	quitting  bool
	ctx       context.Context
	cancel    context.CancelFunc
	prompt    string
	sentImage attachment
	stream    <-chan model.ChatResponse
	answer    string
	reasoning string
	// UI-only reasoning is never added to the model request or disk history.
	reasoningHistory map[int]string
	toolCalls        []model.ResponseToolCall
	toolIndex        int
	approval         bool
	rounds           int
	usage            model.Usage
	streamErr        error
	started          time.Time
	ref              imc.Ref
	history          bool
	page             imc.Page
	cursors          []string
	selected         int
}

type streamOpened struct {
	stream <-chan model.ChatResponse
	err    error
}

type streamChunk struct {
	response model.ChatResponse
	closed   bool
}

type historyPage struct {
	page imc.Page
	err  error
}

type historyLoaded struct {
	request model.D
	ref     imc.Ref
	err     error
}

type savedResult struct {
	summary imc.Summary
	err     error
}

type clipboardCopied struct{ err error }

func newChat(krn *kronk.Kronk, store imc.Store, root *os.Root, vision bool) *chat {
	input := textarea.New()
	input.Placeholder = "Message, /history, /new, /log, /select, /exit, /image PATH, /help"
	input.ShowLineNumbers = false
	input.CharLimit = 0
	input.SetHeight(3)
	input.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "shift+enter"))
	view := viewport.New()
	view.SoftWrap = true
	m := chat{krn: krn, store: store, modelID: krn.ModelInfo().ID, vision: vision,
		root: root, input: input, view: view, width: 80, height: 24}
	m.newConversation()
	return &m
}

func (m *chat) newConversation() {
	m.request = model.D{"messages": []model.D{{"role": "system", "content": systemPrompt}}, "max_tokens": 4096,
		"temperature": 0.0, "top_p": 0.1, "top_k": 1,
		"tools": toolDocuments(), "tool_choice": "auto",
		"stream_options": model.D{"include_usage": true}}
	m.ref, m.image, m.answer = imc.Ref{}, attachment{}, ""
	m.reasoningHistory = make(map[int]string)
	m.showLog, m.dragging = false, false
	m.input.Reset()
	m.status = "New conversation. Completed replies are saved automatically."
	m.refresh(true)
}

func (m *chat) Init() tea.Cmd { return m.input.Focus() }

func (m *chat) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		bottom := m.view.AtBottom()
		m.dragging = false
		m.width, m.height = msg.Width, msg.Height
		m.input.SetWidth(max(10, m.width-4))
		m.view.SetWidth(max(10, m.width-6))
		m.view.SetHeight(max(1, m.height-12))
		m.refresh(bottom)
		return m, nil

	case tea.MouseMsg:
		if !m.selecting {
			m.mouse(msg)
		}
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "f2":
			m.selecting = !m.selecting
			m.dragging = false
			m.status = "Mouse scrolling enabled. F2 switches to text selection."
			if m.selecting {
				m.status = "Text selection enabled. Use terminal Copy/Paste; F2 restores mouse scrolling."
			}
			return m, nil
		case "ctrl+q":
			if m.busy {
				m.quitting = true
				if m.cancel != nil {
					m.cancel()
				}
				m.status = "Finishing active operation before exit..."
				if m.approval {
					return m.finishTurn()
				}
				return m, nil
			}
			return m, tea.Quit
		case "ctrl+c":
			if m.busy {
				if m.cancel != nil {
					m.cancel()
				}
				m.status = "Cancelling; waiting for the stream to close..."
				if m.approval {
					return m.finishTurn()
				}
				return m, nil
			}
			return m, tea.Quit
		case "pgup":
			m.view.PageUp()
			return m, nil
		case "pgdown":
			m.view.PageDown()
			return m, nil
		}
		if m.approval {
			switch msg.String() {
			case "y":
				m.approval = false
				return m, m.runTool()
			case "n", "esc":
				m.approval = false
				call := m.toolCalls[m.toolIndex]
				return m.Update(toolFinished{toolResponse(call, nil, errors.New("user denied this file change"))})
			}
			return m, nil
		}
		if m.busy {
			return m, nil
		}
		if m.history {
			return m.historyKey(msg.String())
		}
		if msg.String() == "enter" {
			return m.submit()
		}

	case streamOpened:
		if msg.err != nil {
			m.streamErr = msg.err
			return m.finishTurn()
		}
		m.stream = msg.stream
		return m, nextChunk(m.stream)

	case streamChunk:
		if msg.closed {
			return m.finishTurn()
		}
		if msg.response.Usage != nil {
			m.usage = *msg.response.Usage
		}
		for _, choice := range msg.response.Choices {
			switch choice.FinishReason() {
			case model.FinishReasonError:
				m.streamErr = errors.New(choice.Delta.Content)
			case model.FinishReasonTool:
				m.toolCalls = choice.Message.ToolCalls
				// Until beginTools appends the call message, there are no
				// outstanding tool results to add on cancellation.
				m.toolIndex = len(m.toolCalls)
			default:
				if choice.Delta != nil {
					m.answer += choice.Delta.Content
					m.reasoning += choice.Delta.Reasoning
				}
			}
		}
		m.refresh(m.view.AtBottom())
		return m, nextChunk(m.stream)

	case toolFinished:
		messages := m.request["messages"].([]model.D)
		m.request["messages"] = append(messages, msg.response)
		m.toolIndex++
		return m.nextTool()

	case clipboardCopied:
		m.status = "Log path copied to clipboard. Paste it into your message."
		if msg.err != nil {
			m.status = "Clipboard unavailable: " + msg.err.Error() + "; use F2 to select the log path above."
		}
		return m, nil

	case savedResult:
		m.busy = false
		m.status = fmt.Sprintf("cached %d | no new snapshot; see disk log | prompt %d | output %d",
			m.usage.PromptTokensDetails.CachedTokens, m.usage.PromptTokens, m.usage.CompletionTokens)
		if msg.err == nil && !msg.summary.SavedAt.Before(m.started) {
			m.ref = msg.summary.Ref
			m.status = fmt.Sprintf("cached %d | saved revision %d | prompt %d | output %d | %.1f tok/s",
				m.usage.PromptTokensDetails.CachedTokens, m.ref.Revision, m.usage.PromptTokens, m.usage.CompletionTokens, m.usage.TokensPerSecond)
		}
		if msg.err != nil {
			m.status += ": " + msg.err.Error()
		}
		if m.quitting {
			return m, tea.Quit
		}
		return m, m.input.Focus()

	case historyPage:
		m.busy = false
		if m.quitting {
			return m, tea.Quit
		}
		if msg.err != nil {
			m.status = "History: " + msg.err.Error() + "; Esc to return, /history to refresh"
			return m, nil
		}
		m.page, m.selected = msg.page, 0
		m.status = "Enter opens messages; the next turn attempts automatic IMC restore."
		m.refresh(true)
		m.view.GotoTop()
		return m, nil

	case historyLoaded:
		m.busy = false
		if m.quitting {
			return m, tea.Quit
		}
		if msg.err != nil {
			m.status = "Open: " + msg.err.Error() + "; Esc, then /history to refresh"
			return m, nil
		}
		m.request, m.ref, m.history = msg.request, msg.ref, false
		m.reasoningHistory = make(map[int]string)
		m.image, m.answer = attachment{}, ""
		m.status = "History loaded. Send a message to attempt checkpoint reuse."
		m.refresh(true)
		return m, m.input.Focus()
	}
	if m.busy || m.history {
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *chat) submit() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return m, nil
	}
	m.input.Reset()
	switch {
	case text == "/quit", text == "/exit":
		return m, tea.Quit
	case text == "/new":
		m.newConversation()
		return m, nil
	case text == "/log":
		m.showLog = true
		m.status = "Log path shown above. Share this file to diagnose the current run."
		m.refresh(true)
		path := m.logPath
		return m, func() tea.Msg { return clipboardCopied{clipboard.WriteAll(path)} }
	case text == "/select":
		return m.Update(tea.KeyPressMsg{Code: tea.KeyF2})
	case text == "/history":
		m.history, m.cursors = true, []string{""}
		m.input.Blur()
		return m, m.listPage()
	case text == "/clear-image":
		m.image = attachment{}
		m.status = "Attachment cleared."
		return m, nil
	case strings.HasPrefix(text, "/image "):
		if !m.vision {
			m.status = "Images require a vision model and -projector."
			return m, nil
		}
		image, err := readAttachment(strings.TrimSpace(strings.TrimPrefix(text, "/image ")))
		if err != nil {
			m.status = "Image: " + err.Error()
			return m, nil
		}
		m.image = image
		m.status = "Image ready. Type the question to send with it."
		return m, nil
	case strings.HasPrefix(text, "/"):
		m.status = "/new | /history | /log | /select | /exit | /image PATH | /clear-image | /quit. Alt+Enter: newline."
		return m, nil
	}
	messages := m.request["messages"].([]model.D)
	message := model.D{"role": "user", "content": text}
	if len(m.image.data) != 0 {
		message = model.ImageMessage(text, m.image.data, m.image.kind)[0]
	}
	m.request["messages"] = append(messages, message)
	m.prompt, m.sentImage = text, m.image
	m.image, m.answer, m.usage, m.streamErr = attachment{}, "", model.Usage{}, nil
	m.busy, m.rounds = true, 0
	m.showLog = false
	m.input.Blur()
	m.refresh(true)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	m.ctx, m.cancel = ctx, cancel
	return m, m.startStream()
}

func nextChunk(stream <-chan model.ChatResponse) tea.Cmd {
	return func() tea.Msg {
		response, ok := <-stream
		return streamChunk{response, !ok}
	}
}

func (m *chat) finishTurn() (tea.Model, tea.Cmd) {
	// Always reach channel closure before changing conversations or unloading.
	if m.streamErr == nil {
		m.streamErr = m.ctx.Err()
	}
	if m.streamErr == nil && len(m.toolCalls) != 0 {
		return m.beginTools()
	}
	m.cancel()
	m.cancel, m.stream, m.approval = nil, nil, false
	if m.streamErr != nil {
		messages := m.request["messages"].([]model.D)
		if m.rounds == 0 {
			// No tools ran, so the input can be retried without side effects.
			m.input.SetValue(m.prompt)
			m.image = m.sentImage
			m.request["messages"] = messages[:len(messages)-1]
		}
		if m.rounds != 0 {
			// Keep completed tool results: retrying blindly could repeat writes.
			for _, call := range m.toolCalls[m.toolIndex:] {
				messages = append(messages, toolResponse(call, nil, errors.New("request cancelled before tool execution")))
			}
			m.request["messages"] = messages
		}
		m.answer, m.reasoning, m.busy = "", "", false
		m.prompt, m.sentImage = "", attachment{}
		m.status = "Request failed/cancelled: " + m.streamErr.Error()
		if m.rounds != 0 {
			m.status = "Stopped. Completed tool changes were not rolled back; send a new message to continue."
		}
		m.refresh(true)
		if m.quitting {
			return m, tea.Quit
		}
		return m, m.input.Focus()
	}
	messages := m.request["messages"].([]model.D)
	if m.reasoning != "" {
		m.reasoningHistory[len(messages)] = m.reasoning
	}
	m.request["messages"] = append(messages, model.D{"role": "assistant", "content": m.answer})
	m.answer, m.reasoning, m.prompt, m.sentImage = "", "", "", attachment{}
	m.refresh(m.view.AtBottom())
	m.status = "Response complete; checking saved history..."
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		page, err := m.store.List(ctx, imc.ListQuery{Model: m.modelID, Limit: 1})
		var summary imc.Summary
		if len(page.Sessions) != 0 {
			summary = page.Sessions[0]
		}
		return savedResult{summary, err}
	}
}

func (m *chat) listPage() tea.Cmd {
	m.busy, m.status = true, "Loading history..."
	cursor := m.cursors[len(m.cursors)-1]
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		page, err := m.store.List(ctx, imc.ListQuery{Model: m.modelID, Limit: 8, Cursor: cursor})
		return historyPage{page, err}
	}
}

func (m *chat) historyKey(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "esc":
		m.history = false
		m.status = "Returned to current conversation."
		m.refresh(true)
		return m, m.input.Focus()
	case "up", "k":
		m.selected = max(0, m.selected-1)
	case "down", "j":
		m.selected = min(max(0, len(m.page.Sessions)-1), m.selected+1)
	case "n":
		if m.page.NextCursor != "" {
			m.cursors = append(m.cursors, m.page.NextCursor)
			return m, m.listPage()
		}
	case "p":
		if len(m.cursors) > 1 {
			m.cursors = m.cursors[:len(m.cursors)-1]
			return m, m.listPage()
		}
	case "enter":
		if len(m.page.Sessions) != 0 {
			ref := m.page.Sessions[m.selected].Ref
			m.busy, m.status = true, "Opening conversation..."
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				request, err := loadHistory(ctx, m.store, ref)
				return historyLoaded{request, ref, err}
			}
		}
	}
	m.refresh(false)
	m.view.EnsureVisible(2+m.selected*3, 0, 0)
	return m, nil
}

func (m *chat) refresh(bottom bool) {
	var text strings.Builder
	label := lipgloss.NewStyle().Bold(true)
	reasoningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5F5F"))
	toolStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#5FFF87"))
	if m.history {
		fmt.Fprintf(&text, "Saved conversations · page %d\n\n", len(m.cursors))
		if len(m.page.Sessions) == 0 {
			text.WriteString("No saved conversations for this model. Esc returns to chat.\n")
		}
		for i, session := range m.page.Sessions {
			prefix := "  "
			if i == m.selected {
				prefix = "> "
			}
			header := fmt.Sprintf("%s%s · revision %d · %s", prefix,
				session.SavedAt.Local().Format("Jan 02 15:04"), session.Ref.Revision, terminalText(session.Ref.Key.ID))
			fmt.Fprintf(&text, "%s\n  %s\n\n", ansi.Truncate(header, m.view.Width(), "…"),
				ansi.Truncate(strings.Join(strings.Fields(terminalText(session.Preview)), " "), max(10, m.view.Width()-4), "…"))
		}
	} else {
		messages := m.request["messages"].([]model.D)
		if len(messages) == 1 {
			text.WriteString("Start a conversation, or use /history to open a saved one.\n\n")
			text.WriteString("Attach a PNG or JPEG with /image /path/to/file, then send a question.\n")
			text.WriteString("Images are stored with the messages; the original file is not needed later.\n")
		}
		for i, message := range messages {
			role, _ := message["role"].(string)
			if role == "system" {
				continue
			}
			if reasoning := m.reasoningHistory[i]; reasoning != "" {
				text.WriteString(reasoningStyle.Bold(true).Render("reasoning (not saved)"))
				text.WriteString("\n")
				text.WriteString(reasoningStyle.Render(terminalText(reasoning)))
				text.WriteString("\n\n")
			}
			text.WriteString(label.Render(terminalText(role)))
			text.WriteString("\n")
			text.WriteString(messageText(message))
			if calls, ok := message["tool_calls"]; ok {
				data, err := json.Marshal(calls)
				callText := "[unsupported tool call]"
				if err == nil {
					callText = terminalText(string(data))
				}
				text.WriteString(toolStyle.Render("\nTool calls: " + callText))
			}
			text.WriteString("\n\n")
		}
		if m.reasoning != "" && m.busy {
			text.WriteString(reasoningStyle.Bold(true).Render("reasoning (live; not saved)"))
			text.WriteString("\n")
			text.WriteString(reasoningStyle.Render(terminalText(m.reasoning)))
			text.WriteString("\n\n")
		}
		if m.answer != "" {
			text.WriteString(label.Render("assistant"))
			text.WriteString("\n")
			text.WriteString(terminalText(m.answer))
		}
		if m.approval {
			call := m.toolCalls[m.toolIndex]
			fmt.Fprintf(&text, "\n%s\n%s\n%s\n", label.Render("Approve file change? y allow · n deny"),
				toolStyle.Render(terminalText(call.Function.Name)), toolStyle.Render(terminalText(fmt.Sprint(call.Function.Arguments))))
		}
	}
	if m.showLog {
		fmt.Fprintf(&text, "\nLog file for this run:\n%s\n", terminalText(m.logPath))
	}
	m.view.SetContent(text.String())
	if bottom {
		m.view.GotoBottom()
	}
}

func (m *chat) View() tea.View {
	if m.width < 40 || m.height < 18 {
		v := tea.NewView("Please resize to at least 40 columns × 18 rows. Ctrl+Q exits.")
		v.AltScreen = true
		return v
	}
	title := lipgloss.NewStyle().Bold(true).Render("KRONK / saved chat")
	info := terminalText(m.modelID)
	if m.ref.Key.ID != "" {
		info += fmt.Sprintf(" · revision %d", m.ref.Revision)
	}
	composer := m.input.View()
	help := "Ctrl+Q quit · F2 select · Enter send · PgUp/PgDn scroll · Alt+Enter newline"
	if m.history {
		composer = "↑/↓ select · Enter open · Esc back\nn next page · p previous page\n"
		help = "Ctrl+Q quit · F2 select · Latest snapshot per conversation"
	}
	if m.selecting {
		help = "F2 mouse controls · Text selection enabled · Ctrl+Q quit"
	}
	attachment := "No image attached"
	if m.image.name != "" {
		attachment = "Image ready: " + terminalText(m.image.name) + " · /clear-image removes it"
	}
	conversation := m.view.View()
	if top, thumb := m.scrollbar(); thumb > 0 {
		trackStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#3D4654"))
		thumbStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
		bar := make([]string, m.view.Height())
		for i := range bar {
			bar[i] = trackStyle.Render("│")
			if i >= top && i < top+thumb {
				bar[i] = thumbStyle.Render("█")
			}
		}
		conversation = lipgloss.JoinHorizontal(lipgloss.Top, conversation, " ", strings.Join(bar, "\n"))
	}
	line := strings.Repeat("─", max(1, m.width-4))
	content := strings.Join([]string{title, ansi.Truncate(info, m.width-4, "…"), line,
		conversation, line, ansi.Truncate(attachment, m.width-4, "…"), composer,
		ansi.Truncate(strings.Join(strings.Fields(terminalText(m.status)), " "), m.width-4, "…"), ansi.Truncate(help, m.width-4, "…")}, "\n")
	v := tea.NewView(lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Padding(1, 2).Render(content))
	v.AltScreen = true
	if !m.selecting {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}
