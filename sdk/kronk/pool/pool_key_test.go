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
		{name: "playground", key: "owner/model/playground/session", want: "owner/model"},
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
