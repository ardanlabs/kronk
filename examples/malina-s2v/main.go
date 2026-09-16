// This example generates a Wan2.2 S2V video from a source image and a WAV
// speech track. It writes Motion-JPEG video and PCM WAV audio as separate
// files because model.SaveAVI does not mux audio.
//
// Experimental: The Malina SDK public API is subject to change.
//
// Download the Wan2.2 S2V diffusion model, Wan 2.1 VAE, UMT5-XXL encoder, and
// wav2vec2 audio encoder described by stable-diffusion.cpp's docs/wan.md, then
// run this example with their paths:
//
//	$ make example-malina-s2v ARGS='-diffusion /path/to/wan2.2-s2v.safetensors \
//	    -vae /path/to/wan_2.1_vae.safetensors \
//	    -t5xxl /path/to/umt5-xxl.safetensors \
//	    -audio-encoder /path/to/wav2vec2_large_english_fp16.safetensors \
//	    -image /path/to/portrait.png -audio /path/to/speech.wav'
package main

import (
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	buckyaudio "github.com/ardanlabs/bucky/pkg/audio"
	"github.com/ardanlabs/kronk/sdk/malina"
	"github.com/ardanlabs/kronk/sdk/malina/model"
	"github.com/ardanlabs/kronk/sdk/tools/malina/libs"
	"golang.org/x/image/draw"
)

type config struct {
	diffusion    string
	vae          string
	t5xxl        string
	audioEncoder string
	image        string
	audio        string
	output       string
	prompt       string
	width        int
	height       int
	steps        int
	frames       int
	fps          int
	seed         int64
}

func main() {
	var cfg config
	flag.StringVar(&cfg.diffusion, "diffusion", "", "Wan2.2 S2V diffusion model path")
	flag.StringVar(&cfg.vae, "vae", "", "Wan 2.1 VAE model path")
	flag.StringVar(&cfg.t5xxl, "t5xxl", "", "UMT5-XXL text encoder path")
	flag.StringVar(&cfg.audioEncoder, "audio-encoder", "", "wav2vec2 audio encoder path")
	flag.StringVar(&cfg.image, "image", "", "source portrait PNG or JPEG path")
	flag.StringVar(&cfg.audio, "audio", "", "driving WAV audio path")
	flag.StringVar(&cfg.output, "out", "malina-s2v.avi", "output Motion-JPEG AVI path")
	flag.StringVar(&cfg.prompt, "prompt", "a person speaking naturally to the camera", "video prompt")
	flag.IntVar(&cfg.width, "width", 832, "video width (multiple of 8)")
	flag.IntVar(&cfg.height, "height", 480, "video height (multiple of 8)")
	flag.IntVar(&cfg.steps, "steps", 20, "sampling steps")
	flag.IntVar(&cfg.frames, "frames", 81, "number of video frames")
	flag.IntVar(&cfg.fps, "fps", 16, "requested frames per second")
	flag.Int64Var(&cfg.seed, "seed", -1, "RNG seed (-1 selects a random seed)")
	flag.Parse()

	if err := run(cfg); err != nil {
		fmt.Println("ERROR:", err)
		os.Exit(1)
	}
}

func run(cfg config) error {
	if err := cfg.validate(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	if err := initMalina(ctx); err != nil {
		return err
	}

	source, err := loadImage(cfg.image)
	if err != nil {
		return err
	}
	source = resize(source, cfg.width, cfg.height)

	drivingAudio, err := loadAudio(cfg.audio)
	if err != nil {
		return err
	}

	mln, err := malina.NewWithContext(
		ctx,
		model.WithDiffusionModelPath(cfg.diffusion),
		model.WithVAEPath(cfg.vae),
		model.WithT5XXLPath(cfg.t5xxl),
		model.WithAudioEncoderPath(cfg.audioEncoder),
	)
	if err != nil {
		return fmt.Errorf("load Wan2.2 S2V: %w", err)
	}

	defer func() {
		if err := mln.Unload(context.Background()); err != nil {
			fmt.Println("unload:", err)
		}
	}()

	params := model.NewVideoParams()
	params.Prompt = cfg.prompt
	params.Width = cfg.width
	params.Height = cfg.height
	params.Steps = cfg.steps
	params.Frames = cfg.frames
	params.FPS = cfg.fps
	params.Seed = cfg.seed
	params.InitImage = source
	params.RefAudios = []model.Audio{drivingAudio}

	video, err := mln.GenerateVideo(ctx, params)
	if err != nil {
		return fmt.Errorf("generate video: %w", err)
	}
	if err := model.SaveAVI(cfg.output, video.Frames, video.FPS, 90); err != nil {
		return err
	}

	outputAudio := drivingAudio
	if video.Audio != nil {
		outputAudio = *video.Audio
	}
	wavPath := strings.TrimSuffix(cfg.output, filepath.Ext(cfg.output)) + ".wav"
	if err := saveWAV(wavPath, outputAudio); err != nil {
		return err
	}

	fmt.Printf("Wrote %s and %s (%d frames at %d fps)\n", cfg.output, wavPath, len(video.Frames), video.FPS)
	fmt.Printf("Mux with: ffmpeg -i %q -i %q -c:v copy -c:a aac -shortest malina-s2v-with-audio.mp4\n", cfg.output, wavPath)

	return nil
}

func (cfg config) validate() error {
	required := []struct {
		name  string
		value string
	}{
		{"diffusion", cfg.diffusion},
		{"vae", cfg.vae},
		{"t5xxl", cfg.t5xxl},
		{"audio-encoder", cfg.audioEncoder},
		{"image", cfg.image},
		{"audio", cfg.audio},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("-%s is required", field.name)
		}
	}

	return nil
}

func initMalina(ctx context.Context) error {
	lib, err := libs.New(
		libs.WithDetect(ctx, malina.FmtLogger),
		libs.WithValidation(true),
	)
	if err != nil {
		return err
	}
	if _, err := lib.Download(ctx, malina.FmtLogger); err != nil {
		return err
	}

	return malina.Init(malina.WithLibPath(lib.LibsPath()))
}

func loadImage(filename string) (image.Image, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("open image: %w", err)
	}
	defer file.Close()

	source, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	return source, nil
}

func resize(source image.Image, width int, height int) image.Image {
	target := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(target, target.Bounds(), source, source.Bounds(), draw.Over, nil)
	return target
}

func loadAudio(filename string) (model.Audio, error) {
	file, err := os.Open(filename)
	if err != nil {
		return model.Audio{}, fmt.Errorf("open audio: %w", err)
	}
	defer file.Close()

	samples, sampleRate, channels, err := buckyaudio.DecodeWAV(file)
	if err != nil {
		return model.Audio{}, fmt.Errorf("decode WAV audio: %w", err)
	}

	return model.Audio{SampleRate: uint32(sampleRate), Channels: uint32(channels), Data: samples}, nil
}

func saveWAV(filename string, audio model.Audio) error {
	if audio.SampleRate == 0 || audio.Channels == 0 || len(audio.Data)%int(audio.Channels) != 0 {
		return errors.New("save WAV: invalid audio")
	}
	if audio.Channels > math.MaxUint16/2 || uint64(audio.SampleRate)*uint64(audio.Channels)*2 > math.MaxUint32 || uint64(len(audio.Data))*2 > math.MaxUint32-36 {
		return errors.New("save WAV: audio is too large")
	}

	dataSize := uint32(len(audio.Data) * 2)
	blockAlign := uint16(audio.Channels * 2)
	header := make([]byte, 44+dataSize)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], 36+dataSize)
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], uint16(audio.Channels))
	binary.LittleEndian.PutUint32(header[24:28], audio.SampleRate)
	binary.LittleEndian.PutUint32(header[28:32], audio.SampleRate*uint32(blockAlign))
	binary.LittleEndian.PutUint16(header[32:34], blockAlign)
	binary.LittleEndian.PutUint16(header[34:36], 16)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], dataSize)

	for i, sample := range audio.Data {
		sample = min(max(sample, -1), 1)
		binary.LittleEndian.PutUint16(header[44+i*2:], uint16(int16(math.Round(float64(sample*32_767)))))
	}

	if err := os.WriteFile(filename, header, 0o644); err != nil {
		return fmt.Errorf("save WAV: %w", err)
	}

	return nil
}
