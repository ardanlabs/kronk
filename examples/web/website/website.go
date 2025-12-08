// Package website provides api and web ui for the chatbot.
package website

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"regexp"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/model"
	"github.com/google/uuid"
)

//go:embed static
var website embed.FS

const (
	websiteDir  = "static"
	websitePath = "/"
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

	if _, err := h.krnChat.ChatCompletions(ctx, log, w, d); err != nil {
		sendError(w, traceID, "streamResponse", err)
		return
	}
}

func (h *handlers) fileServerReact() func(w http.ResponseWriter, r *http.Request) {
	fileMatcher := regexp.MustCompile(`\.[a-zA-Z]*$`)

	fSys, err := fs.Sub(website, websiteDir)
	if err != nil {
		fmt.Printf("switching to static folder: %s", err)
		return nil
	}

	fileServer := http.StripPrefix(websitePath, http.FileServer(http.FS(fSys)))

	f := func(w http.ResponseWriter, r *http.Request) {
		if !fileMatcher.MatchString(r.URL.Path) {
			p, err := website.ReadFile(fmt.Sprintf("%s/index.html", websiteDir))
			if err != nil {
				fmt.Printf("FileServerReact: index.html not found: %v\n", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(p)
			return
		}

		fileServer.ServeHTTP(w, r)
	}

	return f
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
