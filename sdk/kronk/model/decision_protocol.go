package model

import (
	"context"
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
)

type decisionProtocol interface {
	decide(ctx context.Context, req DecisionRequest) (DecisionResponse, error)
}

type decisionProtocolFactory func(m *Model) (decisionProtocol, error)

var decisionProtocolFactories = map[DecisionProtocol]decisionProtocolFactory{
	DecisionProtocolOpenJEV:  newDetectedOpenJEVProtocol,
	DecisionProtocolJevStyle: func(m *Model) (decisionProtocol, error) { return newJevStyleProtocol(m) },
	DecisionProtocolLaya:     func(m *Model) (decisionProtocol, error) { return newLayaProtocol(m) },
	DecisionProtocolLev:      func(m *Model) (decisionProtocol, error) { return newLevProtocol(m) },
	DecisionProtocolKev:      func(m *Model) (decisionProtocol, error) { return newKevProtocol(m) },
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
	if err := validateDecisionRequest(req); err != nil {
		return DecisionResponse{}, err
	}
	return m.protocol.decide(ctx, req)
}
