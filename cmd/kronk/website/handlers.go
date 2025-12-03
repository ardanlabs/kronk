package website

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/model"
	"github.com/google/uuid"
)

type handlers struct {
	krnEmbed *kronk.Kronk
	krnChat  *kronk.Kronk
	timeout  time.Duration
	db       *sql.DB
}

func (h *handlers) chat(w http.ResponseWriter, r *http.Request) {
	traceID := uuid.NewString()

	fmt.Printf("traceID: %s: chat: started\n", traceID)
	defer fmt.Printf("traceID: %s: chat: complete\n", traceID)

	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, traceID, "NewDecoder", err)
		return
	}

	fmt.Printf("traceID: %s: chat: msgs: %#v\n", traceID, req.Messages)

	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()

	log := func(ctx context.Context, format string, a ...any) {
		fmt.Printf("traceID: %s: chat: log: %s\n", traceID, fmt.Sprintf(format, a...))
	}

	params := getParams(traceID, req)

	d := model.D{
		"messages": h.compileChatMessages(traceID, req),
		"tools": []model.D{
			{
				"type": "function",
				"function": model.D{
					"name":        "get_weather",
					"description": "Get the current weather for a location",
					"arguments": model.D{
						"location": model.D{
							"type":        "string",
							"description": "The location to get the weather for, e.g. San Francisco, CA",
						},
					},
				},
			},
		},
	}

	model.AddParams(params, d)

	if err := h.krnChat.ChatStreamingHTTP(ctx, log, w, d); err != nil {
		sendError(w, traceID, "streamResponse", err)
		return
	}
}

// =============================================================================

func (h *handlers) compileChatMessages(traceID string, req Request) []model.D {
	fmt.Printf("traceID: %s: compileChatMessages: started: msgs: %d\n", traceID, len(req.Messages))

	const systemPrompt = `
		- Use any provided Context to answer the user's question.
		- If you don't know the answer, say that you don't know.
		- Responses should be properly formatted to be easily read.
		- Share code if code is presented in the context.
		- If relavant Context is available, use it to answer the question and don't include any additional information not present in the Context.
	`

	// Add 2 more elements for the system prompt and any context.
	msgs := make([]model.D, 0, len(req.Messages)+2)

	// Add the system prompt.
	msgs = append(msgs, model.TextMessage("system", systemPrompt))

	// Add all but the very last message in the history.
	for _, msg := range req.Messages[:len(req.Messages)-1] {
		msgs = append(msgs, model.TextMessage(msg.Role, msg.Content))
	}

	// Add the final message from the history. We expect this to be a question.
	question := req.Messages[len(req.Messages)-1].Content
	msgs = append(msgs, model.TextMessage("user", fmt.Sprintf("Question:\n%s\n\n", question)))

	fmt.Printf("traceID: %s: compileChatMessages: ended: msgs: %d\n", traceID, len(msgs))

	return msgs
}
