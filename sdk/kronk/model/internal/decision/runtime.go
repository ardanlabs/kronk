// Package decision implements decision protocols independently of native
// execution, scheduling, and model resource ownership.
package decision

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
	"github.com/hybridgroup/yzma/pkg/llama"
)

// MaxReadouts bounds the number of decision outputs in one inference pass.
const MaxReadouts = 255

// D contains template values.
type D = map[string]any

// Protocol prepares and interprets decision requests.
type Protocol interface {
	Decide(context.Context, DecisionRequest) (DecisionResponse, error)
}

// ImageProtocol declares support for image inputs independently of projector
// capabilities. Protocols without this contract are text-only.
type ImageProtocol interface {
	SupportsImages() bool
}

// AudioProtocol declares support for audio state and spoken instructions.
type AudioProtocol interface {
	SupportsAudio() bool
}

// Runtime supplies borrowed model metadata and model-owned operations. Protocols
// never create or free native contexts, schedule work, or retain MTMD resources.
type Runtime struct {
	Model         llama.Model
	Vocab         llama.Vocab
	ModelID       string
	Protocol      string
	AddBOSToken   bool
	ContextWindow int
	Calibration   modelprofile.Decision
	Run           func(context.Context, []Work) ([][][]float32, error)
	// RenderChat renders a single-user decision prompt with thinking disabled.
	RenderChat   func(context.Context, string) (string, error)
	PrepareMedia func(context.Context, string, [][]byte) ([]PrefixRow, int, error)
}

// PrefixRow contains one owned media-prefix row, without native resource handles.
type PrefixRow struct {
	Token         llama.Token
	HasToken      bool
	Embedding     []float32
	Positions     [4]llama.Pos
	PositionCount int
}

// Readout selects logits or embeddings at one token position.
type Readout struct {
	Position       int
	Candidates     []llama.Token
	EmbeddingWidth int
}

// Work describes an inference pass. All slices remain owned by the request
// until Run returns; the execution engine owns native staging and isolation.
type Work struct {
	Tokens        []llama.Token
	PrefixRows    []PrefixRow
	Position      int
	PrefixLen     int
	Readouts      []Readout
	DecisionOrder []llama.DecisionOrder
	JointScores   int
}

func (w Work) rowCount() int {
	return len(w.PrefixRows) + len(w.Tokens)
}

type protocolFactory func(*Runtime) (Protocol, error)

var protocolFactories = map[string]protocolFactory{
	"openjev":        newDetectedOpenJEVProtocol,
	"jev-style":      func(r *Runtime) (Protocol, error) { return newJevStyleProtocol(r) },
	"laya":           func(r *Runtime) (Protocol, error) { return newLayaProtocol(r) },
	"lev":            func(r *Runtime) (Protocol, error) { return newLevProtocol(r) },
	"kev":            func(r *Runtime) (Protocol, error) { return newKevProtocol(r) },
	"nimble":         func(r *Runtime) (Protocol, error) { return newNimbleProtocol(r) },
	"clef":           func(r *Runtime) (Protocol, error) { return newClefProtocol(r) },
	"pplx-decider":   newPplxDeciderProtocol,
	"lfm2-d1":        newLFM2D1Protocol,
	"lfm3-d1":        func(r *Runtime) (Protocol, error) { return newLFM3D1Protocol(r) },
	"lfm3-d1-omni":   newLFM3D1OmniProtocol,
	"vllm-d3":        newD3Protocol,
	"vllm-decision2": newDecision2Protocol,
}

// Supports reports whether a protocol has a registered implementation.
func Supports(protocol string) bool {
	_, exists := protocolFactories[protocol]
	return exists
}

// New constructs a protocol with model-owned runtime operations.
func New(runtime Runtime) (Protocol, error) {
	factory, exists := protocolFactories[runtime.Protocol]
	if !exists {
		return nil, fmt.Errorf("init-decision-protocol: unknown decision protocol %q", runtime.Protocol)
	}
	return factory(&runtime)
}

func newDetectedOpenJEVProtocol(r *Runtime) (Protocol, error) {
	if llama.ModelChatTemplate(r.Model, "systemone") != "" {
		return newSystemOneOpenJEVProtocol(r)
	}
	return newOpenJEVProtocol(r)
}

func tokenText(vocab llama.Vocab, token llama.Token) string {
	buf := make([]byte, 128)
	n := llama.TokenToPiece(vocab, token, buf, 0, true)
	if n < 0 {
		return ""
	}
	return string(buf[:n])
}
