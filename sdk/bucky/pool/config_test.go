package pool

import (
	"context"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
)

func TestValidateConfigTTL(t *testing.T) {
	tests := []struct {
		name    string
		ttl     time.Duration
		wantErr bool
	}{
		{name: "negative", ttl: -time.Second, wantErr: true},
		{name: "disabled"},
		{name: "enabled", ttl: time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateConfig(Config{
				Log:    func(context.Context, string, ...any) {},
				Models: &buckymodels.Models{},
				Resman: &resman.Manager{},
				TTL:    tt.ttl,
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got.TTL != tt.ttl {
				t.Errorf("TTL: got %s, want %s", got.TTL, tt.ttl)
			}
		})
	}
}
