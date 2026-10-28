package decision

import (
	"context"
	"slices"

	"github.com/hybridgroup/yzma/pkg/mtmd"
)

// lfm3D1OmniProtocol shares the text protocol's labels, calibration and answer
// semantics. Only media placement and spoken instructions differ.
type lfm3D1OmniProtocol struct {
	lfm3D1Protocol
}

func newLFM3D1OmniProtocol(runtime *Runtime) (Protocol, error) {
	base, err := newLFM3D1Protocol(runtime)
	if err != nil {
		return nil, err
	}
	p := lfm3D1OmniProtocol{lfm3D1Protocol: *base}
	return &p, nil
}

func (p *lfm3D1OmniProtocol) SupportsImages() bool {
	return true
}

func (p *lfm3D1OmniProtocol) SupportsAudio() bool {
	return true
}

func (p *lfm3D1OmniProtocol) Decide(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	// Strip audio payloads before serializing template values. Their markers
	// are inserted only after user text has been escaped.
	text := req
	text.Questions = slices.Clone(req.Questions)
	for i, question := range text.Questions {
		if _, spoken := question.Instructions.(DecisionAudioInstructions); spoken {
			text.Questions[i].Instructions = ""
		}
	}
	questions, err := p.questions(text)
	if err != nil {
		return DecisionResponse{}, err
	}
	stateFiles := slices.Concat(req.Images, req.Files)
	marker := mtmd.DefaultMarker()
	for i := range questions {
		files := stateFiles
		audio, spoken := req.Questions[i].Instructions.(DecisionAudioInstructions)
		if spoken {
			files = append(slices.Clone(stateFiles), audio.Data)
		}
		if len(files) == 0 {
			continue
		}
		values := replaceDecisionTemplateText(questions[i].values, marker, " ").(map[string]any)
		markers := make([]string, len(stateFiles))
		for j := range markers {
			markers[j] = marker
		}
		values["images"] = markers
		if spoken {
			values["instructions"] = marker
		}
		questions[i].values = values
		questions[i].files = files
	}
	return p.run(ctx, req, questions, answerLFM3D1Question)
}
