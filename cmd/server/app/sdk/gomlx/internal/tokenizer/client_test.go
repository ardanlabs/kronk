package tokenizer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestClientTokenize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/tokenize" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		var body struct {
			Content      string `json:"content"`
			AddSpecial   bool   `json:"add_special"`
			ParseSpecial bool   `json:"parse_special"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Content != "<bos>hello" || body.AddSpecial || !body.ParseSpecial {
			t.Errorf("body = %+v", body)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"tokens": []int32{2, 10, 11}})
	}))
	defer server.Close()

	client, err := New(server.URL+"/", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := client.Tokenize(t.Context(), "<bos>hello")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(tokens, []int32{2, 10, 11}) {
		t.Fatalf("tokens = %v", tokens)
	}
}

func TestClientTokenizeRejectsBadResponses(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		response string
		match    string
	}{
		{"server error", http.StatusInternalServerError, "unavailable", "500"},
		{"malformed", http.StatusOK, `{`, "decode"},
		{"empty", http.StatusOK, `{"tokens":[]}`, "no token IDs"},
		{"negative", http.StatusOK, `{"tokens":[2,-1]}`, "must not be negative"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(test.response))
			}))
			defer server.Close()
			client, err := New(server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Tokenize(t.Context(), "content")
			if err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("error = %v, want containing %q", err, test.match)
			}
		})
	}
}

func TestClientVerifyModel(t *testing.T) {
	modelPath := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(modelPath, []byte("model"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/props" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		_ = json.NewEncoder(writer).Encode(map[string]string{"model_path": modelPath})
	}))
	defer server.Close()
	client, err := New(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := client.VerifyModel(t.Context(), modelPath); err != nil {
		t.Fatal(err)
	}

	otherPath := filepath.Join(t.TempDir(), "other.gguf")
	if err := os.WriteFile(otherPath, []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := client.VerifyModel(t.Context(), otherPath); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatch error = %v", err)
	}
}
