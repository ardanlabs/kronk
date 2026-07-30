package metrics

import (
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
			name:   "inference_request_duration_seconds",
			labels: map[string]string{"model_id": modelID, "operation": "embedding", "runtime": "batchseq"},
			value:  func(m *dto.Metric) float64 { return float64(m.GetHistogram().GetSampleCount()) },
			want:   1,
		},
		{
			name:   "usage_tokens_total",
			labels: map[string]string{"model_id": modelID, "kind": "prompt"},
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
