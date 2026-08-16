package speculation

import "github.com/hybridgroup/yzma/pkg/llama"

type disabledController struct {
	host Host
}

// NewDisabled constructs a target-only controller.
func NewDisabled(host Host) Controller {
	return &disabledController{host: host}
}

func (*disabledController) Mode() Mode    { return ModeDisabled }
func (*disabledController) Enabled() bool { return false }
func (*disabledController) BeginBatch()   {}
func (*disabledController) Prepare()      {}

func (*disabledController) PlanGeneration(int) (Generation, error) {
	return Generation{}, nil
}

func (*disabledController) CommitGeneration(int, []llama.Token, TargetRange) error {
	return nil
}

func (*disabledController) TargetRowsStaged(int, TargetRange) {}

func (c *disabledController) AfterTargetDecode(buf []byte) {
	for slot := range c.host.SlotCount() {
		if c.host.SlotActive(slot) {
			c.host.ProcessOrdinary(slot, buf)
		}
	}
}
