package models

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidModelID is returned when a model identifier is not in canonical
// provider/modelID or provider/modelID/profile form.
var ErrInvalidModelID = errors.New("invalid model id")

// ModelID identifies an installed model and an optional named configuration.
type ModelID struct {
	Provider string
	Model    string
	Profile  string
}

// ParseModelID parses a canonical model identifier.
func ParseModelID(value string) (ModelID, error) {
	value = strings.TrimSpace(value)
	parts := strings.Split(value, "/")
	if len(parts) < 2 || len(parts) > 3 {
		return ModelID{}, fmt.Errorf("%w: %q must use provider/modelID or provider/modelID/profile", ErrInvalidModelID, value)
	}

	for _, part := range parts {
		if part == "" || strings.TrimSpace(part) != part {
			return ModelID{}, fmt.Errorf("%w: %q contains an empty or whitespace-padded segment", ErrInvalidModelID, value)
		}
	}

	id := ModelID{
		Provider: parts[0],
		Model:    parts[1],
	}
	if len(parts) == 3 {
		id.Profile = parts[2]
	}

	return id, nil
}

// Base returns the provider/modelID portion of the identifier.
func (id ModelID) Base() string {
	return id.Provider + "/" + id.Model
}

// String returns the complete identifier, including its profile when set.
func (id ModelID) String() string {
	if id.Profile == "" {
		return id.Base()
	}

	return id.Base() + "/" + id.Profile
}
