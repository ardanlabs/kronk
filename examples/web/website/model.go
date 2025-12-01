package website

import (
	"fmt"

	"github.com/ardanlabs/kronk/model"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Messages    []Message `json:"messages"`
	TopK        *int32    `json:"top_k"`
	TopP        *float32  `json:"top_p"`
	Temperature *float32  `json:"temperature"`
	MaxTokens   *int      `json:"max_tokens"`
}

func getParams(traceID string, req Request) model.Params {
	var topK int32
	if req.TopK != nil {
		fmt.Printf("traceID: %s: getParams: topK: %#v\n", traceID, *req.TopK)
		topK = *req.TopK
	}

	var topP float32
	if req.TopP != nil {
		fmt.Printf("traceID: %s: getParams: topP: %#v\n", traceID, *req.TopP)
		topP = *req.TopP
	}

	var temp float32
	if req.Temperature != nil {
		fmt.Printf("traceID: %s: getParams: temp: %#v\n", traceID, *req.Temperature)
		temp = *req.Temperature
	}

	var maxTokens int
	if req.MaxTokens != nil {
		fmt.Printf("traceID: %s: getParams: maxTokens: %#v\n", traceID, *req.MaxTokens)
		maxTokens = *req.MaxTokens
	}

	params := model.Params{
		TopK:        topK,
		TopP:        topP,
		Temperature: temp,
		MaxTokens:   maxTokens,
	}

	return params
}
