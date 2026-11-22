package metrics

import (
	"math"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
)

func TestInferenceAndBatchSeqMetrics(t *testing.T) {
	const modelID = "metrics-test-embedding"

	AddInferenceActiveRequests(modelID, "embedding", "batchseq", 1)
	AddInferenceActiveRequests(modelID, "embedding", "batchseq", -1)
	ObserveInferenceRequest(modelID, "embedding", "batchseq", "ok", 25*time.Millisecond, 7)
	ObserveBatchSeqQueueWait(modelID, "embedding", 5*time.Millisecond)
	ObserveBatchSeqBatch(modelID, "embedding", "ok", 3)

	tests := []struct {
		name   string
		labels map[string]string
		value  func(*dto.Metric) float64
		want   float64
	}{
		{
			name:   "inference_requests_total",
			labels: map[string]string{"model_id": modelID, "operation": "embedding", "runtime": "batchseq", "status": "ok"},
			value:  func(m *dto.Metric) float64 { return m.GetCounter().GetValue() },
			want:   1,
		},
		{
			name:   "inference_active_requests",
			labels: map[string]string{"model_id": modelID, "operation": "embedding", "runtime": "batchseq"},
			value:  func(m *dto.Metric) float64 { return m.GetGauge().GetValue() },
			want:   0,
		},
		{
			name:   "gen_ai_server_request_duration_seconds",
			labels: map[string]string{"gen_ai_operation_name": "embeddings", "gen_ai_request_model": modelID, "error_type": "", "runtime": "batchseq", "gen_ai_provider_name": "kronk"},
			value:  func(m *dto.Metric) float64 { return float64(m.GetHistogram().GetSampleCount()) },
			want:   1,
		},
		{
			name:   "gen_ai_client_inference_usage_input_tokens_total",
			labels: map[string]string{"gen_ai_operation_name": "embeddings", "gen_ai_request_model": modelID, "gen_ai_provider_name": "kronk", "gen_ai_token_modality": "unknown"},
			value:  func(m *dto.Metric) float64 { return m.GetCounter().GetValue() },
			want:   7,
		},
		{
			name:   "batchseq_queue_wait_seconds",
			labels: map[string]string{"model_id": modelID, "operation": "embedding"},
			value:  func(m *dto.Metric) float64 { return float64(m.GetHistogram().GetSampleCount()) },
			want:   1,
		},
		{
			name:   "batchseq_items",
			labels: map[string]string{"model_id": modelID, "operation": "embedding"},
			value:  func(m *dto.Metric) float64 { return m.GetHistogram().GetSampleSum() },
			want:   3,
		},
		{
			name:   "batchseq_batches_total",
			labels: map[string]string{"model_id": modelID, "operation": "embedding", "status": "ok"},
			value:  func(m *dto.Metric) float64 { return m.GetCounter().GetValue() },
			want:   1,
		},
	}

	families, err := Gatherer().Gather()
	if err != nil {
		t.Fatalf("Gather: unexpected error: %v", err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metric := findMetric(families, tt.name, tt.labels)
			if metric == nil {
				t.Fatalf("metric %q with labels %v not found", tt.name, tt.labels)
			}
			if got := tt.value(metric); got != tt.want {
				t.Errorf("value: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPoolGaugesAreSeparatedByBackend(t *testing.T) {
	SetPoolItemsInPool("kronk-test", 2)
	SetPoolItemsInPool("bucky-test", 3)
	SetPoolMaxItemsInPool("kronk-test", 4)
	SetPoolMaxItemsInPool("bucky-test", 5)
	SetPoolInflightLoads("kronk-test", 6)
	SetPoolInflightLoads("bucky-test", 7)

	tests := []struct {
		name    string
		backend string
		want    float64
	}{
		{name: "pool_items_in_pool", backend: "kronk-test", want: 2},
		{name: "pool_items_in_pool", backend: "bucky-test", want: 3},
		{name: "pool_max_items_in_pool", backend: "kronk-test", want: 4},
		{name: "pool_max_items_in_pool", backend: "bucky-test", want: 5},
		{name: "pool_inflight_loads", backend: "kronk-test", want: 6},
		{name: "pool_inflight_loads", backend: "bucky-test", want: 7},
	}

	families, err := Gatherer().Gather()
	if err != nil {
		t.Fatalf("Gather: unexpected error: %v", err)
	}
	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.backend, func(t *testing.T) {
			metric := findMetric(families, tt.name, map[string]string{"backend": tt.backend})
			if metric == nil {
				t.Fatalf("metric %q with backend %q not found", tt.name, tt.backend)
			}
			if got := metric.GetGauge().GetValue(); got != tt.want {
				t.Errorf("value: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChatGenAIMetrics(t *testing.T) {
	const modelID = "metrics-test-chat-genai"

	AddChatCompletionsUsage(modelID, 31, 3, 7, 10, 41, 10)
	AddChatCompletionsUsage(modelID, 13, 2, 5, 7, 20, 100)
	AddChatCompletionsUsage(modelID+"-single", 11, 0, 1, 1, 12, 0)
	AddRequestTTFT(modelID, 125*time.Millisecond)
	ObserveChatRequestDuration(modelID, "", 250*time.Millisecond)
	ObserveChatRequestDuration(modelID, "active-slot", 500*time.Millisecond)
	ObserveChatRequestDuration(modelID, "context-cancelled", 750*time.Millisecond)

	tests := []struct {
		name     string
		modality string
		want     float64
	}{
		{"gen_ai_client_inference_usage_input_tokens_total", "unknown", 44},
		{"gen_ai_client_inference_usage_output_tokens_total", "text", 17},
		{"gen_ai_client_inference_usage_reasoning_output_tokens_total", "", 5},
	}

	families, err := Gatherer().Gather()
	if err != nil {
		t.Fatalf("Gather: unexpected error: %v", err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels := map[string]string{"gen_ai_operation_name": "chat", "gen_ai_request_model": modelID, "gen_ai_provider_name": "kronk"}
			if tt.modality != "" {
				labels["gen_ai_token_modality"] = tt.modality
			}
			metric := findMetric(families, tt.name, labels)
			if metric == nil {
				t.Fatalf("metric %q with labels %v not found", tt.name, labels)
			}
			if got := metric.GetCounter().GetValue(); got != tt.want {
				t.Errorf("counter: got %v, want %v", got, tt.want)
			}
			if got := len(metric.Label); got != len(labels) {
				t.Errorf("label count: got %d, want %d", got, len(labels))
			}
		})
	}

	labels := map[string]string{"gen_ai_operation_name": "chat", "gen_ai_request_model": modelID, "gen_ai_provider_name": "kronk"}
	tpot := findMetric(families, "gen_ai_server_time_per_output_token_seconds", labels)
	if tpot == nil {
		t.Fatal("TPOT histogram not found")
	}
	if got := tpot.GetHistogram().GetSampleCount(); got != 2 {
		t.Errorf("TPOT count: got %d, want 2", got)
	}
	if got := tpot.GetHistogram().GetSampleSum(); math.Abs(got-0.11) > 1e-12 {
		t.Errorf("TPOT sum: got %v, want 0.11", got)
	}
	ttft := findMetric(families, "gen_ai_server_time_to_first_token_seconds", labels)
	if ttft == nil || ttft.GetHistogram().GetSampleCount() != 1 || ttft.GetHistogram().GetSampleSum() != 0.125 {
		t.Errorf("TTFT: got %v, want one sample of 0.125 seconds", ttft)
	}
	labels["gen_ai_request_model"] = modelID + "-single"
	if metric := findMetric(families, "gen_ai_server_time_per_output_token_seconds", labels); metric != nil {
		t.Errorf("single-token TPOT: got %v, want no observation", metric)
	}

	for _, tt := range []struct {
		errorType string
		want      float64
	}{
		{"", 0.25},
		{"active-slot", 0.5},
		{"context-cancelled", 0.75},
	} {
		labels := map[string]string{"gen_ai_operation_name": "chat", "gen_ai_request_model": modelID, "gen_ai_provider_name": "kronk", "runtime": "batch", "error_type": tt.errorType}
		metric := findMetric(families, "gen_ai_server_request_duration_seconds", labels)
		if metric == nil || metric.GetHistogram().GetSampleCount() != 1 || metric.GetHistogram().GetSampleSum() != tt.want {
			t.Errorf("duration %q: got %v, want one sample of %v seconds", tt.errorType, metric, tt.want)
		}
	}
}

func findMetric(families []*dto.MetricFamily, name string, labels map[string]string) *dto.Metric {
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.Metric {
			matched := 0
			for _, pair := range metric.Label {
				if labels[pair.GetName()] == pair.GetValue() {
					matched++
				}
			}
			if matched == len(labels) {
				return metric
			}
		}
	}
	return nil
}
