package catalog

import (
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// RetrieveModelInfo reads a GGUF model file for the specieid model id and
// extracts its metadata.
func (c *Catalog) RetrieveModelInfo(modelID string) (models.ModelInfo, error) {

	// The modelID might have a / because it's a model in the config
	// with different settings. Ex. model/FMC.
	// We need to remove /FMC for the RetrievePath call.
	fsModelID, _, _ := strings.Cut(modelID, "/")

	md, err := c.models.RetrieveModelInfo(fsModelID)
	if err != nil {
		return models.ModelInfo{}, fmt.Errorf("retrieve-model-config: unable to parse model[%s] metadata: %w", modelID, err)
	}

	return md, nil
}
