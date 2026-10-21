package chatapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func TestCompareEmbeddingVectors(t *testing.T) {
	tests := []struct {
		name    string
		want    []float32
		got     []float32
		others  [][]float32
		wantErr bool
	}{
		{name: "identical", want: []float32{1, 0}, got: []float32{1, 0}},
		{name: "Vulkan sized drift", want: []float32{1, 0}, got: []float32{0.9998, 0.02}},
		{name: "above cosine threshold", want: []float32{1, 0}, got: []float32{0.9991, float32(math.Sqrt(1 - 0.9991*0.9991))}},
		{name: "below cosine threshold", want: []float32{1, 0}, got: []float32{0.9989, float32(math.Sqrt(1 - 0.9989*0.9989))}, wantErr: true},
		{name: "opposite", want: []float32{1, 0}, got: []float32{-1, 0}, wantErr: true},
		{name: "dimension mismatch", want: []float32{1, 0}, got: []float32{1}, wantErr: true},
		{name: "zero vector", want: []float32{0, 0}, got: []float32{0, 0}, wantErr: true},
		{name: "nonfinite output", want: []float32{1, 0}, got: []float32{float32(math.NaN()), 0}, wantErr: true},
		{name: "nonfinite reference", want: []float32{float32(math.Inf(1)), 0}, got: []float32{1, 0}, wantErr: true},
		{name: "intended reference closest", want: []float32{1, 0}, got: []float32{1, 0}, others: [][]float32{{0.9998, 0.02}, {0, 1}}},
		{name: "swapped nearby input", want: []float32{1, 0}, got: []float32{0.9998, 0.02}, others: [][]float32{{0, 1}, {0.9998, 0.02}}, wantErr: true},
		{name: "ambiguous identity", want: []float32{1, 0}, got: []float32{1, 0}, others: [][]float32{{1, 0}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := testlib.CompareEmbeddingVectors(tt.want, tt.got, tt.others...); (err != nil) != tt.wantErr {
				t.Fatalf("comparison: got error %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}

// Operation checks run inside their existing serial model groups. Concurrent
// calls here exercise request isolation, not concurrent model loading.
func operationIntegrity(t *testing.T, test *apitest.Test, token, id, path string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	krn, err := test.Pool.Kronk.AquireModel(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("operation %s: %s / %s / %s; %s", id, krn.ModelInfo().Type,
		krn.ModelInfo().SequenceProcessing, krn.ModelInfo().Architecture, krn.ModelConfig().String())

	var handlers atomic.Int64
	victimFinished := make(chan error, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlers.Add(1)
		defer handlers.Add(-1)
		if r.Header.Get("X-Test-Cancel") == "true" {
			defer func() { victimFinished <- r.Context().Err() }()
		}
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

	post := func(ctx context.Context, d model.D, victim bool) ([]byte, error) {
		d["model"] = id
		body, err := json.Marshal(d)
		if err != nil {
			return nil, err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+path, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		if victim {
			request.Header.Set("X-Test-Cancel", "true")
		}
		response, err := client.Do(request)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s: status %d: %s", path, response.StatusCode, data)
		}
		return data, nil
	}
	embed := func(ctx context.Context, d model.D) (model.EmbedReponse, error) {
		var response model.EmbedReponse
		data, err := post(ctx, d, false)
		if err == nil {
			err = json.Unmarshal(data, &response)
		}
		return response, err
	}
	rerank := func(ctx context.Context, d model.D) (model.RerankResponse, error) {
		var response model.RerankResponse
		data, err := post(ctx, d, false)
		if err == nil {
			err = json.Unmarshal(data, &response)
		}
		return response, err
	}
	if path == "/v1/embeddings" {
		t.Run("batch-and-concurrent-identity", func(t *testing.T) { testlib.TestEmbeddingIntegrity(t, id, embed) })
	} else {
		t.Run("batch-and-concurrent-identity", func(t *testing.T) { testlib.TestRerankIntegrity(t, id, rerank) })
	}

	t.Run("cancel-with-surviving-peer", func(t *testing.T) {
		wait := func(label string, predicate func() bool) {
			t.Helper()
			deadline, stop := context.WithTimeout(ctx, 10*time.Second)
			defer stop()
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for !predicate() {
				select {
				case <-deadline.Done():
					t.Fatalf("%s: %v; active=%d handlers=%d", label, deadline.Err(), krn.ActiveStreams(), handlers.Load())
				case <-ticker.C:
				}
			}
		}
		request := func(text string, count int) model.D {
			inputs := make([]string, count)
			for i := range inputs {
				inputs[i] = text
			}
			if path == "/v1/embeddings" {
				return model.D{"input": inputs}
			}
			return model.D{"query": "What is the capital of France?", "documents": inputs, "return_documents": true}
		}
		const peerText = "Paris is the capital and largest city of France."
		reference, err := post(ctx, request(peerText, 1), false)
		if err != nil {
			t.Fatal(err)
		}
		check := func(data []byte, count int) {
			t.Helper()
			if path == "/v1/embeddings" {
				var response, baseline model.EmbedReponse
				if err := json.Unmarshal(data, &response); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(reference, &baseline); err != nil {
					t.Fatal(err)
				}
				if err := testlib.CheckEmbeddings(baseline, id, 1, embeddingDimension); err != nil {
					t.Fatal(err)
				}
				if err := testlib.CheckEmbeddings(response, id, count, embeddingDimension); err != nil {
					t.Fatal(err)
				}
				for i, result := range response.Data {
					if err := testlib.CompareEmbeddingVectors(baseline.Data[0].Embedding, result.Embedding); err != nil {
						t.Fatalf("peer/recovery embedding[%d]: %v", i, err)
					}
				}
				return
			}
			var response, baseline model.RerankResponse
			if err := json.Unmarshal(data, &response); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(reference, &baseline); err != nil {
				t.Fatal(err)
			}
			if err := testlib.CheckRerank(baseline, id, []string{peerText}, 0, true); err != nil {
				t.Fatal(err)
			}
			documents := request(peerText, count)["documents"].([]string)
			if err := testlib.CheckRerank(response, id, documents, 0, true); err != nil {
				t.Fatal(err)
			}
			for i, result := range response.Data {
				if math.Abs(float64(result.RelevanceScore-baseline.Data[0].RelevanceScore)) > 1e-3 {
					t.Fatalf("peer/recovery rank[%d]: score %g, want %g within 1e-3", i, result.RelevanceScore, baseline.Data[0].RelevanceScore)
				}
			}
		}
		check(reference, 1)

		victimCtx, stopVictim := context.WithCancel(ctx)
		defer stopVictim()
		victimResult := make(chan error, 1)
		go func() {
			_, err := post(victimCtx, request("Berlin is the capital of Germany.", 4096), true)
			victimResult <- err
		}()
		wait("victim admitted", func() bool { return krn.ActiveStreams() == 1 })
		type result struct {
			data []byte
			err  error
		}
		peer := make(chan result, 1)
		go func() {
			data, err := post(ctx, request(peerText, 128), false)
			peer <- result{data, err}
		}()
		// Admission proves two live SDK operations, not native decode-batch overlap.
		// Shared-batch cancellation is covered separately by the engine unit tests.
		wait("victim and peer admitted", func() bool { return krn.ActiveStreams() == 2 && handlers.Load() == 2 })
		stopVictim()
		select {
		case err := <-victimResult:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled client: got %v, want context.Canceled", err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		select {
		case err := <-victimFinished:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("server request context: got %v, want context.Canceled", err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		select {
		case result := <-peer:
			if result.err != nil {
				t.Fatalf("surviving peer: %v", result.err)
			}
			check(result.data, 128)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		wait("cancelled operation and peer release capacity", func() bool { return krn.ActiveStreams() == 0 && handlers.Load() == 0 })
		data, err := post(ctx, request(peerText, 1), false)
		if err != nil {
			t.Fatalf("recovery: %v", err)
		}
		check(data, 1)
		wait("recovery releases capacity", func() bool { return krn.ActiveStreams() == 0 && handlers.Load() == 0 })
		t.Log("client cancellation reached handler; peer and recovery matched isolated reference; no active operations remain")
	})
}
