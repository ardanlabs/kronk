package pool

import (
	"strings"
	"testing"
)

func TestCustomModelID(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		want    string
		wantErr string
	}{
		{name: "accuracy", key: "owner/model/custom/accuracy", want: "owner/model"},
		{name: "efficiency profile", key: "owner/model/profile/custom/efficiency", want: "owner/model/profile"},
		{name: "preparation", key: "owner/model/custom/preparation", want: "owner/model"},
		{name: "playground", key: "owner/model/playground/session", want: "owner/model"},
		{name: "training", key: "owner/model/training/run", want: "owner/model"},
		{name: "training profile", key: "owner/model/profile/training/session", want: "owner/model/profile"},
		{name: "training nested suffix", key: "owner/model/training/run/session", want: "owner/model"},
		{name: "training model name", key: "owner/training/training/run", want: "owner/training"},
		{name: "training suffix resembling playground", key: "owner/model/training/run/playground/session", want: "owner/model"},
		{name: "playground suffix resembling training", key: "owner/model/playground/session/training/run", want: "owner/model"},
		{name: "training suffix resembling workload", key: "owner/model/training/run/custom/accuracy", want: "owner/model"},
		{name: "training empty suffix", key: "owner/model/training/", wantErr: "invalid key"},
		{name: "training profile empty suffix", key: "owner/model/profile/training/", wantErr: "invalid key"},
		{name: "training missing suffix", key: "owner/model/training", wantErr: "collides"},
		{name: "training invalid model", key: "model/training/run", wantErr: "collides"},
		{name: "training invalid canonical id", key: "owner/model/profile/extra/training/run", wantErr: "invalid model id"},
		{name: "catalog collision", key: "owner/model/accuracy", wantErr: "collides"},
		{name: "unknown workload", key: "owner/model/custom/unknown", wantErr: "invalid key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := customModelID(tt.key)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("customModelID: got %q, %v, want error containing %q", got, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("customModelID: %v", err)
			}
			if got != tt.want {
				t.Errorf("customModelID: got %q, want %q", got, tt.want)
			}
		})
	}
}
