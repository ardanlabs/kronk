package imageapp

import "encoding/json"

type progressEvent struct {
	Scope          string  `json:"scope"`
	Status         string  `json:"status"`
	Step           int     `json:"step"`
	Steps          int     `json:"steps"`
	Percent        float64 `json:"percent"`
	SecondsPerStep float32 `json:"seconds_per_step"`
}

type generationRequest struct {
	Model          string   `json:"model"`
	Prompt         string   `json:"prompt"`
	Size           string   `json:"size"`
	N              int      `json:"n"`
	ResponseFormat string   `json:"response_format"`
	User           string   `json:"user"`
	NegativePrompt string   `json:"negative_prompt"`
	Steps          *int     `json:"steps"`
	CFGScale       *float32 `json:"cfg_scale"`
	Seed           *int64   `json:"seed"`
}

type generationResponse struct {
	Created int64                 `json:"created"`
	Data    []generationImageData `json:"data"`
}

type generationImageData struct {
	B64JSON string `json:"b64_json"`
	Seed    int64  `json:"seed"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
}

func (gr generationResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(gr)
	return data, "application/json", err
}
