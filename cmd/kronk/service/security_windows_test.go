package service

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestServiceDataSecurityPolicy(t *testing.T) {
	for _, test := range []struct {
		name    string
		sddl    string
		wantErr bool
	}{
		{"protected data", "O:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FA;;;LS)", false},
		{"service owns file", "O:LSD:(A;;FA;;;LS)(A;;FA;;;BA)", false},
		{"untrusted owner", "O:WDD:(A;;FA;;;LS)", true},
		{"untrusted explicit access", "O:BAD:(A;;FA;;;LS)(A;;FW;;;WD)", true},
		{"no account access", "O:BAD:(A;;FA;;;BA)", true},
		{"account read only", "O:BAD:(A;;FR;;;LS)", true},
		{"inherit only", "O:BAD:(A;OIIO;FA;;;LS)", true},
		{"empty permissions", "O:BAD:", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			sd, err := windows.SecurityDescriptorFromString(test.sddl)
			if err != nil {
				t.Fatal(err)
			}
			err = validateDataSecurity(sd, "S-1-5-19")
			if (err != nil) != test.wantErr {
				t.Fatalf("data permissions: got %v, want error=%t", err, test.wantErr)
			}
		})
	}
}
