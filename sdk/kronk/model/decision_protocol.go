package model

import (
	"context"
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

type decisionProtocol interface {
	decide(ctx context.Context, req DecisionRequest) (DecisionResponse, error)
}

// decisionMediaProtocol declares prompt support, independently of the loaded
// projector's capabilities. Protocols without this contract remain text-only.
type decisionMediaProtocol interface {
	mediaSupport() MediaType
}

type decisionProtocolFactory func(m *Model) (decisionProtocol, error)

var decisionProtocolFactories = map[DecisionProtocol]decisionProtocolFactory{
	DecisionProtocolOpenJEV:       newDetectedOpenJEVProtocol,
	DecisionProtocolJevStyle:      func(m *Model) (decisionProtocol, error) { return newJevStyleProtocol(m) },
	DecisionProtocolLaya:          func(m *Model) (decisionProtocol, error) { return newLayaProtocol(m) },
	DecisionProtocolLev:           func(m *Model) (decisionProtocol, error) { return newLevProtocol(m) },
	DecisionProtocolKev:           func(m *Model) (decisionProtocol, error) { return newKevProtocol(m) },
	DecisionProtocolNimble:        func(m *Model) (decisionProtocol, error) { return newNimbleProtocol(m) },
	DecisionProtocolClef:          func(m *Model) (decisionProtocol, error) { return newClefProtocol(m) },
	DecisionProtocolPplxDecider:   newDeciderProtocol,
	DecisionProtocolLfm2D1:        newDeciderProtocol,
	DecisionProtocolVllmD3:        newDeciderProtocol,
	DecisionProtocolVllmDecision2: newDecision2Protocol,
}

func initDecisionProtocol(m *Model) error {
	factory, exists := decisionProtocolFactories[m.modelInfo.decisionProtocol]
	if !exists {
		return fmt.Errorf("init-decision-protocol: unknown decision protocol %q", m.modelInfo.decisionProtocol)
	}

	protocol, err := factory(m)
	if err != nil {
		return err
	}
	m.protocol = protocol
	return nil
}

func newDetectedOpenJEVProtocol(m *Model) (decisionProtocol, error) {
	if llama.ModelChatTemplate(m.model, "systemone") != "" {
		return newSystemOneOpenJEVProtocol(m)
	}
	return newOpenJEVProtocol(m)
}

// Decision evaluates a set of decision questions against shared state.
func (m *Model) Decision(ctx context.Context, req DecisionRequest) (DecisionResponse, error) {
	if m.protocol == nil {
		return DecisionResponse{}, fmt.Errorf("decision: protocol was not detected for model %q; configure model.WithDecisionProtocol(...) with the model's protocol", m.modelInfo.ID)
	}
	if err := validateDecisionRequest(req, m.modelInfo.decisionProtocol); err != nil {
		return DecisionResponse{}, err
	}
	if len(req.Images) > 0 {
		media, ok := m.protocol.(decisionMediaProtocol)
		if !ok || media.mediaSupport() != MediaTypeVision {
			return DecisionResponse{}, fmt.Errorf("%w: decision protocol %q does not support images", ErrDecisionRequest, m.modelInfo.decisionProtocol)
		}
		if m.mtmdMetaCtx == 0 || !mtmd.SupportVision(m.mtmdMetaCtx) {
			return DecisionResponse{}, fmt.Errorf("%w: decision images require a compatible vision projector", ErrDecisionRequest)
		}
	}
	return m.protocol.decide(ctx, req)
}
