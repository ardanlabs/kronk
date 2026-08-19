package model

import (
	"testing"
	"time"
)

func TestConfigAdmissionTimeout(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want time.Duration
	}{
		{
			name: "default",
			cfg:  NewConfig(),
			want: 3 * time.Minute,
		},
		{
			name: "configured",
			cfg:  NewConfig(WithAdmissionTimeout(time.Second)),
			want: time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.WithDefaults().AdmissionTimeout
			if got != tt.want {
				t.Errorf("AdmissionTimeout: got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestConfigQueueDepth(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want int
	}{
		{
			name: "default",
			cfg:  NewConfig(),
			want: 0,
		},
		{
			name: "configured",
			cfg:  NewConfig(WithQueueDepth(2)),
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.WithDefaults().QueueDepth
			if got != tt.want {
				t.Errorf("QueueDepth: got %d, want %d", got, tt.want)
			}
		})
	}
}
