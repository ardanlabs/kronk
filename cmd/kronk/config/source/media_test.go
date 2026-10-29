package source

import "testing"

func TestParseSize(t *testing.T) {
	for in, want := range map[string]int64{"75 MB": 75 << 20, "1.5 GB": 3 << 29, "0.5 MB": 1 << 19, "~2 GB": 2 << 30, "": 0, "big": 0} {
		if got := parseSize(in); got != want {
			t.Errorf("parseSize(%q) = %d, want %d", in, got, want)
		}
	}
	if got := FormatBytes(uint64(parseSize("1.5 GB"))); got != "1.5 GB" {
		t.Errorf("round trip = %q, the picker must show the catalog's size", got)
	}
}

func TestPullCommand(t *testing.T) {
	for _, tt := range []struct {
		c    Choice
		want string
	}{
		{Choice{ID: "unsloth/Qwen3-0.6B-Q8_0"}, "kronk model pull --local unsloth/Qwen3-0.6B-Q8_0"},
		{Choice{ID: "tiny", Kind: KindAudioModel}, "kronk bucky model pull --local tiny"},
		{Choice{ID: "sd-1.5", Kind: KindImageModel}, "kronk malina model pull --local sd-1.5"},
	} {
		if got := PullCommand(tt.c); got != tt.want {
			t.Errorf("PullCommand(%+v) = %q, want %q", tt.c, got, tt.want)
		}
	}
}
