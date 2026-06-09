package libs

import "testing"

func TestVersionGreater(t *testing.T) {
	tests := []struct {
		name   string
		v1, v2 string
		want   bool
	}{
		{"equal", "v1.8.6", "v1.8.6", false},
		{"patch greater", "v1.8.6", "v1.8.4", true},
		{"patch lesser", "v1.8.4", "v1.8.6", false},
		{"minor greater", "v1.9.0", "v1.8.6", true},
		{"major greater", "v2.0.0", "v1.8.6", true},
		{"two-digit minor greater", "v1.10.0", "v1.8.6", true},
		{"two-digit minor lesser", "v1.8.6", "v1.10.0", false},
		{"two-digit patch greater", "v1.8.10", "v1.8.6", true},
		{"missing segment lesser", "v1.8", "v1.8.1", false},
		{"missing segment greater", "v1.8.1", "v1.8", true},
		{"missing segment equal", "v1.8", "v1.8.0", false},
		{"no prefix numeric", "1.8.6", "1.8.4", true},
		{"empty v1", "", "v1.8.6", false},
		{"empty v2", "v1.8.6", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := versionGreater(tt.v1, tt.v2)
			if got != tt.want {
				t.Errorf("versionGreater(%q, %q) = %v, want %v", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}
