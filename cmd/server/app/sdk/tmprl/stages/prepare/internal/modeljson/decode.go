// Package modeljson decodes strict structured model responses.
package modeljson

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

func Decode(text string, output any) error {
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return fmt.Errorf("invalid model JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return fmt.Errorf("model JSON contains trailing data")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}
