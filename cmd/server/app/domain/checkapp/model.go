package checkapp

import "encoding/json"

// Info represents information about the service.
type Info struct {
	Status     string `json:"status,omitempty"`
	Build      string `json:"build,omitempty"`
	Revision   string `json:"vcs_revision,omitempty"`
	CommitTime string `json:"vcs_time,omitempty"`
	Modified   *bool  `json:"vcs_modified,omitempty"`
	Host       string `json:"host,omitempty"`
	Name       string `json:"name,omitempty"`
	PodIP      string `json:"podIP,omitempty"`
	Node       string `json:"node,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	GOMAXPROCS int    `json:"GOMAXPROCS,omitempty"`
	OS         string `json:"os,omitempty"`
}

// Encode implements the encoder interface.
func (app Info) Encode() ([]byte, string, error) {
	data, err := json.Marshal(app)
	return data, "application/json", err
}
