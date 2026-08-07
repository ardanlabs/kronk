package auth_test

import (
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
)

func TestMode(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    auth.Mode
		wantErr bool
	}{
		{name: "open", value: "open", want: auth.Open},
		{name: "management", value: "management", want: auth.Management},
		{name: "authenticated", value: "authenticated", want: auth.Authenticated},
		{name: "full protected", value: "full-protected", want: auth.FullProtected},
		{name: "unknown", value: "unknown", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := auth.ParseMode(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseMode() error = %v, wantErr %t", err, tt.wantErr)
			}
			if !got.Equal(tt.want) {
				t.Errorf("ParseMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestModeText(t *testing.T) {
	var mode auth.Mode
	if err := mode.UnmarshalText([]byte("authenticated")); err != nil {
		t.Fatalf("UnmarshalText() error = %v", err)
	}
	if !mode.Equal(auth.Authenticated) {
		t.Fatalf("UnmarshalText() = %q, want %q", mode, auth.Authenticated)
	}

	text, err := mode.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText() error = %v", err)
	}
	if got, want := string(text), "authenticated"; got != want {
		t.Errorf("MarshalText() = %q, want %q", got, want)
	}

	if err := mode.UnmarshalText([]byte("invalid")); err == nil {
		t.Fatal("UnmarshalText() error = nil, want an error")
	}
}
