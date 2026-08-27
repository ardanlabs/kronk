package libs

import "testing"

func TestOptionsValidate(t *testing.T) {
	tests := []struct {
		name    string
		opts    options
		wantErr bool
	}{
		{name: "default"},
		{name: "install", opts: options{install: true, arch: "arm64", opSys: "darwin", processor: "metal"}},
		{name: "missing triple", opts: options{install: true}, wantErr: true},
		{name: "multiple operations", opts: options{install: true, listInstalls: true}, wantErr: true},
		{name: "upgrade explicit operation", opts: options{listInstalls: true, upgrade: true}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.opts.validate(); (err != nil) != tt.wantErr {
				t.Errorf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
