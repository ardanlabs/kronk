package speculation

import (
	"encoding"
	"reflect"
	"testing"
)

func TestRegisteredTypes(t *testing.T) {
	for _, value := range []any{
		Mode{}, ModeAuto, ModeDisabled, ModeClassic, ModeMTP,
		SourceNone, SourceClassic, SourceMTP,
		MTPArchitectureNone, MTPArchitectureQwen35OwnKV, MTPArchitectureQwen4ExpOwnKV,
		MTPArchitectureNemotronOwnKV, MTPArchitectureGemmaSharedKV, MTPArchitectureGLM5Next,
		MTPArtifactNone, MTPArtifactEmbedded, MTPArtifactCompanion,
	} {
		data, err := value.(encoding.TextMarshaler).MarshalText()
		if err != nil {
			t.Fatal(err)
		}
		decoded := reflect.New(reflect.TypeOf(value))
		parser := decoded.Interface().(encoding.TextUnmarshaler)
		if err := parser.UnmarshalText(data); err != nil {
			t.Fatal(err)
		}
		if got := decoded.Elem().Interface(); got != value {
			t.Errorf("text round trip: got %v, want %v", got, value)
		}
		if err := parser.UnmarshalText([]byte("future")); err == nil {
			t.Errorf("%T accepted unknown value", value)
		}
		if got := decoded.Elem().Interface(); got != value {
			t.Errorf("invalid input changed value: got %v, want %v", got, value)
		}
	}
	if !(Source{}).Equal(SourceNone) || !(Source{}).IsZero() {
		t.Error("source zero must be none")
	}
	if !(MTPArchitecture{}).Equal(MTPArchitectureNone) || !(MTPArchitecture{}).IsZero() {
		t.Error("architecture zero must be none")
	}
	if !(MTPArtifact{}).Equal(MTPArtifactNone) || !(MTPArtifact{}).IsZero() {
		t.Error("artifact zero must be none")
	}
}
