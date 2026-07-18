package kronk

import "testing"

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
		{name: "password without admin", password: sha, wantErr: true},
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
