package modelprofile_test

import (
	"encoding"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
	yaml "go.yaml.in/yaml/v2"
	yamlv3 "go.yaml.in/yaml/v3"
)

func TestRegisteredStringValues(t *testing.T) {
	for _, tt := range []struct {
		value any
		want  string
	}{
		{modelprofile.Class{}, ""},
		{modelprofile.ClassDense, "dense"},
		{modelprofile.ClassMoE, "moe"},
		{modelprofile.ClassHybrid, "hybrid"},
		{modelprofile.Role{}, ""},
		{modelprofile.RoleLanguage, "language"},
		{modelprofile.RoleVisionEncoder, "vision_encoder"},
		{modelprofile.Purpose{}, ""},
		{modelprofile.PurposeGeneration, "generation"},
		{modelprofile.PurposeEmbedding, "embedding"},
		{modelprofile.PurposeRerank, "rerank"},
		{modelprofile.MemorySemantics{}, ""},
		{modelprofile.MemoryAttention, "attention"},
		{modelprofile.MemoryRecurrent, "recurrent"},
		{modelprofile.MTPArchitectureNone, ""},
		{modelprofile.MTPArchitectureQwen35OwnKV, "qwen35-own-kv"},
		{modelprofile.MTPArchitectureQwen4ExpOwnKV, "qwen4exp-own-kv"},
		{modelprofile.MTPArchitectureNemotronOwnKV, "nemotron-own-kv"},
		{modelprofile.MTPArchitectureGemmaSharedKV, "gemma-shared-kv"},
		{modelprofile.MTPArchitectureGLM5Next, "glm5-next"},
		{modelprofile.BatchInputMixingUnknown, ""},
		{modelprofile.BatchInputMixingSupported, "supported"},
		{modelprofile.BatchInputMixingUnsupported, "unsupported"},
	} {
		t.Run(reflect.TypeOf(tt.value).Name()+"/"+tt.want, func(t *testing.T) {
			if got := tt.value.(interface{ String() string }).String(); got != tt.want {
				t.Errorf("String: got %q, want %q", got, tt.want)
			}
			if got := tt.value.(interface{ IsZero() bool }).IsZero(); got != (tt.want == "") {
				t.Errorf("IsZero: got %t, want %t", got, tt.want == "")
			}
			data, err := tt.value.(encoding.TextMarshaler).MarshalText()
			if err != nil || string(data) != tt.want {
				t.Fatalf("text: got %q, %v, want %q, nil", data, err, tt.want)
			}
			decoded := reflect.New(reflect.TypeOf(tt.value))
			if err := decoded.Interface().(encoding.TextUnmarshaler).UnmarshalText(data); err != nil {
				t.Fatal(err)
			}
			if got := decoded.Elem().Interface(); got != tt.value {
				t.Errorf("text round trip: got %v, want %v", got, tt.value)
			}
			if err := decoded.Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte("invalid")); err == nil || decoded.Elem().Interface() != tt.value {
				t.Fatal("invalid text did not fail atomically")
			}
			for _, codec := range []struct {
				name      string
				marshal   func(any) ([]byte, error)
				unmarshal func([]byte, any) error
			}{
				{"json", json.Marshal, json.Unmarshal},
				{"yaml2", yaml.Marshal, yaml.Unmarshal},
				{"yaml3", yamlv3.Marshal, yamlv3.Unmarshal},
			} {
				t.Run(codec.name, func(t *testing.T) {
					data, err := codec.marshal(tt.value)
					if err != nil {
						t.Fatal(err)
					}
					var wire string
					if err := codec.unmarshal(data, &wire); err != nil || wire != tt.want {
						t.Fatalf("wire: got %q, %v, want %q, nil", wire, err, tt.want)
					}
					decoded := reflect.New(reflect.TypeOf(tt.value))
					if err := codec.unmarshal(data, decoded.Interface()); err != nil {
						t.Fatal(err)
					}
					if got := decoded.Elem().Interface(); got != tt.value {
						t.Errorf("round trip: got %v, want %v", got, tt.value)
					}
					invalid, err := codec.marshal("invalid")
					if err != nil {
						t.Fatal(err)
					}
					if err := codec.unmarshal(invalid, decoded.Interface()); err == nil || decoded.Elem().Interface() != tt.value {
						t.Fatal("invalid wire value did not fail atomically")
					}
				})
			}
		})
	}
}

func TestComputeKindNumericWireValues(t *testing.T) {
	members := []modelprofile.ComputeKind{
		modelprofile.ComputeGeneric, modelprofile.ComputeIndexed, modelprofile.ComputeGemma4,
		modelprofile.ComputeQwen35, modelprofile.ComputeGPTOSS, modelprofile.ComputeMistral3,
		modelprofile.ComputeLFM2, modelprofile.ComputeGLM4MoE, modelprofile.ComputeGemma3,
		modelprofile.ComputeLlama, modelprofile.ComputeClef, modelprofile.ComputeModernBERT,
	}
	for index, member := range members {
		if member.Value() != uint8(index) || !modelprofile.MustParseComputeKind(uint8(index)).Equal(member) {
			t.Fatalf("member %d: got %d, want %d", index, member.Value(), index)
		}
		for _, codec := range []struct {
			name      string
			marshal   func(any) ([]byte, error)
			unmarshal func([]byte, any) error
		}{
			{"json", json.Marshal, json.Unmarshal},
			{"yaml2", yaml.Marshal, yaml.Unmarshal},
			{"yaml3", yamlv3.Marshal, yamlv3.Unmarshal},
		} {
			t.Run(codec.name+"/"+member.String(), func(t *testing.T) {
				data, err := codec.marshal(member)
				if err != nil {
					t.Fatal(err)
				}
				var number uint8
				if err := codec.unmarshal(data, &number); err != nil || number != uint8(index) {
					t.Fatalf("wire: got %d, %v, want %d, nil", number, err, index)
				}
				var decoded modelprofile.ComputeKind
				if err := codec.unmarshal(data, &decoded); err != nil || !decoded.Equal(member) {
					t.Fatalf("round trip: got %v, %v, want %v, nil", decoded, err, member)
				}
				if err := codec.unmarshal([]byte("12"), &decoded); err == nil || !decoded.Equal(member) {
					t.Fatal("unregistered numeric ID did not fail atomically")
				}
			})
		}
	}
	for _, input := range []string{"-1", "256", "1.5", "\"1\"", "{}"} {
		value := modelprofile.ComputeIndexed
		if err := json.Unmarshal([]byte(input), &value); err == nil || !value.Equal(modelprofile.ComputeIndexed) {
			t.Errorf("invalid JSON %s did not fail atomically", input)
		}
	}
	if value := (modelprofile.ComputeKind{}); !value.IsZero() || !value.Equal(modelprofile.ComputeGeneric) {
		t.Fatal("zero compute kind is not generic")
	}
}

func TestProfileValueOmission(t *testing.T) {
	type values struct {
		Class   modelprofile.Class            `json:"class,omitzero" yaml:"class,omitempty"`
		Role    modelprofile.Role             `json:"role,omitzero" yaml:"role,omitempty"`
		Purpose modelprofile.Purpose          `json:"purpose,omitzero" yaml:"purpose,omitempty"`
		Memory  modelprofile.MemorySemantics  `json:"memory,omitzero" yaml:"memory,omitempty"`
		MTP     modelprofile.MTPArchitecture  `json:"mtp,omitzero" yaml:"mtp,omitempty"`
		Mixing  modelprofile.BatchInputMixing `json:"mixing,omitzero" yaml:"mixing,omitempty"`
		Compute modelprofile.ComputeKind      `json:"compute,omitzero" yaml:"compute,omitempty"`
	}
	for _, marshal := range []func(any) ([]byte, error){json.Marshal, yaml.Marshal, yamlv3.Marshal} {
		data, err := marshal(values{})
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := yaml.Unmarshal(data, &fields); err != nil || len(fields) != 0 {
			t.Fatalf("zero fields: got %s, %v, want empty object", data, err)
		}
		data, err = marshal(values{
			Class: modelprofile.ClassDense, Role: modelprofile.RoleLanguage,
			Purpose: modelprofile.PurposeGeneration, Memory: modelprofile.MemoryAttention,
			MTP: modelprofile.MTPArchitectureQwen35OwnKV, Mixing: modelprofile.BatchInputMixingSupported,
			Compute: modelprofile.ComputeIndexed,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := yaml.Unmarshal(data, &fields); err != nil || len(fields) != 7 {
			t.Fatalf("nonzero fields: got %s, %v, want seven fields", data, err)
		}
	}
}

func TestMustParseRejectsUnknownValues(t *testing.T) {
	for _, tt := range []struct {
		name  string
		parse func()
	}{
		{"class", func() { modelprofile.MustParseClass("invalid") }},
		{"role", func() { modelprofile.MustParseRole("invalid") }},
		{"purpose", func() { modelprofile.MustParsePurpose("invalid") }},
		{"memory", func() { modelprofile.MustParseMemorySemantics("invalid") }},
		{"mtp", func() { modelprofile.MustParseMTPArchitecture("invalid") }},
		{"mixing", func() { modelprofile.MustParseBatchInputMixing("invalid") }},
		{"compute", func() { modelprofile.MustParseComputeKind(255) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("MustParse did not panic")
				}
			}()
			tt.parse()
		})
	}
}
