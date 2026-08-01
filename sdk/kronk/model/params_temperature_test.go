package model

import (
	"context"
	"fmt"
	"testing"
)

// newTempTestModel builds the minimum *Model needed to exercise the
// parameter-resolution path. No GGUF is loaded: parseParams / adjustParams
// only touch m.cfg, m.log and m.parser.
func newTempTestModel() *Model {
	return &Model{
		cfg: Config{
			PtrContextWindow: new(4096),
		},
		log: func(ctx context.Context, msg string, args ...any) {},
	}
}

// TestParseParamsExplicitTemperatureZeroIsPreserved pins the temperature-0
// rewrite bug described in findings2 §6a.
//
// sdk/kronk/model/params.go:725-727
//
//	if p.Temperature <= 0 {
//		p.Temperature = DefTemp
//	}
//
// adjustParams cannot distinguish "temperature was not supplied" (the zero
// value of Params.Temperature) from "the caller explicitly asked for 0". An
// explicit "temperature": 0 in the request document is therefore silently
// rewritten to DefTemp (0.8), which makes deterministic / greedy decoding
// unreachable through the public API.
//
// Greedy decoding is not cosmetic here: verifySpeculativeTokens
// (batchgen_speculative.go:335) selects its verification branch with
// `greedy := temperature == 0`, so the classic-drafter greedy verify path is
// dead code as long as this rewrite stands, and no temperature-0 equivalence
// test can be written for MTP.
//
// The non-zero and unset sub-tests are controls: they document the behaviour
// that must NOT change when the guard is fixed.
func TestParseParamsExplicitTemperatureZeroIsPreserved(t *testing.T) {
	tests := []struct {
		name string
		doc  D
		want float32
	}{
		{name: "explicit zero float64", doc: D{"temperature": float64(0)}, want: 0},
		{name: "explicit zero float32", doc: D{"temperature": float32(0)}, want: 0},
		{name: "explicit zero int", doc: D{"temperature": 0}, want: 0},
		{name: "explicit zero string", doc: D{"temperature": "0"}, want: 0},
		{name: "explicit non-zero", doc: D{"temperature": 0.25}, want: 0.25},
		{name: "unset falls back to DefTemp", doc: D{}, want: DefTemp},
	}

	m := newTempTestModel()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// parseParams mirrors resolved values back into d, so render
			// the request document before the call.
			input := fmt.Sprintf("%#v", tt.doc)

			p, err := m.parseParams(tt.doc)
			if err != nil {
				t.Fatalf("parseParams(%s): unexpected error: %v", input, err)
			}

			if p.Temperature != tt.want {
				t.Errorf("parseParams(%s): resolved Temperature = %v, want %v\n"+
					"params.go:725-727 rewrites any temperature <= 0 to DefTemp (%v), so an\n"+
					"explicitly requested temperature of 0 cannot reach the sampler and\n"+
					"deterministic/greedy decoding is unreachable via the public API.\n"+
					"The guard must only apply when the caller did not supply a temperature.",
					input, p.Temperature, tt.want, DefTemp)
			}
		})
	}
}

// TestAdjustParamsKeepsExplicitTemperatureZero pins the same defect one layer
// down, at sdk/kronk/model/params.go:725-727, without going through document
// parsing. Config.DefaultParams flows straight into adjustParams, so a
// deployment that configures a deterministic default temperature of 0 is
// silently overridden to DefTemp too.
func TestAdjustParamsKeepsExplicitTemperatureZero(t *testing.T) {
	m := newTempTestModel()
	m.cfg.DefaultParams = Params{Temperature: 0}

	got := m.adjustParams(m.cfg.DefaultParams)

	if got.Temperature != 0 {
		t.Errorf("adjustParams(Params{Temperature: 0}).Temperature = %v, want 0\n"+
			"params.go:725-727 uses `p.Temperature <= 0` as the not-set sentinel and\n"+
			"rewrites a deliberate 0 to DefTemp (%v). Temperature 0 is a valid, meaningful\n"+
			"request (greedy decoding), not an absent value.",
			got.Temperature, DefTemp)
	}
}

// TestAddParamsRoundTripsTemperatureZero pins the second half of findings2 §6a.
//
// sdk/kronk/model/params.go:372-374
//
//	if params.Temperature != 0 {
//		d["temperature"] = params.Temperature
//	}
//
// AddParams treats the zero value as "unset" for every field, but temperature 0
// is a meaningful request (greedy decoding). Converting a typed Params back into
// a D therefore drops it, so a Params carrying an explicit 0 cannot be
// round-tripped through the document form that parseParams consumes.
//
// The non-zero sub-test is a control.
func TestAddParamsRoundTripsTemperatureZero(t *testing.T) {
	tests := []struct {
		name string
		in   Params
		want float32
	}{
		{name: "explicit zero", in: Params{Temperature: 0}, want: 0},
		{name: "explicit non-zero", in: Params{Temperature: 0.25}, want: 0.25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := D{}
			AddParams(tt.in, d)

			val, exists := d["temperature"]
			if !exists {
				t.Fatalf("AddParams(Params{Temperature: %v}): key %q missing from D\n"+
					"params.go:372-374 only copies a non-zero temperature, so an explicit 0 is\n"+
					"dropped and the Params -> D conversion is lossy for greedy decoding.",
					tt.in.Temperature, "temperature")
			}

			got, ok := val.(float32)
			if !ok {
				t.Fatalf("AddParams: d[\"temperature\"] has type %T, want float32", val)
			}

			if got != tt.want {
				t.Errorf("AddParams: d[\"temperature\"] = %v, want %v", got, tt.want)
			}
		})
	}
}
