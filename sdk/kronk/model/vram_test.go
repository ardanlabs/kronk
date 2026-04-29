package model

import "testing"

func TestParseMetadataInt64OrArrayAvg(t *testing.T) {
	tests := []struct {
		name    string
		val     string
		want    int64
		wantErr bool
	}{
		{name: "scalar", val: "8", want: 8},
		// 8*5 + 2 = 42, 42/6 = 7 (integer division).
		{name: "space-array", val: "[8 8 8 8 8 2]", want: 7},
		{name: "comma-array (llama.cpp gguf_kv_to_str)", val: "[8, 8, 8, 8, 8, 2]", want: 7},
		// 8 + 8 + 8 + 2 = 26, 26/4 = 6.
		{name: "mixed-whitespace-array", val: "[ 8 ,8 , 8 , 2 ]", want: 6},
		{name: "single-element-array", val: "[16]", want: 16},
		{name: "empty-array", val: "[]", wantErr: true},
		{name: "missing-key", val: "", wantErr: true},
		{name: "non-int", val: "abc", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			md := map[string]string{}
			if tc.val != "" {
				md["k"] = tc.val
			}

			got, err := parseMetadataInt64OrArrayAvg(md, "k")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got value=%d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}
