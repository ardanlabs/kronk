package kronk

import (
	"testing"
	"time"
)

func TestValidateTimeoutConfig(t *testing.T) {
	tests := []struct {
		name      string
		inference time.Duration
		write     time.Duration
		wantErr   bool
	}{
		{name: "defaults", inference: 60 * time.Minute, write: 61 * time.Minute},
		{name: "write disabled", inference: time.Minute},
		{name: "inference zero", write: time.Minute, wantErr: true},
		{name: "inference negative", inference: -time.Second, write: time.Minute, wantErr: true},
		{name: "write equal", inference: time.Minute, write: time.Minute, wantErr: true},
		{name: "write shorter", inference: time.Minute, write: time.Second, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTimeoutConfig(tt.inference, tt.write)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateTimeoutConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAdminConfig(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	tests := []struct {
		name     string
		admin    bool
		web      bool
		password string
		host     string
		wantErr  bool
	}{
		{name: "open"},
		{name: "admin only", admin: true},
		{name: "open web admin", web: true},
		{name: "protected web admin", admin: true, web: true, password: sha},
		{name: "protected web missing password", admin: true, web: true, wantErr: true},
		{name: "inactive password", password: sha},
		{name: "inactive web password with external auth", web: true, password: sha, host: "auth:9000"},
		{name: "short digest", admin: true, password: "abcd", wantErr: true},
		{name: "non hex digest", admin: true, password: sha[:63] + "z", wantErr: true},
		{name: "external browser login", admin: true, web: true, password: sha, host: "auth:9000", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAdminConfig(tt.admin, tt.web, tt.password, tt.host)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateAdminConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
