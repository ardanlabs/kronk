package metrics

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestIMCSessionsCollector(t *testing.T) {
	collector := newIMCSessionsCollector()
	registry := prometheus.NewRegistry()
	if err := registry.Register(collector); err != nil {
		t.Fatalf("Register: unexpected error: %v", err)
	}

	var calls atomic.Int64
	provider := func() []IMCSession {
		calls.Add(1)
		return []IMCSession{
			{
				ModelID:       "test-model",
				Entry:         2,
				State:         "idle",
				Messages:      17,
				Context:       1024,
				Allocated:     2048,
				InputMessages: 19,
				InputTokens:   1200,
				OutputTokens:  300,
				PeakContext:   1600,
				Window:        4096,
				HasMedia:      true,
				LastUsed:      time.Unix(123, 500_000_000),
			},
			{
				ModelID: "test-model",
				Entry:   3,
				State:   "empty",
				Window:  4096,
			},
		}
	}

	unregister, err := collector.register(provider)
	if err != nil {
		t.Fatalf("register: unexpected error: %v", err)
	}

	if _, err := collector.register(provider); err == nil {
		t.Fatal("register: expected duplicate provider error")
	}

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather: unexpected error: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("provider calls: got %d, want 1", got)
	}

	tests := []struct {
		name   string
		labels map[string]string
		want   float64
	}{
		{
			name:   "imc_session_state",
			labels: map[string]string{"model_id": "test-model", "entry": "2", "state": "idle"},
			want:   1,
		},
		{
			name:   "imc_session_messages",
			labels: map[string]string{"model_id": "test-model", "entry": "2"},
			want:   17,
		},
		{
			name:   "imc_session_context_tokens",
			labels: map[string]string{"model_id": "test-model", "entry": "2"},
			want:   1024,
		},
		{
			name:   "imc_session_allocated_tokens",
			labels: map[string]string{"model_id": "test-model", "entry": "2"},
			want:   2048,
		},
		{
			name:   "imc_session_latest_request_messages",
			labels: map[string]string{"model_id": "test-model", "entry": "2"},
			want:   19,
		},
		{
			name:   "imc_session_latest_request_input_tokens",
			labels: map[string]string{"model_id": "test-model", "entry": "2"},
			want:   1200,
		},
		{
			name:   "imc_session_latest_request_output_tokens",
			labels: map[string]string{"model_id": "test-model", "entry": "2"},
			want:   300,
		},
		{
			name:   "imc_session_latest_request_context_tokens",
			labels: map[string]string{"model_id": "test-model", "entry": "2"},
			want:   1500,
		},
		{
			name:   "imc_session_peak_context_tokens",
			labels: map[string]string{"model_id": "test-model", "entry": "2"},
			want:   1600,
		},
		{
			name:   "imc_session_peak_used_percent",
			labels: map[string]string{"model_id": "test-model", "entry": "2"},
			want:   39.0625,
		},
		{
			name:   "imc_session_window_tokens",
			labels: map[string]string{"model_id": "test-model", "entry": "2"},
			want:   4096,
		},
		{
			name:   "imc_session_used_percent",
			labels: map[string]string{"model_id": "test-model", "entry": "2"},
			want:   25,
		},
		{
			name:   "imc_session_has_media",
			labels: map[string]string{"model_id": "test-model", "entry": "2"},
			want:   1,
		},
		{
			name:   "imc_session_last_used_timestamp_seconds",
			labels: map[string]string{"model_id": "test-model", "entry": "2"},
			want:   123.5,
		},
		{
			name:   "imc_session_last_used_timestamp_seconds",
			labels: map[string]string{"model_id": "test-model", "entry": "3"},
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metric := findMetric(families, tt.name, tt.labels)
			if metric == nil {
				t.Fatalf("metric %q with labels %v not found", tt.name, tt.labels)
			}
			if got := metric.GetGauge().GetValue(); got != tt.want {
				t.Errorf("value: got %v, want %v", got, tt.want)
			}
		})
	}
	unregister()
	unregister()

	families, err = registry.Gather()
	if err != nil {
		t.Fatalf("Gather after unregister: unexpected error: %v", err)
	}
	if metric := findMetric(families, "imc_session_context_tokens", map[string]string{"model_id": "test-model"}); metric != nil {
		t.Error("metric found after unregister")
	}
}

func TestIMCSessionsCollectorConcurrentRegistration(t *testing.T) {
	collector := newIMCSessionsCollector()
	registry := prometheus.NewRegistry()
	if err := registry.Register(collector); err != nil {
		t.Fatalf("Register: unexpected error: %v", err)
	}

	provider := func() []IMCSession {
		return []IMCSession{{ModelID: "test-model", Entry: 0, State: "empty"}}
	}

	done := make(chan error, 1)
	go func() {
		for range 100 {
			if _, err := registry.Gather(); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()

	for range 100 {
		unregister, err := collector.register(provider)
		if err != nil {
			t.Fatalf("register: unexpected error: %v", err)
		}
		unregister()
	}

	if err := <-done; err != nil {
		t.Fatalf("Gather: unexpected error: %v", err)
	}
}
