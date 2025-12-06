package mngtapp

import (
	"encoding/json"

	"github.com/ardanlabs/kronk/tools"
	"github.com/hybridgroup/yzma/pkg/download"
)

// Version returns information about the installed libraries.
type Version struct {
	Status    string `json:"status"`
	LibPath   string `json:"libs_paths"`
	Processor string `json:"processor"`
	Latest    string `json:"latest"`
	Current   string `json:"current"`
}

// Encode implements the encoder interface.
func (app Version) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toAppVersion(status string, libPath string, processor download.Processor, krn tools.LibVersion) Version {
	return Version{
		Status:    status,
		LibPath:   libPath,
		Processor: processor.String(),
		Latest:    krn.Latest,
		Current:   krn.Current,
	}
}
