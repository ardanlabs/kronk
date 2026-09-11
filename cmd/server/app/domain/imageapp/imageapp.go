package imageapp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/malina/model"
	"github.com/ardanlabs/kronk/sdk/pool"
)

type app struct {
	log  *logger.Logger
	pool *pool.Pool
}

func newApp(cfg Config) *app {
	return &app{log: cfg.Log, pool: cfg.Pool}
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

func (a *app) generations(ctx context.Context, r *http.Request) web.Encoder {
	var req generationRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return errs.New(errs.InvalidArgument, fmt.Errorf("decode request: %w", err))
	}

	params, err := req.params()
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	handle, err := a.pool.Malina.AcquireModel(ctx, req.Model)
	if err != nil {
		return errs.FromSDK(err)
	}

	a.log.Info(ctx, "image-generation", "model", req.Model, "width", params.Width, "height", params.Height, "steps", params.Steps)

	image, err := handle.Generate(ctx, params)
	if err != nil {
		return errs.FromSDK(fmt.Errorf("generate image: %w", err))
	}

	return generationResponse{
		Created: time.Now().Unix(),
		Data: []generationImageData{{
			B64JSON: base64.StdEncoding.EncodeToString(image.PNG),
			Seed:    image.Seed,
			Width:   image.Width,
			Height:  image.Height,
		}},
	}
}

func (r generationRequest) params() (model.GenerateParams, error) {
	if strings.TrimSpace(r.Model) == "" {
		return model.GenerateParams{}, errors.New("model is required")
	}
	if r.N == 0 {
		r.N = 1
	}
	if r.N != 1 {
		return model.GenerateParams{}, errors.New("n must be 1")
	}
	if r.ResponseFormat == "" {
		r.ResponseFormat = "b64_json"
	}
	if r.ResponseFormat != "b64_json" {
		return model.GenerateParams{}, errors.New("response_format must be b64_json")
	}

	params := model.NewGenerateParams()
	params.Prompt = r.Prompt
	params.NegativePrompt = r.NegativePrompt
	if r.Size != "" {
		width, height, err := parseSize(r.Size)
		if err != nil {
			return model.GenerateParams{}, err
		}
		params.Width = width
		params.Height = height
	}
	if r.Steps != nil {
		params.Steps = *r.Steps
	}
	if r.CFGScale != nil {
		params.CFGScale = *r.CFGScale
	}
	if r.Seed != nil {
		params.Seed = *r.Seed
	}
	if err := params.Validate(); err != nil {
		return model.GenerateParams{}, err
	}

	return params, nil
}

func parseSize(value string) (int, int, error) {
	widthText, heightText, ok := strings.Cut(value, "x")
	if !ok || widthText == "" || heightText == "" || strings.Contains(heightText, "x") {
		return 0, 0, fmt.Errorf("size must use WIDTHxHEIGHT format")
	}
	width, err := strconv.Atoi(widthText)
	if err != nil {
		return 0, 0, fmt.Errorf("size width must be an integer: %w", err)
	}
	height, err := strconv.Atoi(heightText)
	if err != nil {
		return 0, 0, fmt.Errorf("size height must be an integer: %w", err)
	}

	return width, height, nil
}
