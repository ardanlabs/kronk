package website

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ardanlabs/kronk"
)

type Config struct {
	KRNChat    *kronk.Kronk
	KRNTimeout time.Duration
}

func WebAPI(cfg Config) http.Handler {
	mux := http.NewServeMux()

	rts := handlers{
		krnChat: cfg.KRNChat,
		timeout: cfg.KRNTimeout,
	}

	mux.HandleFunc("POST /chat/completions", rts.chat)

	return corsMiddleware(mux)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func sendError(w http.ResponseWriter, traceID string, context string, err error) {
	fmt.Printf("traceID: %s: chat: %s: ERROR: %s\n", traceID, context, err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
