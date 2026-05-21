package backend_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/tools/backend"
)

// fakeLibs is a minimal LibsManager used to exercise the registry
// without depending on any concrete backend.
type fakeLibs struct{}

func (fakeLibs) LibsPath() string                              { return "/tmp/fake" }
func (fakeLibs) Arch() string                                  { return "arm64" }
func (fakeLibs) OS() string                                    { return "darwin" }
func (fakeLibs) Processor() string                             { return "metal" }
func (fakeLibs) ReadOnly() bool                                { return false }
func (fakeLibs) SupportedCombinations() []backend.Combination  { return nil }
func (fakeLibs) IsSupported(string, string, string) bool       { return false }
func (fakeLibs) InstalledVersion() (backend.VersionTag, error) { return backend.VersionTag{}, nil }
func (fakeLibs) InstalledFor(string, string, string) (backend.VersionTag, error) {
	return backend.VersionTag{}, nil
}
func (fakeLibs) List() ([]backend.VersionTag, error) { return nil, nil }
func (fakeLibs) Download(context.Context, applog.Logger) (backend.VersionTag, error) {
	return backend.VersionTag{}, nil
}
func (fakeLibs) DownloadFor(context.Context, applog.Logger, string, string, string, string) (backend.VersionTag, error) {
	return backend.VersionTag{}, nil
}
func (fakeLibs) Remove(string, string, string) error { return nil }

// fakeCatalog is a minimal Catalog used to exercise the registry.
type fakeCatalog struct{ base string }

func (c fakeCatalog) Path() string                       { return c.base + "/models" }
func (c fakeCatalog) BasePath() string                   { return c.base }
func (fakeCatalog) BuildIndex(applog.Logger, bool) error { return nil }
func (fakeCatalog) Download(context.Context, applog.Logger, string) (backend.ModelPath, error) {
	return backend.ModelPath{}, nil
}
func (fakeCatalog) FullPath(string) (backend.ModelPath, error)    { return backend.ModelPath{}, nil }
func (fakeCatalog) Remove(backend.ModelPath, applog.Logger) error { return nil }

func TestRegister_Validation(t *testing.T) {
	tests := []struct {
		name string
		in   backend.Backend
		want string
	}{
		{
			name: "empty kind",
			in:   backend.Backend{},
			want: "backend: register: kind is required",
		},
		{
			name: "missing NewLibs",
			in: backend.Backend{
				Kind:       "test-empty-libs",
				NewCatalog: func(string) (backend.Catalog, error) { return fakeCatalog{}, nil },
			},
			want: `backend: register: "test-empty-libs": NewLibs is required`,
		},
		{
			name: "missing NewCatalog",
			in: backend.Backend{
				Kind:    "test-empty-catalog",
				NewLibs: func() (backend.LibsManager, error) { return fakeLibs{}, nil },
			},
			want: `backend: register: "test-empty-catalog": NewCatalog is required`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := backend.Register(tt.in)
			if err == nil || err.Error() != tt.want {
				t.Errorf("Register: got %v, want %q", err, tt.want)
			}
		})
	}
}

func TestRegister_RoundTrip(t *testing.T) {
	const kind = "test-fake"

	b := backend.Backend{
		Kind:       kind,
		NewLibs:    func() (backend.LibsManager, error) { return fakeLibs{}, nil },
		NewCatalog: func(basePath string) (backend.Catalog, error) { return fakeCatalog{base: basePath}, nil },
	}

	if err := backend.Register(b); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, ok := backend.Get(kind)
	if !ok {
		t.Fatalf("Get(%q): not found after Register", kind)
	}
	if got.Kind != kind {
		t.Errorf("Get.Kind: got %q, want %q", got.Kind, kind)
	}

	lm, err := got.NewLibs()
	if err != nil {
		t.Fatalf("NewLibs: %v", err)
	}
	if lm.LibsPath() != "/tmp/fake" {
		t.Errorf("LibsPath: got %q, want /tmp/fake", lm.LibsPath())
	}

	cat, err := got.NewCatalog("/tmp/base")
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	if cat.BasePath() != "/tmp/base" {
		t.Errorf("BasePath: got %q, want /tmp/base", cat.BasePath())
	}

	kinds := backend.Kinds()
	if !slices.Contains(kinds, kind) {
		t.Errorf("Kinds: %v missing %q", kinds, kind)
	}
}

func TestRegister_Idempotent(t *testing.T) {
	const kind = "test-replace"

	first := backend.Backend{
		Kind:       kind,
		NewLibs:    func() (backend.LibsManager, error) { return nil, errors.New("first") },
		NewCatalog: func(string) (backend.Catalog, error) { return fakeCatalog{}, nil },
	}
	second := backend.Backend{
		Kind:       kind,
		NewLibs:    func() (backend.LibsManager, error) { return fakeLibs{}, nil },
		NewCatalog: func(string) (backend.Catalog, error) { return fakeCatalog{}, nil },
	}

	if err := backend.Register(first); err != nil {
		t.Fatalf("Register first: %v", err)
	}
	if err := backend.Register(second); err != nil {
		t.Fatalf("Register second: %v", err)
	}

	got, _ := backend.Get(kind)

	lm, err := got.NewLibs()
	if err != nil {
		t.Fatalf("NewLibs from replaced backend: %v", err)
	}
	if lm == nil {
		t.Errorf("NewLibs: got nil, want fakeLibs from second registration")
	}
}
