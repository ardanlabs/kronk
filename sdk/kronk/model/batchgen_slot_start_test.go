package model

import (
	"context"
	"testing"
)

func TestDisableMTPForMediaMRoPE(t *testing.T) {
	tests := []struct {
		name     string
		draft    drafter
		useMRoPE bool
		want     string
	}{
		{name: "embedded MTP with media M-RoPE", draft: &mtpDrafter{}, useMRoPE: true, want: "media-mrope"},
		{name: "embedded MTP with linear media", draft: &mtpDrafter{}, useMRoPE: false},
		{name: "classic draft with media M-RoPE", draft: &classicDrafter{}, useMRoPE: true},
		{name: "no draft with media M-RoPE", useMRoPE: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{
				draft: tt.draft,
				log:   func(context.Context, string, ...any) {},
			}
			e := batchEngine{model: &m}
			s := slot{useMRoPE: tt.useMRoPE}
			job := chatJob{ctx: t.Context(), id: "request-id"}

			e.disableMTPForMediaMRoPE(&s, &job)

			if s.mtp.DisableReason != tt.want {
				t.Errorf("disable reason = %q, want %q", s.mtp.DisableReason, tt.want)
			}
			if s.mtp.Disabled != (tt.want != "") {
				t.Errorf("disabled = %t, want %t", s.mtp.Disabled, tt.want != "")
			}
		})
	}
}
