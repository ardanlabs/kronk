package mngtapp

import (
	"encoding/json"

	"github.com/ardanlabs/kronk/install"
	"github.com/hybridgroup/yzma/pkg/download"
)

// Version returns information about the installed libraries.
type Version struct {
	Status    string `json:"status"`
	LibsPath  string `json:"libs_paths"`
	Processor string `json:"processor"`
	Latest    string `json:"latest"`
	Current   string `json:"current"`
}

// Encode implements the encoder interface.
func (app Version) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}

func toAppVersion(status string, libsPath string, processor download.Processor, krn install.Version) Version {
	return Version{
		Status:    status,
		LibsPath:  libsPath,
		Processor: processor.String(),
		Latest:    krn.Latest,
		Current:   krn.Current,
	}
}
