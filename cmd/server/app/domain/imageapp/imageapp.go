package imageapp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/malinaprogress"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/malina/model"
	"github.com/ardanlabs/kronk/sdk/pool"
	malinamodels "github.com/ardanlabs/kronk/sdk/tools/malina/models"
	"golang.org/x/image/draw"
)

const (
	maxImageUploadBytes   = 25 << 20
	maxMultipartOverhead  = 1 << 20
	maxDecodedImagePixels = 32 << 20
)

type app struct {
	log      *logger.Logger
	pool     *pool.Pool
	progress *malinaprogress.Broker
}

func newApp(cfg Config) *app {
	return &app{
		log:      cfg.Log,
		pool:     cfg.Pool,
		progress: cfg.MalinaProgress,
	}
}

func (a *app) events(ctx context.Context, _ *http.Request) web.Encoder {
	if a.progress == nil {
		return errs.Errorf(errs.Internal, "malina progress is not configured")
	}

	updates, unsubscribe := a.progress.Subscribe()
	defer unsubscribe()

	w := web.GetWriter(ctx)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	if err := writeProgressEvent(w, progressEvent{Scope: "global", Status: "connected"}); err != nil {
		return web.NewNoResponseError(errs.New(errs.Internal, err))
	}

	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-ctx.Done():
			return web.NewNoResponse()

		case update := <-updates:
			percent := 0.0
			if update.Steps > 0 {
				percent = min(100, max(0, float64(update.Step)/float64(update.Steps)*100))
			}
			event := progressEvent{
				Scope:          "global",
				Status:         "progress",
				Step:           update.Step,
				Steps:          update.Steps,
				Percent:        percent,
				SecondsPerStep: update.SecondsPerStep,
			}
			if err := writeProgressEvent(w, event); err != nil {
				return web.NewNoResponseError(errs.New(errs.Internal, err))
			}

		case <-keepAlive.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return web.NewNoResponseError(errs.New(errs.Internal, fmt.Errorf("write Malina progress keep-alive: %w", err)))
			}
			if err := http.NewResponseController(w).Flush(); err != nil {
				return web.NewNoResponseError(errs.New(errs.Internal, fmt.Errorf("flush Malina progress keep-alive: %w", err)))
			}
		}
	}
}

func writeProgressEvent(w http.ResponseWriter, event progressEvent) error {
	return writeImageEvent(w, "", event)
}

func writeImageEvent(w http.ResponseWriter, name string, event any) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal malina progress event: %w", err)
	}
	if name != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", name); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return fmt.Errorf("write malina progress event: %w", err)
	}
	if err := http.NewResponseController(w).Flush(); err != nil {
		return fmt.Errorf("flush malina progress event: %w", err)
	}

	return nil
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
	if req.Model == malinamodels.BundleMOSSImageEditHQ.String() {
		return errs.Errorf(errs.InvalidArgument, "MOSS editing requires an image; use /v1/images/edits")
	}

	return a.generate(ctx, req.Model, params, "text-to-image", req.Stream)
}

func (a *app) edits(ctx context.Context, r *http.Request) web.Encoder {
	r.Body = http.MaxBytesReader(nil, r.Body, maxImageUploadBytes+maxMultipartOverhead)
	if err := r.ParseMultipartForm(maxImageUploadBytes); err != nil {
		return errs.New(errs.InvalidArgument, fmt.Errorf("parse multipart form: %w", err))
	}
	defer r.MultipartForm.RemoveAll()

	req, strength, err := parseEditRequest(r)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		return errs.New(errs.InvalidArgument, fmt.Errorf("image form field: %w", err))
	}
	defer file.Close()
	if header.Size > maxImageUploadBytes {
		return errs.Errorf(errs.InvalidArgument, "image exceeds 25 MiB limit")
	}

	source, err := decodeImage(file)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}
	params, err := editParams(req, source, strength)
	if err != nil {
		return errs.New(errs.InvalidArgument, err)
	}

	return a.generate(ctx, req.Model, params, "image-to-image", req.Stream)
}

func (a *app) generate(ctx context.Context, modelID string, params model.GenerateParams, operation string, stream bool) web.Encoder {
	handle, err := a.pool.Malina.AcquireModel(ctx, modelID)
	if err != nil {
		return errs.FromSDK(err)
	}

	a.log.Info(ctx, "image-generation", "operation", operation, "model", modelID, "width", params.Width, "height", params.Height, "steps", params.Steps)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	w := web.GetWriter(ctx)
	var progressMu sync.Mutex
	var progressErr error
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		params.Progress = func(event model.GenerationProgress) {
			progressMu.Lock()
			defer progressMu.Unlock()
			if progressErr != nil {
				return
			}
			percent := 0.0
			if event.Total > 0 {
				percent = min(100, max(0, float64(event.Completed)/float64(event.Total)*100))
			}
			progressErr = writeImageEvent(w, "progress", progressEvent{
				Scope: "request", Status: "progress", Stage: event.Stage, Detail: event.Detail,
				Unit: event.Unit, SamplePass: event.SamplePass,
				Step: int(event.Completed), Steps: int(event.Total), Percent: percent,
			})
			if progressErr != nil {
				cancel()
			}
		}
	}
	image, err := handle.Generate(ctx, params)
	progressMu.Lock()
	defer progressMu.Unlock()
	if progressErr != nil {
		return web.NewNoResponseError(errs.New(errs.Internal, progressErr))
	}
	if err != nil {
		if stream {
			if writeErr := writeImageEvent(w, "error", errs.FromSDK(err)); writeErr != nil {
				return web.NewNoResponseError(errs.New(errs.Internal, writeErr))
			}
			return web.NewNoResponse()
		}
		return errs.FromSDK(fmt.Errorf("generate image: %w", err))
	}

	response := generationResponse{
		Created: time.Now().Unix(),
		Data: []generationImageData{{
			B64JSON: base64.StdEncoding.EncodeToString(image.PNG),
			Seed:    image.Seed,
			Width:   image.Width,
			Height:  image.Height,
		}},
	}
	if stream {
		if err := writeImageEvent(w, "result", response); err != nil {
			return web.NewNoResponseError(errs.New(errs.Internal, err))
		}
		return web.NewNoResponse()
	}
	return response
}

func parseEditRequest(r *http.Request) (generationRequest, float32, error) {
	form := r.MultipartForm.Value
	req := generationRequest{
		Model:          firstFormValue(form, "model"),
		Prompt:         firstFormValue(form, "prompt"),
		Size:           firstFormValue(form, "size"),
		ResponseFormat: firstFormValue(form, "response_format"),
		User:           firstFormValue(form, "user"),
		NegativePrompt: firstFormValue(form, "negative_prompt"),
	}
	if value, exists := form["stream"]; exists {
		if len(value) != 1 || (value[0] != "true" && value[0] != "false") {
			return generationRequest{}, 0, errors.New("field[stream] must be true or false")
		}
		req.Stream = value[0] == "true"
	}

	n, exists, err := optionalFormInt(form, "n", 32)
	if err != nil {
		return generationRequest{}, 0, err
	}
	if exists {
		req.N = int(n)
	}
	steps, exists, err := optionalFormInt(form, "steps", 32)
	if err != nil {
		return generationRequest{}, 0, err
	}
	if exists {
		value := int(steps)
		req.Steps = &value
	}
	seed, exists, err := optionalFormInt(form, "seed", 64)
	if err != nil {
		return generationRequest{}, 0, err
	}
	if exists {
		req.Seed = &seed
	}
	cfgScale, exists, err := optionalFormFloat(form, "cfg_scale")
	if err != nil {
		return generationRequest{}, 0, err
	}
	if exists {
		req.CFGScale = &cfgScale
	}
	strength := float32(0.75)
	if value, exists, err := optionalFormFloat(form, "strength"); err != nil {
		return generationRequest{}, 0, err
	} else if exists {
		strength = value
	}

	return req, strength, nil
}

func firstFormValue(form map[string][]string, field string) string {
	if values := form[field]; len(values) > 0 {
		return values[0]
	}
	return ""
}

func optionalFormInt(form map[string][]string, field string, bitSize int) (int64, bool, error) {
	values, exists := form[field]
	if !exists {
		return 0, false, nil
	}
	if len(values) == 0 || values[0] == "" {
		return 0, false, fmt.Errorf("field[%s] must be an integer", field)
	}
	value, err := strconv.ParseInt(values[0], 10, bitSize)
	if err != nil {
		return 0, false, fmt.Errorf("field[%s] must be an integer: %w", field, err)
	}

	return value, true, nil
}

func optionalFormFloat(form map[string][]string, field string) (float32, bool, error) {
	values, exists := form[field]
	if !exists {
		return 0, false, nil
	}
	if len(values) == 0 || values[0] == "" {
		return 0, false, fmt.Errorf("field[%s] must be a number", field)
	}
	value, err := strconv.ParseFloat(values[0], 32)
	if err != nil {
		return 0, false, fmt.Errorf("field[%s] must be a number: %w", field, err)
	}

	return float32(value), true, nil
}

func decodeImage(file io.ReadSeeker) (image.Image, error) {
	config, format, err := image.DecodeConfig(file)
	if err != nil {
		return nil, fmt.Errorf("decode image configuration: %w", err)
	}
	if format != "jpeg" && format != "png" {
		return nil, fmt.Errorf("image must be PNG or JPEG")
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxDecodedImagePixels/config.Height {
		return nil, fmt.Errorf("decoded image exceeds %d pixel limit", maxDecodedImagePixels)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("rewind image: %w", err)
	}
	source, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	return source, nil
}

func editParams(req generationRequest, source image.Image, strength float32) (model.GenerateParams, error) {
	params, err := req.params()
	if err != nil {
		return model.GenerateParams{}, err
	}
	if req.Size == "" {
		params.Width, params.Height, err = generationSize(source.Bounds())
		if err != nil {
			return model.GenerateParams{}, err
		}
	}
	multiple := 8
	switch req.Model {
	case malinamodels.BundleMOSSImageEditHQ.String():
		multiple = 16
	case malinamodels.BundleQwenImage21.String():
		multiple = 32
	}
	if multiple != 8 {
		if req.Size == "" {
			params.Width = params.Width / multiple * multiple
			params.Height = params.Height / multiple * multiple
		}
		if params.Width%multiple != 0 || params.Height%multiple != 0 {
			return model.GenerateParams{}, fmt.Errorf("%s output dimensions must be multiples of %d", req.Model, multiple)
		}
	}
	params.Strength = strength
	if req.Model == malinamodels.BundleQwenImage21.String() || req.Model == malinamodels.BundleFlux2KleinBase4B.String() {
		if source.Bounds().Dx() > 1024 || source.Bounds().Dy() > 1024 {
			width, height, err := generationSize(source.Bounds())
			if err != nil {
				return model.GenerateParams{}, err
			}
			source = resizeImage(source, width, height)
		}
		params.RefImages = []image.Image{source}
	} else {
		params.InitImage = resizeImage(source, params.Width, params.Height)
	}
	if err := params.Validate(); err != nil {
		return model.GenerateParams{}, err
	}

	return params, nil
}

func generationSize(bounds image.Rectangle) (int, int, error) {
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return 0, 0, errors.New("source image dimensions must be positive")
	}
	scale := min(1, min(1024/float64(width), 1024/float64(height)))
	width = int(float64(width)*scale) / 8 * 8
	height = int(float64(height)*scale) / 8 * 8
	if width < 64 || height < 64 {
		return 0, 0, errors.New("source image aspect ratio produces a dimension below 64 pixels")
	}

	return width, height, nil
}

func resizeImage(source image.Image, width int, height int) image.Image {
	if source.Bounds().Dx() == width && source.Bounds().Dy() == height {
		return source
	}
	destination := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(destination, destination.Bounds(), source, source.Bounds(), draw.Over, nil)
	return destination
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
	switch r.Model {
	case malinamodels.BundleFlux2KleinBase4B.String():
		params.CFGScale = 4
	case malinamodels.BundleQwenImage21.String():
		params.CFGScale = 6
	}
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
	if r.Model == malinamodels.BundleQwenImage21.String() && (params.Width%32 != 0 || params.Height%32 != 0) {
		return model.GenerateParams{}, errors.New("qwen image 2.1 output dimensions must be multiples of 32")
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
